package files

// safeio.go — TOCTOU symlink-yarışına dayanıklı dosya mutasyonları.
//
// SORUN: jailJoinStrict() yol'u KONTROL anında EvalSymlinks ile çözer ve resolved bir
// STRING döner. Mutasyon (os.Chmod/os.WriteFile/os.Rename/os.RemoveAll/os.Create/os.Chown)
// SONRADAN o string üzerinde root olarak çalışır. Tenant, kontrol ile işlem arasında
// ara-dizini bir symlink'e takas ederek (yarış) root'u jail-DIŞI bir dosyada işlem yapmaya
// kandırabilir (LPE / yerel yetki yükseltme).
//
// ÇÖZÜM: openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS) ile home'a-göreli, HİÇBİR symlink
// takip etmeden, home dışına ÇIKAMADAN, ATOMİK bir fd al; sonra fd/*at-syscall üzerinden
// işlem yap. "Çöz + işlem" tek adımda kernel'de olur; ara-bileşen symlink takası imkânsızlaşır.
// AlmaLinux 10 / kernel 6.12 openat2'yi destekler.

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// errTooBig: readFileBeneath'in boyut sınırı aşıldığında döndürdüğü sentinel.
var errTooBig = errors.New("dosya boyut sınırını aşıyor")

const dirOpenFlags = unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK

// relClean: kullanıcı-verdiği yolu home'a-GÖRELİ, '..'-temiz bir yola indirger. "/" öneki
// eklenip Clean edilerek her türlü '..' sözlüksel olarak eritilir; asıl zorlamayı yine de
// openat2'nin RESOLVE_BENEATH bayrağı yapar.
func relClean(userYol string) string {
	return strings.TrimPrefix(filepath.Clean("/"+userYol), "/")
}

// openHomeFd: home dizinini O_DIRECTORY ile açar. home (/home/c_<slug>) root tarafından
// oluşturulur; /home root'a aittir → tenant home DİZİN GİRDİSİNİ symlink'e takas edemez,
// bu yüzden home'u doğrudan açmak güvenlidir. Alt bileşenler openat2 ile korunur.
func openHomeFd(home string) (int, error) {
	return unix.Open(home, unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
}

// openAt2Beneath: rel'i home altında, hiçbir symlink takip etmeden, home dışına çıkamadan,
// ATOMİK açar ve *os.File döner (çağıran Close etmeli).
func openAt2Beneath(home, rel string, flags int, mode uint32) (*os.File, error) {
	hf, err := openHomeFd(home)
	if err != nil {
		return nil, err
	}
	defer unix.Close(hf)
	p := relClean(rel)
	if p == "" {
		p = "."
	}
	how := &unix.OpenHow{
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	}
	fd, err := unix.Openat2(hf, p, how)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(home, p)), nil
}

// isDirBeneath: rel home altında bir DİZİN mi? (symlink-güvenli; ara-bileşen symlink ise hata).
func isDirBeneath(home, rel string) (bool, error) {
	f, err := openAt2Beneath(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	return st.IsDir(), nil
}

// safeParentFd: rel'in PARENT dizinini home altında symlink-takipsiz açar (raw fd) ve
// tek-bileşen leaf adını döner. Çağıran unix.Close(parentFd) etmeli. Parent fd pinlenir →
// leaf üstündeki ara-bileşenler artık takas edilemez; yalnız tek leaf işleme konu olur.
func safeParentFd(home, rel string) (parentFd int, leaf string, err error) {
	p := relClean(rel)
	parent := filepath.Dir(p) // "a/b" -> "a", "f" -> "."
	leaf = filepath.Base(p)
	f, err := openAt2Beneath(home, parent, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return -1, "", err
	}
	fd, err := unix.Dup(int(f.Fd()))
	f.Close()
	if err != nil {
		return -1, "", err
	}
	return fd, leaf, nil
}

// tenantIDs: sk (c_<slug>) sistem kullanıcısının uid/gid'i.
func tenantIDs(sk string) (uid, gid int, ok bool) {
	uu, err := userLookup(sk)
	if err != nil {
		return 0, 0, false
	}
	return uu.UID, uu.GID, true
}

// withinHome: p, home'un (symlink-çözülmüş) altında mı? restorecon-by-path gibi
// artık-kalan path işlemlerini jail'e sınırlamak için son bir emniyet kemeri.
func withinHome(home, p string) bool {
	hr, err := filepath.EvalSymlinks(home)
	if err != nil {
		hr = home
	}
	pr, err := filepath.EvalSymlinks(p)
	if err != nil {
		pr = p
	}
	return pr == hr || strings.HasPrefix(pr, hr+string(filepath.Separator))
}

// restoreconFd: fd'nin PİNLENMİŞ gerçek yolunu (/proc/self/fd/N → kernel çözer, saldırgan
// symlink'ine bağışık) alıp, hâlâ home altındaysa restorecon çalıştırır. Enforcing SELinux
// sunucularda root'un oluşturduğu dosya doğru context (httpd_sys_content_t) almazsa
// nginx/php-fpm erişemez; bu yüzden ŞART. within-home kontrolü relabel'ı jail'e sınırlar.
func restoreconFd(home string, f *os.File) {
	real, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd())))
	if err != nil || !withinHome(home, real) {
		return
	}
	_, _ = exec.Command("restorecon", real).CombinedOutput()
}

// fchownRestoreFd: fd'yi tenant'a chown (symlink-güvenli: fd üzerinden Fchown) + SELinux
// context'i düzelt. Eski path-tabanlı chown(abs, sk) os.Chown symlink TAKİP EDERDİ →
// /etc/shadow'u tenant'a devretme (LPE) riski; Fchown pinlenmiş inode'da çalışır.
func fchownRestoreFd(home string, f *os.File, sk string) {
	if uid, gid, ok := tenantIDs(sk); ok {
		_ = unix.Fchown(int(f.Fd()), uid, gid)
	}
	restoreconFd(home, f)
}

// ---- Yüksek seviye, symlink-güvenli mutasyonlar ----

// ---- Symlink-güvenli OKUMA ----
//
// 🔴 Bu bölüm eksikti: mutasyon yolları (yaz/sil/taşı/chmod) openat2'ye taşınmışken
// OKUMA yolları (List/Read/Download) jailJoinStrict'in döndürdüğü RESOLVED STRING
// üzerinde os.ReadDir/os.Stat/os.Open ile çalışmayı sürdürüyordu. jailJoinStrict
// kontrol ANINDA symlink'i çözer; işlem SONRADAN yola göre yapılır. Tenant, kontrol
// ile açma arasında ara bileşeni symlink'e takas ederek (renameat2 RENAME_EXCHANGE
// döngüsü — kazanması kolay bir yarış) root'a jail DIŞINDAKİ bir dosyayı okutabilirdi:
// /etc/shadow, panelin JWT gizli anahtarı, SSL özel anahtarları. Yazma tarafındaki
// aynı sınıf düzeltilmişken okuma tarafının açık kalması, bilgi-sızması yönünde tam
// eşdeğer bir yetki yükseltmesiydi.

// openReadBeneath: home altındaki dosyayı okumak için symlink-güvenli açar.
// Ara bileşen veya leaf symlink ise ELOOP, jail dışına çıkıyorsa EXDEV döner.
func openReadBeneath(home, rel string) (*os.File, error) {
	return openAt2Beneath(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
}

// readFileBeneath: symlink-güvenli dosya okuma. limit'ten büyük dosyalarda
// (ör. editör 2 MB sınırı) okumaya hiç girmeden boyutu da döndürür.
func readFileBeneath(home, rel string, limit int64) (data []byte, boyut int64, err error) {
	f, err := openReadBeneath(home, rel)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if st.IsDir() {
		return nil, 0, unix.EISDIR
	}
	if limit > 0 && st.Size() > limit {
		return nil, st.Size(), errTooBig
	}
	b, err := io.ReadAll(io.LimitReader(f, st.Size()))
	return b, st.Size(), err
}

// statBeneath: symlink-güvenli stat. Yol openat2 ile açılıp fd üzerinden
// stat'lanır — dolayısıyla "stat'lanan" ile "açılan" aynı inode'dur.
// 🔴 O_NONBLOCK KULLANMA: openat2, openat'ten farklı olarak geçersiz bayrak
// birleşimlerini sessizce yok saymaz — EINVAL ile REDDEDER. O_PATH yalnızca
// O_CLOEXEC / O_DIRECTORY / O_NOFOLLOW ile birleşebilir. Buraya eklenmiş olan
// O_NONBLOCK yüzünden statBeneath HER ÇAĞRIDA "invalid argument" dönüyordu:
// arşiv açma "dosya bulunamadı veya klasör" hatası veriyor, arşiv oluşturma
// boyutu 0 raporluyor, arama sonuçlarında izin/tarih boş kalıyordu. Hatanın
// sessiz kalmasının sebebi, symlink testlerinin yalnızca "hata döndü mü" diye
// bakması ve fonksiyon her durumda hata verdiği için geçmesiydi.
func statBeneath(home, rel string) (os.FileInfo, error) {
	f, err := openAt2Beneath(home, rel, unix.O_PATH, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

// dirEntryStat: readDirBeneath'in tek girdi için döndürdüğü ham metadata.
type dirEntryStat struct {
	Ad    string
	Boyut int64
	Mode  os.FileMode
	UID   uint32
	GID   uint32
	MTime time.Time
}

// readDirBeneath: symlink-güvenli dizin listeleme. Dizin openat2 ile pinlenir;
// her girdi AT_SYMLINK_NOFOLLOW ile fstatat edilir — bir girdi jail dışına işaret
// eden symlink olsa bile HEDEFİ değil, bağın KENDİSİ raporlanır (hedefin boyutu/
// izinleri sızmaz).
func readDirBeneath(home, rel string) ([]dirEntryStat, error) {
	f, err := openAt2Beneath(home, rel, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	adlar, err := readdirnamesFd(int(f.Fd()))
	if err != nil {
		return nil, err
	}
	out := make([]dirEntryStat, 0, len(adlar))
	for _, ad := range adlar {
		if ad == "." || ad == ".." {
			continue
		}
		var st unix.Stat_t
		if err := unix.Fstatat(int(f.Fd()), ad, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // yarışta silinmiş olabilir; atla
		}
		out = append(out, dirEntryStat{
			Ad:    ad,
			Boyut: st.Size,
			Mode:  modeFromStat(st.Mode),
			UID:   st.Uid,
			GID:   st.Gid,
			MTime: time.Unix(st.Mtim.Sec, st.Mtim.Nsec),
		})
	}
	return out, nil
}

// modeFromStat: ham st_mode'u os.FileMode'a çevirir (yalnız List'in ayırt ettiği
// tipler: dizin / sembolik bağ / normal dosya).
func modeFromStat(m uint32) os.FileMode {
	fm := os.FileMode(m & 0o777)
	switch m & unix.S_IFMT {
	case unix.S_IFDIR:
		fm |= os.ModeDir
	case unix.S_IFLNK:
		fm |= os.ModeSymlink
	}
	return fm
}

// ---- Dış araçları (zip/tar/find/du/gunzip) tenant kimliğinde çalıştırma ----
//
// 🔴 Bu araçlar bir fd'ye değil bir YOLA çalışır, yani openat2 koruması onlara
// doğrudan uygulanamaz. internal/archivex'in ÇIKARMA tarafında kurduğu çift-savunma
// deseninin arşivLEME/arama/ölçme tarafındaki karşılığı burasıdır:
//
//	Katman 1 (DAC): araç root DEĞİL, tenant (c_<sk>) olarak koşar → yol doğrulaması
//	  bir yarışla aşılsa bile tenant'ın zaten okuyamadığı/yazamadığı bir şeye
//	  erişilemez.
//	Katman 2 (yol doğrulama): kaynak/hedef yolları openat2 ile önceden doğrulanır.

// tenantKomut: argv'yi tenant kullanıcısı olarak, panel sırları OLMADAN, temiz
// env ile çalıştıracak komutu hazırlar (archivex.runuserKomut ile aynı desen).
func tenantKomut(ctx context.Context, sk string, argv ...string) (*exec.Cmd, error) {
	if !strings.HasPrefix(sk, "c_") {
		return nil, errors.New("güvenlik: geçersiz tenant kullanıcısı")
	}
	full := append([]string{"-u", sk, "--"}, argv...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/home/" + sk,
	}
	return cmd, nil
}

// dogrulanmisYol: home/rel'i openat2 ile doğrular (symlink bileşeni veya jail
// kaçışı varsa hata) ve dış araca verilebilecek mutlak yolu döner. Doğrulama ile
// exec arasındaki artık TOCTOU payını tenantKomut'un DAC katmanı kapatır.
func dogrulanmisYol(home, rel string) (string, error) {
	f, err := openAt2Beneath(home, rel, unix.O_PATH, 0)
	if err != nil {
		return "", err
	}
	f.Close()
	return filepath.Join(home, relClean(rel)), nil
}

// ciktiHazirla: bir dış aracın YAZACAĞI hedefi güvenli hâle getirir — üst dizinleri
// symlink-güvenli oluşturur, hedefin kendisi bir SYMLINK ise reddeder (jail dışına
// yazma vektörü), duran normal dosyayı temizler ve mutlak yolu döner.
func ciktiHazirla(home, rel, sk string) (string, error) {
	p := relClean(rel)
	if p == "" || p == "." {
		return "", errors.New("geçersiz çıktı yolu")
	}
	if err := mkdirAllBeneath(home, filepath.Dir(p), sk); err != nil {
		return "", err
	}
	pfd, leaf, err := safeParentFd(home, p)
	if err != nil {
		return "", err
	}
	defer unix.Close(pfd)
	var st unix.Stat_t
	switch err := unix.Fstatat(pfd, leaf, &st, unix.AT_SYMLINK_NOFOLLOW); {
	case err == unix.ENOENT: // yok — araç oluşturacak
	case err != nil:
		return "", err
	case st.Mode&unix.S_IFMT == unix.S_IFLNK:
		return "", errors.New("güvenlik: çıktı yolu sembolik bağ — reddedildi")
	default:
		// Duran dosyayı kaldır: araçlar (zip) mevcut dosyayı "bozuk arşiv" sanabilir.
		if err := unix.Unlinkat(pfd, leaf, 0); err != nil && err != unix.ENOENT {
			return "", err
		}
	}
	return filepath.Join(home, p), nil
}

// chmodBeneath: symlink-güvenli chmod. Leaf'i openat2 ile (symlink ise REDDEDİLİR) açıp
// Fchmod uygular; ara-bileşen takası kernel tarafından engellenir.
func chmodBeneath(home, rel string, mode uint32) error {
	f, err := openAt2Beneath(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Fchmod(int(f.Fd()), mode)
}

// writeBeneath: symlink-güvenli dosya yazma (oluştur/truncate). Mevcut dosyanın izinleri
// korunur (open, create-dışında mode'a dokunmaz); yeni dosya createMode alır. Ardından fd
// üzerinden tenant'a chown + restorecon.
func writeBeneath(home, rel string, data []byte, createMode uint32, sk string) error {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC, createMode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	fchownRestoreFd(home, f, sk)
	return nil
}

// createExclBeneath: symlink-güvenli yeni-boş-dosya (O_EXCL). Zaten varsa unix.EEXIST.
func createExclBeneath(home, rel, sk string) error {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0644)
	if err != nil {
		return err
	}
	fchownRestoreFd(home, f, sk)
	return f.Close()
}

// copyStreamBeneath: symlink-güvenli akışlı yazma (upload). src'den fd'ye kopyalar.
func copyStreamBeneath(home, rel string, src io.Reader, sk string) (int64, error) {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, src)
	if err != nil {
		return n, err
	}
	fchownRestoreFd(home, f, sk)
	return n, nil
}

// mkdirAllBeneath: symlink-güvenli `mkdir -p`. Her bileşeni Mkdirat + O_NOFOLLOW openat ile
// yürür; herhangi bir bileşen symlink ise O_NOFOLLOW REDDEDER. Yeni oluşturulan dizinler
// (sk != "") tenant'a chown edilir.
func mkdirAllBeneath(home, rel, sk string) error {
	p := relClean(rel)
	hf, err := openHomeFd(home)
	if err != nil {
		return err
	}
	if p == "" || p == "." {
		unix.Close(hf)
		return nil
	}
	dirfd := hf
	uid, gid, haveIDs := tenantIDs(sk)
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." {
			continue
		}
		created := false
		if err := unix.Mkdirat(dirfd, part, 0755); err == nil {
			created = true
		} else if err != unix.EEXIST {
			unix.Close(dirfd)
			return err
		}
		nfd, err := unix.Openat(dirfd, part, dirOpenFlags, 0)
		unix.Close(dirfd)
		if err != nil {
			return err
		}
		dirfd = nfd
		if created && haveIDs {
			_ = unix.Fchown(dirfd, uid, gid)
		}
	}
	unix.Close(dirfd)
	return nil
}

// renameBeneath: symlink-güvenli rename/move. Kaynak ve hedef PARENT'ları openat2 ile
// pinler, Renameat ile taşır (rename final-bileşen symlink'ini TAKİP ETMEZ, girdiyi taşır).
func renameBeneath(home, oldRel, newRel, sk string) error {
	if err := mkdirAllBeneath(home, filepath.Dir(relClean(newRel)), sk); err != nil {
		return err
	}
	of, oleaf, err := safeParentFd(home, oldRel)
	if err != nil {
		return err
	}
	defer unix.Close(of)
	nf, nleaf, err := safeParentFd(home, newRel)
	if err != nil {
		return err
	}
	defer unix.Close(nf)
	return unix.Renameat(of, oleaf, nf, nleaf)
}

// removeAllBeneath: symlink-güvenli `rm -rf`. Parent'ı pinler, leaf'i (dosya/symlink ise
// unlink; dizin ise fd-özyinelemeli unlinkat) siler. Hiçbir adımda symlink takip edilmez.
func removeAllBeneath(home, rel string) error {
	pfd, leaf, err := safeParentFd(home, rel)
	if err != nil {
		return err
	}
	defer unix.Close(pfd)
	return removeAt(pfd, leaf)
}

// removeAt: dirfd'ye göre name'i özyinelemeli sil (tüm işlemler pinlenmiş fd'lere göreli,
// O_NOFOLLOW → symlink asla takip edilmez, jail-dışı silme imkânsız).
func removeAt(dirfd int, name string) error {
	if err := unix.Unlinkat(dirfd, name, 0); err == nil {
		return nil
	} else if err == unix.ENOENT {
		return nil
	} else if err != unix.EISDIR && err != unix.EPERM && err != unix.ENOTEMPTY {
		return err
	}
	cfd, err := unix.Openat(dirfd, name, dirOpenFlags, 0)
	if err != nil {
		return err
	}
	names, rerr := readdirnamesFd(cfd)
	if rerr != nil {
		unix.Close(cfd)
		return rerr
	}
	for _, n := range names {
		if n == "." || n == ".." {
			continue
		}
		if e := removeAt(cfd, n); e != nil {
			unix.Close(cfd)
			return e
		}
	}
	unix.Close(cfd)
	return unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR)
}

// readdirnamesFd: raw dir fd'yi (sahipliğini almadan) listeler. Dup + os.File ile okuyup
// dup'ı kapatır; asıl fd çağırana kalır.
func readdirnamesFd(dirfd int) ([]string, error) {
	dup, err := unix.Dup(dirfd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), "dir")
	names, err := f.Readdirnames(-1)
	f.Close()
	return names, err
}

// chownTreeBeneath: dst alt-ağacını symlink-güvenli özyinelemeli tenant'a chown eder
// (Fchownat AT_SYMLINK_NOFOLLOW → symlink'in KENDİSİ chown edilir, hedefi değil).
func chownTreeBeneath(home, rel, sk string) error {
	uid, gid, ok := tenantIDs(sk)
	if !ok {
		return nil // kullanıcı yok → sessiz atla (test/kenar durum)
	}
	pfd, leaf, err := safeParentFd(home, rel)
	if err != nil {
		return err
	}
	defer unix.Close(pfd)
	return chownAt(pfd, leaf, uid, gid)
}

func chownAt(dirfd int, name string, uid, gid int) error {
	if err := unix.Fchownat(dirfd, name, uid, gid, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	cfd, err := unix.Openat(dirfd, name, dirOpenFlags, 0)
	if err != nil {
		return nil // dosya/symlink → alt yok
	}
	names, rerr := readdirnamesFd(cfd)
	if rerr != nil {
		unix.Close(cfd)
		return rerr
	}
	for _, n := range names {
		if n == "." || n == ".." {
			continue
		}
		if e := chownAt(cfd, n, uid, gid); e != nil {
			unix.Close(cfd)
			return e
		}
	}
	unix.Close(cfd)
	return nil
}

// chmodTreeBeneath: symlink-güvenli özyinelemeli chmod. Dizinler dirMode, dosyalar fileMode
// alır; symlink'ler ATLANIR — Linux'ta symlink'in kendi izin bitleri anlamsızdır/ayarlanamaz
// ve fchmodat sembolik BAĞI TAKİP EDER (AT_SYMLINK_NOFOLLOW desteklemez), bu yüzden hiç
// çağrılmaz — jail-dışı (ör. public_html/link -> /etc/passwd) bir hedefin chmod'lanması
// böylece imkânsızdır.
func chmodTreeBeneath(home, rel string, dirMode, fileMode uint32) error {
	pfd, leaf, err := safeParentFd(home, rel)
	if err != nil {
		return err
	}
	defer unix.Close(pfd)
	return chmodAt(pfd, leaf, dirMode, fileMode)
}

func chmodAt(dirfd int, name string, dirMode, fileMode uint32) error {
	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if err == unix.ENOENT {
			return nil
		}
		return err
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		return nil
	case unix.S_IFDIR:
		if err := unix.Fchmodat(dirfd, name, dirMode, 0); err != nil {
			return err
		}
		cfd, err := unix.Openat(dirfd, name, dirOpenFlags, 0)
		if err != nil {
			return nil
		}
		names, rerr := readdirnamesFd(cfd)
		if rerr != nil {
			unix.Close(cfd)
			return rerr
		}
		for _, n := range names {
			if n == "." || n == ".." {
				continue
			}
			if e := chmodAt(cfd, n, dirMode, fileMode); e != nil {
				unix.Close(cfd)
				return e
			}
		}
		unix.Close(cfd)
		return nil
	default:
		return unix.Fchmodat(dirfd, name, fileMode, 0)
	}
}

// copyTreeBeneath: symlink-güvenli özyinelemeli kopya. Kaynak ve hedef PARENT'ları pinler;
// dosyaları O_NOFOLLOW ile kopyalar (jail-dışı symlink İÇERİĞİ okunmaz → bilgi sızması yok),
// symlink'leri (readlink+symlinkat ile) OLDUĞU gibi yeniden kurar, dizinleri özyineler.
func copyTreeBeneath(home, srcRel, dstRel, sk string) error {
	if err := mkdirAllBeneath(home, filepath.Dir(relClean(dstRel)), sk); err != nil {
		return err
	}
	sfd, sleaf, err := safeParentFd(home, srcRel)
	if err != nil {
		return err
	}
	defer unix.Close(sfd)
	dfd, dleaf, err := safeParentFd(home, dstRel)
	if err != nil {
		return err
	}
	defer unix.Close(dfd)
	uid, gid, haveIDs := tenantIDs(sk)
	return copyEntryAt(sfd, sleaf, dfd, dleaf, uid, gid, haveIDs)
}

func copyEntryAt(sdir int, sname string, ddir int, dname string, uid, gid int, haveIDs bool) error {
	var st unix.Stat_t
	if err := unix.Fstatat(sdir, sname, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		if err := unix.Mkdirat(ddir, dname, st.Mode&0o777); err != nil && err != unix.EEXIST {
			return err
		}
		ncd, err := unix.Openat(ddir, dname, dirOpenFlags, 0)
		if err != nil {
			return err
		}
		defer unix.Close(ncd)
		if haveIDs {
			_ = unix.Fchown(ncd, uid, gid)
		}
		nsd, err := unix.Openat(sdir, sname, dirOpenFlags, 0)
		if err != nil {
			return err
		}
		defer unix.Close(nsd)
		names, rerr := readdirnamesFd(nsd)
		if rerr != nil {
			return rerr
		}
		for _, n := range names {
			if n == "." || n == ".." {
				continue
			}
			if e := copyEntryAt(nsd, n, ncd, n, uid, gid, haveIDs); e != nil {
				return e
			}
		}
		return nil
	case unix.S_IFLNK:
		target, err := readlinkAt(sdir, sname)
		if err != nil {
			return err
		}
		_ = unix.Unlinkat(ddir, dname, 0)
		return unix.Symlinkat(target, ddir, dname)
	case unix.S_IFREG:
		return copyRegAt(sdir, sname, ddir, dname, st.Mode&0o777, uid, gid, haveIDs)
	default:
		return nil // özel dosyaları atla
	}
}

func readlinkAt(dirfd int, name string) (string, error) {
	buf := make([]byte, 4096)
	n, err := unix.Readlinkat(dirfd, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func copyRegAt(sdir int, sname string, ddir int, dname string, perm uint32, uid, gid int, haveIDs bool) error {
	sf, err := unix.Openat(sdir, sname, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	in := os.NewFile(uintptr(sf), sname)
	defer in.Close()
	df, err := unix.Openat(ddir, dname, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, perm)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(df), dname)
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if haveIDs {
		_ = unix.Fchown(df, uid, gid)
	}
	return nil
}
