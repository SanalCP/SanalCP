// Package archivex: jail'li + tenant-user + symlink-korumalı ortak arşiv çıkarma.
//
// Güvenlik modeli (çift savunma / defense-in-depth):
//
//	Katman 1 (DAC): çıkarma işlemi ROOT değil, tenant kullanıcısı (c_<sk>) olarak
//	  `runuser -u <sk>` ile çalışır. Bir symlink/hardlink üyesi jail'i aşsa bile,
//	  yetkisiz kullanıcı başka tenant'ın home'una veya /root'a YAZAMAZ.
//	Katman 2 (üye doğrulama): çıkarmadan ÖNCE arşiv Go stdlib (archive/zip,
//	  archive/tar) ile taranır; mutlak yollu, ".." bileşenli, jail dışına çıkan veya
//	  symlink/hardlink/aygıt üyesi tespit edilirse çıkarma tamamen REDDEDİLİR.
//
// Bu iki katman birbirinden bağımsızdır: biri baypas edilse bile diğeri korur.
// Bu paket, hem dosya yöneticisi Extract hem de yedek Restore tarafından ORTAK
// kullanılır (tek güvenli-extract yolu).
package archivex

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sanalcp/internal/adlar"
	"sort"
	"strconv"
	"strings"
)

// Güvenlik hataları.
var (
	// ErrDesteklenmeyen: bu ortak helper üye-tabanlı arşivleri (zip/tar ailesi/rar) çıkarır;
	// tek dosyalık .gz çağıran tarafından ayrı ele alınır.
	ErrDesteklenmeyen = errors.New("desteklenmeyen arşiv formatı (zip, tar, tar.gz/tgz, tar.bz2, tar.xz, rar)")
	// ErrRarAraciYok: .rar için sistemde açıcı (7z/unar/unrar) kurulu değil.
	ErrRarAraciYok = errors.New("güvenlik: sunucuda RAR açıcı (7z/unar/unrar) kurulu değil — .rar açılamıyor")
	// ErrUyeJailDisi: arşiv üyesi mutlak yol / ".." ile jail dışına çıkmaya çalışıyor.
	ErrUyeJailDisi = errors.New("güvenlik: arşiv üyesi ev dizini (jail) dışına çıkıyor — reddedildi")
	// ErrUyeSymlink: arşivde symlink/hardlink/aygıt üyesi var (jail-escape vektörü) — reddedildi.
	ErrUyeSymlink = errors.New("güvenlik: arşiv içinde symlink/hardlink/aygıt üyesi reddedildi")
)

// Tur: desteklenen arşiv türleri.
type Tur int

const (
	TurBilinmeyen Tur = iota
	TurZip
	TurTar
	TurTarGz
	TurTarBz2
	TurTarXz
	TurRar
)

// TuruBelirle: dosya adının uzantısından arşiv türünü döndürür (küçük harfe duyarsız).
func TuruBelirle(ad string) Tur {
	low := strings.ToLower(ad)
	switch {
	case strings.HasSuffix(low, ".zip"):
		return TurZip
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		return TurTarGz
	case strings.HasSuffix(low, ".tar.bz2"), strings.HasSuffix(low, ".tbz2"):
		return TurTarBz2
	case strings.HasSuffix(low, ".tar.xz"), strings.HasSuffix(low, ".txz"):
		return TurTarXz
	case strings.HasSuffix(low, ".tar"):
		return TurTar
	case strings.HasSuffix(low, ".rar"):
		return TurRar
	}
	return TurBilinmeyen
}

// uyeAdiTehlikeli: bir arşiv üye adı, çıkarma aracının (tar/unzip) HEDEF dizini aşmasına
// yol açar mı? Aracın ham adı nasıl yorumladığını modeller: mutlak yol veya ".." bileşeni
// içeriyorsa tehlikelidir. (Ham adı sanitize etmeyiz — tespit edip reddederiz.)
func uyeAdiTehlikeli(ad string) bool {
	// zip içinde Windows tarzı ters-eğik-çizgi ayraç gelebilir; onu da böl.
	ad = strings.ReplaceAll(ad, "\\", "/")
	if ad == "" {
		return false // boş ad zararsız; araç zaten atlar
	}
	if strings.HasPrefix(ad, "/") {
		return true // mutlak yol
	}
	for _, part := range strings.Split(ad, "/") {
		if part == ".." {
			return true // yol yukarı-çıkış bileşeni
		}
	}
	return false
}

// Tara: arşivin TÜM üyelerini Go stdlib ile önceden tarar; tehlikeli bir üye
// (jail-dışı ad, symlink, hardlink, aygıt) bulursa hata döner. Hiçbir şey yazmaz.
func Tara(archivePath string, tur Tur) error {
	switch tur {
	case TurZip:
		return zipTara(archivePath)
	case TurTar, TurTarGz, TurTarBz2, TurTarXz:
		return tarTara(archivePath, tur)
	case TurRar:
		return rarTara(archivePath)
	default:
		return ErrDesteklenmeyen
	}
}

// rarAraclari: RAR açmak için tercih sırasıyla denenecek araçlar.
//
//	bsdtar (libarchive) — PRİMER: AlmaLinux 10 base/appstream'de var, RAR/RAR5 güvenilir okur,
//	  temiz listeler (-tf), üstelik kendisi de ".." ve mutlak yolu REDDEDER (ekstra savunma).
//	unar/unrar — fallback.
//
// 🔴 NOT: `7z` (AlmaLinux 10 default = 7-Zip 26.02) RAR codec içermez ("Cannot open the file
// as archive") ve p7zip 7zip paketiyle çakışır → 7z LİSTEDE YOK. bsdtar en güvenilir seçim.
var rarAraclari = []string{"bsdtar", "unar", "unrar"}

// rarAraci: sistemde kurulu ilk RAR açıcıyı (tercih sırasıyla) döndürür.
func rarAraci() (string, bool) {
	for _, t := range rarAraclari {
		if _, err := exec.LookPath(t); err == nil {
			return t, true
		}
	}
	return "", false
}

// rarUyeAdlari: seçilen araçla arşivdeki üye ADLARINI listeler (Katman 2 ön-taraması için).
func rarUyeAdlari(tool, archivePath string) ([]string, error) {
	var names []string
	switch tool {
	case "bsdtar":
		// -tf: üye adları, satır başına bir tane (temiz).
		out, err := exec.Command("bsdtar", "-tf", archivePath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("rar liste (bsdtar): %s", strings.TrimSpace(string(out)))
		}
		for _, ln := range strings.Split(string(out), "\n") {
			if s := strings.TrimRight(ln, "\r"); strings.TrimSpace(s) != "" {
				names = append(names, s)
			}
		}
	case "unar":
		// lsar: ilk satır "archive.rar: RAR" başlığı; sonraki satırlar üyeler.
		out, err := exec.Command("lsar", archivePath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("rar liste (lsar): %s", strings.TrimSpace(string(out)))
		}
		lines := strings.Split(string(out), "\n")
		for i, ln := range lines {
			s := strings.TrimSpace(ln)
			if i == 0 || s == "" {
				continue
			}
			names = append(names, s)
		}
	case "unrar":
		// unrar-free çıktısı gürültülü (banner + tablo başlığı). Yalnız üye satırlarını süz:
		// başlık/ayraç/banner olmayan, dosya-yolu gibi görünen satırları al.
		out, err := exec.Command("unrar", "lb", archivePath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("rar liste (unrar): %s", strings.TrimSpace(string(out)))
		}
		for _, ln := range strings.Split(string(out), "\n") {
			s := strings.TrimSpace(ln)
			if s == "" || strings.HasPrefix(s, "unrar") || strings.HasPrefix(s, "RAR archive") ||
				strings.HasPrefix(s, "Pathname") || strings.HasPrefix(s, "Size") ||
				strings.HasPrefix(s, "Copyright") || strings.HasPrefix(s, "----") ||
				strings.HasPrefix(s, "Extracting") || strings.HasPrefix(s, "All OK") {
				continue
			}
			names = append(names, s)
		}
	default:
		return nil, ErrRarAraciYok
	}
	return names, nil
}

// rarTara: RAR üyelerini araç yardımıyla ÖN-TARAR. zip/tar için Go-stdlib pre-scan'in
// karşılığı: mutlak yol / ".." içeren üyeler REDDEDİLİR (ErrUyeJailDisi). Sembolik-bağlantı
// gerçek koruması Katman 1 (tenant-user DAC) tarafından sağlanır: RAR bir symlink içerse
// bile çıkarma tenant kimliğinde ve tenant'ın KENDİ home'una yapılır — komşu tenant'a/sisteme
// yazamaz (0710 home + DAC). Ayrıca primer araç bsdtar ".."/mutlak yolu KENDİSİ de reddeder.
func rarTara(archivePath string) error {
	tool, ok := rarAraci()
	if !ok {
		return ErrRarAraciYok
	}
	names, err := rarUyeAdlari(tool, archivePath)
	if err != nil {
		return err
	}
	for _, n := range names {
		if uyeAdiTehlikeli(n) {
			return ErrUyeJailDisi
		}
	}
	return nil
}

func zipTara(archivePath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip okuma: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		// Symlink üyesi (zip'te mod bitlerinden anlaşılır) → reddet.
		if f.Mode()&os.ModeSymlink != 0 {
			return ErrUyeSymlink
		}
		if uyeAdiTehlikeli(f.Name) {
			return ErrUyeJailDisi
		}
	}
	return nil
}

func tarTara(archivePath string, tur Tur) error {
	return tarGez(archivePath, tur, func(ad string, _ int64, _ bool) error {
		if uyeAdiTehlikeli(ad) {
			return ErrUyeJailDisi
		}
		return nil
	})
}

// tarGez: tar ailesindeki üyeleri dolaşır; tehlikeli üye TİPLERİNİ reddeder,
// her geçerli üye için fn çağırır (ad doğrulaması fn'in işi).
func tarGez(archivePath string, tur Tur, fn func(ad string, boyut int64, dizin bool) error) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("arşiv okuma: %w", err)
	}
	defer f.Close()

	var r io.Reader = f
	switch tur {
	case TurTarGz:
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return fmt.Errorf("gzip: %w", gerr)
		}
		defer gz.Close()
		r = gz
	case TurTarBz2:
		r = bzip2.NewReader(f)
	case TurTarXz:
		// Go stdlib xz çözmez → sadece TARAMA için `xz -dc` ile aç (root okur).
		xzc := exec.Command("xz", "-dc")
		xzc.Stdin = f
		pipe, perr := xzc.StdoutPipe()
		if perr != nil {
			return fmt.Errorf("xz pipe: %w", perr)
		}
		if serr := xzc.Start(); serr != nil {
			return fmt.Errorf("xz başlat: %w", serr)
		}
		defer func() { _ = xzc.Wait() }()
		defer pipe.Close()
		r = pipe
	}

	tr := tar.NewReader(r)
	for {
		hdr, nerr := tr.Next()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			return fmt.Errorf("tar okuma: %w", nerr)
		}
		// Tehlikeli üye tipleri: symlink, hardlink, char/block aygıt, fifo → reddet.
		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return ErrUyeSymlink
		}
		if err := fn(hdr.Name, hdr.Size, hdr.Typeflag == tar.TypeDir); err != nil {
			return err
		}
	}
	return nil
}

// Ozet: bir arşivin ÇIKARILMADAN alınan envanteri. "Genel içe aktarma"
// akışında kullanıcıya arşivin ne içerdiğini göstermek ve tek ortak kök
// klasörü (public_html/, htdocs/domain.com/ …) tespit etmek için.
type Ozet struct {
	UyeSayisi  int      `json:"uye_sayisi"`
	ToplamBayt int64    `json:"toplam_bayt"`
	KokKlasor  string   `json:"kok_klasor"` // tek ortak kök varsa adı, yoksa ""
	Kokler     []string `json:"kokler"`     // en üst seviyedeki ilk birkaç girdi
	// Isaretler: aranan işaret dosyalarının bulunduğu dizinler (arşiv içi,
	// kök klasör DAHİL). Örn. "wp-config.php" -> ["public_html"].
	Isaretler map[string][]string `json:"isaretler"`
}

// ozetKokSiniri: farklı kök girdisi sayısı bunu aşarsa tek-kök olmadığı kesindir;
// listeyi büyütmeyi bırakırız (bellek sınırı).
const ozetKokSiniri = 64

// ozetIsaretSiniri: bir işaret dosyası için saklanacak azami dizin sayısı.
const ozetIsaretSiniri = 32

// Ozetle: arşivi çıkarmadan tarar ve envanterini döndürür. Güvenlik ön-taraması
// (Tara ile aynı kurallar: mutlak yol / ".." / symlink-hardlink-aygıt reddi) bu
// geçişte de uygulanır — özet alınabiliyorsa arşiv çıkarılabilir demektir.
//
// isaretAdlari: aranacak dosya adları (örn. wp-config.php, artisan, .env).
func Ozetle(archivePath string, tur Tur, isaretAdlari []string) (Ozet, error) {
	oz := Ozet{Kokler: []string{}, Isaretler: map[string][]string{}}
	isaret := make(map[string]bool, len(isaretAdlari))
	for _, a := range isaretAdlari {
		isaret[strings.ToLower(a)] = true
	}
	kokSet := map[string]bool{}
	kokDosyaVar := false

	ekle := func(ad string, boyut int64, dizin bool) error {
		if uyeAdiTehlikeli(ad) {
			return ErrUyeJailDisi
		}
		temiz := strings.Trim(strings.TrimPrefix(strings.ReplaceAll(ad, "\\", "/"), "./"), "/")
		if temiz == "" {
			return nil
		}
		oz.UyeSayisi++
		oz.ToplamBayt += boyut
		parcalar := strings.Split(temiz, "/")
		if len(kokSet) <= ozetKokSiniri && !kokSet[parcalar[0]] {
			kokSet[parcalar[0]] = true
			oz.Kokler = append(oz.Kokler, parcalar[0])
		}
		if len(parcalar) == 1 && !dizin {
			kokDosyaVar = true // kökte dosya var → tek kök klasör yok
		}
		if !dizin {
			taban := strings.ToLower(parcalar[len(parcalar)-1])
			if isaret[taban] {
				dizinYolu := strings.Join(parcalar[:len(parcalar)-1], "/")
				mevcut := oz.Isaretler[taban]
				if len(mevcut) < ozetIsaretSiniri && !icerirMi(mevcut, dizinYolu) {
					oz.Isaretler[taban] = append(mevcut, dizinYolu)
				}
			}
		}
		return nil
	}

	if err := uyeleriGez(archivePath, tur, ekle); err != nil {
		return Ozet{}, err
	}
	sort.Strings(oz.Kokler)
	if len(oz.Kokler) == 1 && !kokDosyaVar {
		oz.KokKlasor = oz.Kokler[0]
	}
	return oz, nil
}

func icerirMi(liste []string, v string) bool {
	for _, s := range liste {
		if s == v {
			return true
		}
	}
	return false
}

// uyeleriGez: arşiv üyelerini türe göre dolaşır ve her biri için fn çağırır.
// Tehlikeli ÜYE TİPLERİ (symlink/hardlink/aygıt) burada reddedilir; ad
// doğrulaması fn'e bırakılır (hem Tara hem Ozetle aynı kuralı uygular).
func uyeleriGez(archivePath string, tur Tur, fn func(ad string, boyut int64, dizin bool) error) error {
	switch tur {
	case TurZip:
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return fmt.Errorf("zip okuma: %w", err)
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Mode()&os.ModeSymlink != 0 {
				return ErrUyeSymlink
			}
			if err := fn(f.Name, int64(f.UncompressedSize64), f.FileInfo().IsDir()); err != nil {
				return err
			}
		}
		return nil
	case TurTar, TurTarGz, TurTarBz2, TurTarXz:
		return tarGez(archivePath, tur, fn)
	case TurRar:
		tool, ok := rarAraci()
		if !ok {
			return ErrRarAraciYok
		}
		names, err := rarUyeAdlari(tool, archivePath)
		if err != nil {
			return err
		}
		for _, n := range names {
			// RAR listesinde boyut/dizin bilgisi araca göre değişir; ad yeterli.
			if err := fn(n, 0, strings.HasSuffix(n, "/")); err != nil {
				return err
			}
		}
		return nil
	default:
		return ErrDesteklenmeyen
	}
}

// runuserKomut: argv'yi tenant kullanıcısı (sk) olarak, panel sırları OLMADAN,
// temiz env ile çalıştıracak komutu hazırlar (panelin composer/git/redis deseni).
func runuserKomut(sk string, argv ...string) *exec.Cmd {
	full := append([]string{"-u", sk, "--"}, argv...)
	cmd := exec.Command("runuser", full...)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/home/" + sk,
	}
	return cmd
}

// GuvenliCikar: arşivi destDir içine, tenant kullanıcısı sk olarak, üye-yollarını
// doğrulayarak güvenli biçimde çıkarır (çift savunma).
//
// Önkoşul: destDir sk tarafından yazılabilir olmalı (çağıran chown etmelidir).
// Dönüş: aracın birleşik çıktısı (hata mesajı için) ve hata.
//
// tar ailesi için arşiv baytları stdin üzerinden akıtılır; böylece root-sahipli
// arşivler (örn. yedek deposu) bile tenant kullanıcısına okutulmadan çıkarılabilir.
func GuvenliCikar(archivePath, destDir, sk string) (string, error) {
	return GuvenliCikarStrip(archivePath, destDir, sk, 0)
}

// ErrStripDesteklenmiyor: zip/rar için bileşen atlama bsdtar (libarchive)
// gerektirir; sistemde yoksa kullanıcıya arşivi kök klasörsüz hazırlaması
// söylenmelidir.
var ErrStripDesteklenmiyor = errors.New(
	"kök klasör atlama için bsdtar (libarchive) gerekli — sunucuda kurulu değil")

// StripDesteklenirMi: zip/rar arşivlerinde bileşen atlama mümkün mü?
// (tar ailesinde her zaman mümkündür — GNU tar --strip-components.)
func StripDesteklenirMi() bool {
	_, err := exec.LookPath("bsdtar")
	return err == nil
}

// GuvenliCikarStrip: GuvenliCikar + üye yollarının ilk `strip` bileşenini atlar.
//
// Neden gerekli: başka panellerden alınan yedekler neredeyse her zaman tek bir
// kapsayıcı klasör içerir (public_html/, htdocs/<domain>/ …). Düz çıkarma
// public_html/public_html/index.php üretir ve site açılmaz.
//
// strip=0 davranışı GuvenliCikar ile birebir aynıdır. strip>0'da zip/rar için
// unzip/unrar bileşen atlayamadığından bsdtar kullanılır; bsdtar aynı arşivleri
// okur ve kendisi de ".."/mutlak yolu reddeder, yani her iki güvenlik katmanı
// (Go stdlib ön-tarama + tenant DAC) korunur.
func GuvenliCikarStrip(archivePath, destDir, sk string, strip int) (string, error) {
	tur := TuruBelirle(archivePath)
	if tur == TurBilinmeyen {
		return "", ErrDesteklenmeyen
	}
	if !adlar.SKGecerli(sk) {
		return "", errors.New("güvenlik: geçersiz tenant kullanıcısı")
	}
	if strip < 0 {
		return "", errors.New("güvenlik: negatif strip")
	}

	// Katman 2: üye ön-taraması (jail-dışı / symlink / hardlink reddi).
	if err := Tara(archivePath, tur); err != nil {
		return "", err
	}

	// zip/rar + strip → bsdtar'a devret (unzip/unrar bileşen atlayamaz).
	if strip > 0 && (tur == TurZip || tur == TurRar) {
		if _, err := exec.LookPath("bsdtar"); err != nil {
			return "", ErrStripDesteklenmiyor
		}
		cmd := runuserKomut(sk, "bsdtar", "-x",
			"--strip-components="+strconv.Itoa(strip),
			"-f", archivePath, "-C", destDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("çıkarma (tenant=%s): %w", sk, err)
		}
		return string(out), nil
	}

	// Katman 1: tenant-user (DAC) altında çıkar.
	var cmd *exec.Cmd
	switch tur {
	case TurZip:
		// unzip stdin okuyamaz; arşiv sk-okunur olmalı (tenant home'undaki dosya).
		cmd = runuserKomut(sk, "unzip", "-o", "-q", archivePath, "-d", destDir)
	case TurRar:
		// RAR: seçilen açıcıyı tenant kimliğinde çalıştır (tam-yol koru, üzerine yaz).
		tool, ok := rarAraci()
		if !ok {
			return "", ErrRarAraciYok
		}
		switch tool {
		case "bsdtar":
			// libarchive: RAR/RAR5 okur, -C hedef; kendisi de ".."/mutlak yolu reddeder.
			cmd = runuserKomut(sk, "bsdtar", "-x", "-f", archivePath, "-C", destDir)
		case "unar":
			// -f: üzerine yaz, -D: kapsayıcı dizin oluşturma, -o: hedef.
			cmd = runuserKomut(sk, "unar", "-f", "-D", "-o", destDir, archivePath)
		default: // unrar
			// x: tam-yol çıkar, -o+: üzerine yaz, hedef sonuna / şart.
			cmd = runuserKomut(sk, "unrar", "x", "-o+", archivePath, destDir+"/")
		}
	default:
		// tar ailesi: root arşivi açar, baytlar tenant tar'a stdin'den akar.
		f, err := os.Open(archivePath)
		if err != nil {
			return "", fmt.Errorf("arşiv aç: %w", err)
		}
		defer f.Close()
		flag := "-x"
		switch tur {
		case TurTarGz:
			flag = "-xz"
		case TurTarBz2:
			flag = "-xj"
		case TurTarXz:
			flag = "-xJ"
		}
		argv := []string{"tar", flag, "-f", "-", "-C", destDir}
		if strip > 0 {
			argv = append(argv, "--strip-components="+strconv.Itoa(strip))
		}
		cmd = runuserKomut(sk, argv...)
		cmd.Stdin = f
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("çıkarma (tenant=%s): %w", sk, err)
	}
	return string(out), nil
}
