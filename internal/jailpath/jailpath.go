// Package jailpath: tenant ev dizini (/home/c_<slug>) altındaki yolları
// symlink-güvenli biçimde açar/oluşturur/temizler.
//
// SORUN: Panel root olarak çalışır (bkz. assets/systemd/sanalcp.service).
// Tenant kendi home'unda İSTEDİĞİ yere symlink kurabilir (FTP, SSH, PHP
// symlink() — paylaşımlı hosting'in tanımı gereği). Root olarak yapılan
// path-tabanlı bir dosya işlemi (os.MkdirAll, os.ReadDir+os.RemoveAll,
// rsync <hedef>/) bu symlink'i TAKİP eder ve jail DIŞINDA çalışır:
//
//	ln -s /etc /home/c_x/public_html   →  rsync -a --delete ... /home/c_x/public_html/
//	                                       root olarak /etc'ye yazar ve /etc'yi siler.
//
// ÇÖZÜM: openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS) ile home'a-göreli,
// hiçbir symlink takip etmeden, home dışına çıkamadan ATOMİK fd al; işlemi
// fd/*at-syscall üzerinden yap. "Çöz + işlem" tek adımda kernel'de olur.
//
// KARDEŞ UYGULAMA: internal/files/safeio.go aynı tekniği dosya yöneticisi
// için uygular (ayrı ve daha geniş bir API'si var, kendi testleriyle birlikte).
// Bu paket o korumanın dosya yöneticisi DIŞINDA kalan çağrı yerleri (yedek
// geri yükleme, git deploy, site kopyası, subdomain/ek-domain docroot) için
// ihtiyaç duyulan alt kümesidir.
//
// exec ile çalışan araçlar (rsync gibi) bir fd'ye yazamaz — onlarda bu paketin
// doğrulaması ilk katman, aracı `runuser -u <tenant>` ile tenant kimliğinde
// çalıştırmak (DAC) ise doğrulama-ile-çalıştırma arasındaki TOCTOU yarışını
// kapatan ikinci katmandır (bkz. internal/transfers'daki aynı desen).
package jailpath

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sanalcp/internal/adlar"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// HomeKok: tenant ev dizinlerinin kökü.
const HomeKok = "/home"

const dizinBayrak = unix.O_DIRECTORY | unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK

// TenantHome: sistem kullanıcısını doğrular ve ev dizinini döner.
// sk mutlaka "c_" ile başlamalı (panelin tenant kullanıcı deseni) ve yol
// bileşeni içermemeli — aksi hâlde "../../root" gibi bir değer home kökünden
// çıkabilirdi.
func TenantHome(sk string) (string, error) {
	if !adlar.SKGecerli(sk) {
		return "", fmt.Errorf("jailpath: geçersiz tenant kullanıcısı: %q", sk)
	}
	return filepath.Join(HomeKok, sk), nil
}

// TenantIDs: sk kullanıcısının uid/gid'i (yoksa ok=false).
func TenantIDs(sk string) (uid, gid int, ok bool) {
	u, err := user.Lookup(sk)
	if err != nil {
		return 0, 0, false
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return uid, gid, true
}

// temizRel: kullanıcı-verdiği yolu home'a-göreli, '..'-temiz bir yola indirger.
// Asıl zorlamayı yine openat2'nin RESOLVE_BENEATH bayrağı yapar; bu yalnızca
// sözlüksel bir ön-sadeleştirmedir.
func temizRel(rel string) string {
	p := strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	if p == "" {
		return "."
	}
	return p
}

// homeFd: home dizinini açar. /home root'a aittir → tenant home GİRDİSİNİ
// symlink'e takas edemez, bu yüzden home'u doğrudan açmak güvenlidir; alt
// bileşenlerin tamamı openat2 ile korunur.
func homeFd(home string) (int, error) {
	return unix.Open(home, unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
}

// Ac: home altında rel'i, hiçbir symlink takip etmeden ve home dışına
// çıkamadan ATOMİK açar. Çağıran Close etmelidir.
//
// rel'in herhangi bir bileşeni symlink ise ELOOP, home dışına çıkıyorsa EXDEV
// döner — yani saldırı sessizce başarısız olmaz, açık bir hata verir.
func Ac(home, rel string, flags int, mode uint32) (*os.File, error) {
	hf, err := homeFd(home)
	if err != nil {
		return nil, err
	}
	defer unix.Close(hf)
	p := temizRel(rel)
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

// AcDizin: rel'i symlink-güvenli bir DİZİN olarak açar.
func AcDizin(home, rel string) (*os.File, error) {
	return Ac(home, rel, dizinBayrak, 0)
}

// DizinDogrula: home/rel gerçekten (symlink içermeyen) bir dizin mi?
// rsync/tar gibi exec ile çalışan araçlara bir yol vermeden ÖNCE çağrılır —
// tek başına TOCTOU'ya kapalı değildir, aracın tenant kimliğinde
// çalıştırılmasıyla birlikte kullanılmalıdır (bkz. paket açıklaması).
func DizinDogrula(home, rel string) error {
	f, err := AcDizin(home, rel)
	if err != nil {
		return err
	}
	return f.Close()
}

// DizinOlustur: symlink-güvenli `mkdir -p`. Her bileşen Mkdirat + O_NOFOLLOW
// openat ile yürünür; bileşenlerden biri symlink ise işlem REDDEDİLİR.
// Yeni oluşturulan dizinler (uid/gid bulunabiliyorsa) tenant'a devredilir.
func DizinOlustur(home, rel, sk string) error {
	p := temizRel(rel)
	hf, err := homeFd(home)
	if err != nil {
		return err
	}
	if p == "." {
		unix.Close(hf)
		return nil
	}
	uid, gid, idVar := TenantIDs(sk)
	dirfd := hf
	for _, parca := range strings.Split(p, "/") {
		if parca == "" || parca == "." {
			continue
		}
		yeni := false
		if err := unix.Mkdirat(dirfd, parca, 0o755); err == nil {
			yeni = true
		} else if err != unix.EEXIST {
			unix.Close(dirfd)
			return err
		}
		// O_NOFOLLOW: bileşen symlink ise ELOOP — jail dışına adım atılamaz.
		nfd, err := unix.Openat(dirfd, parca, dizinBayrak|unix.O_NOFOLLOW, 0)
		unix.Close(dirfd)
		if err != nil {
			return err
		}
		dirfd = nfd
		if yeni && idVar {
			_ = unix.Fchown(dirfd, uid, gid)
		}
	}
	unix.Close(dirfd)
	return nil
}

// DosyaYaz: home altına symlink-güvenli dosya yazar (oluştur/truncate) ve
// tenant'a devreder. Hedef zaten bir symlink ise RESOLVE_NO_SYMLINKS reddeder.
func DosyaYaz(home, rel, sk string, veri []byte, mode uint32) error {
	f, err := Ac(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(veri); err != nil {
		return err
	}
	if uid, gid, ok := TenantIDs(sk); ok {
		_ = unix.Fchown(int(f.Fd()), uid, gid)
	}
	return nil
}

// Tasi: home içindeki iki yol arasında, parent dizinleri openat2 ile pinleyerek
// atomik ve symlink takip etmeyen taşıma yapar. Hedef varsa üzerine yazmaz.
func Tasi(home, eskiRel, yeniRel string) error {
	eskiRel, yeniRel = temizRel(eskiRel), temizRel(yeniRel)
	eskiAd, yeniAd := filepath.Base(eskiRel), filepath.Base(yeniRel)
	if eskiAd == "." || yeniAd == "." || eskiAd == ".." || yeniAd == ".." {
		return fmt.Errorf("jailpath: geçersiz taşıma yolu")
	}
	eskiDizin, yeniDizin := filepath.Dir(eskiRel), filepath.Dir(yeniRel)
	ef, err := AcDizin(home, eskiDizin)
	if err != nil {
		return err
	}
	defer ef.Close()
	yf, err := AcDizin(home, yeniDizin)
	if err != nil {
		return err
	}
	defer yf.Close()
	return unix.Renameat2(int(ef.Fd()), eskiAd, int(yf.Fd()), yeniAd, unix.RENAME_NOREPLACE)
}

// IceriginiSil: home/rel dizininin İÇERİĞİNİ symlink-güvenli siler (dizinin
// kendisi kalır). Tüm işlemler pinlenmiş fd'lere görelidir; hiçbir adımda
// symlink takip edilmez — jail dışında silme imkânsızdır.
//
// Buradaki fd-göreli özyineleme TOCTOU'ya TAM kapalıdır (exec yok, ara
// bileşen takası kernel tarafından engellenir).
func IceriginiSil(home, rel string) error {
	f, err := AcDizin(home, rel)
	if err != nil {
		return err
	}
	defer f.Close()
	adlar, err := adlariOku(int(f.Fd()))
	if err != nil {
		return err
	}
	for _, ad := range adlar {
		if ad == "." || ad == ".." {
			continue
		}
		if err := silAt(int(f.Fd()), ad); err != nil {
			return err
		}
	}
	return nil
}

// Adlari: home/rel dizinindeki girdi adlarını symlink-güvenli listeler.
// Dizinin kendisine giden yolun hiçbir bileşeni symlink olamaz; girdiler ham
// ad olarak döner (alt dizin/symlink ayrımı yapılmaz).
func Adlari(home, rel string) ([]string, error) {
	f, err := AcDizin(home, rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	adlar, err := adlariOku(int(f.Fd()))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(adlar))
	for _, ad := range adlar {
		if ad == "." || ad == ".." {
			continue
		}
		out = append(out, ad)
	}
	return out, nil
}

// Sil: home/rel girdisini (dosya veya dizin) symlink-güvenli, özyinelemeli
// siler. IceriginiSil'den farkı: girdinin KENDİSİ de silinir.
func Sil(home, rel string) error {
	p := temizRel(rel)
	if p == "." {
		return fmt.Errorf("jailpath: ev dizininin kendisi silinemez")
	}
	ust := filepath.Dir(p)
	ad := filepath.Base(p)
	f, err := AcDizin(home, ust)
	if err != nil {
		return err
	}
	defer f.Close()
	return silAt(int(f.Fd()), ad)
}

// silAt: dirfd'ye göre name'i özyinelemeli siler (symlink'in KENDİSİ silinir,
// hedefi değil — unlinkat sembolik bağı takip etmez).
func silAt(dirfd int, ad string) error {
	err := unix.Unlinkat(dirfd, ad, 0)
	switch err {
	case nil, unix.ENOENT:
		return nil
	case unix.EISDIR, unix.EPERM, unix.ENOTEMPTY:
		// dizin → içeriğini boşaltıp rmdir
	default:
		return err
	}
	cfd, err := unix.Openat(dirfd, ad, dizinBayrak|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	adlar, rerr := adlariOku(cfd)
	if rerr != nil {
		unix.Close(cfd)
		return rerr
	}
	for _, alt := range adlar {
		if alt == "." || alt == ".." {
			continue
		}
		if e := silAt(cfd, alt); e != nil {
			unix.Close(cfd)
			return e
		}
	}
	unix.Close(cfd)
	return unix.Unlinkat(dirfd, ad, unix.AT_REMOVEDIR)
}

// adlariOku: raw dir fd'yi SAHİPLİĞİNİ ALMADAN listeler (dup + os.File; asıl
// fd çağırana kalır).
func adlariOku(dirfd int) ([]string, error) {
	dup, err := unix.Dup(dirfd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), "dir")
	adlar, err := f.Readdirnames(-1)
	f.Close()
	return adlar, err
}
