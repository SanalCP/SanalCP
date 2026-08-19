// Package iceaktarim: panel-bağımsız ("genel") içe aktarma.
//
// cPanel tam yedeği (internal/transfers) tek bir üreticiye özgüdür ve yeni bir
// domain OLUŞTURUR. Bu paket ise MEVCUT bir domaine, HANGİ panelden geldiği
// önemli olmadan, iki bağımsız parçayı aktarır:
//
//  1. Site dosyaları — zip/tar/tar.gz/tar.bz2/tar.xz/rar arşivi → public_html
//  2. Veritabanı     — .sql veya .sql.gz dump → domainin kendi veritabanı
//  3. (opsiyonel) Uygulama yapılandırması — wp-config.php / .env içindeki DB
//     bilgilerini hedefteki yeni değerlerle günceller.
//
// GÜVENLİK — bu paketin dokunduğu iki tehlikeli yüzey ve nasıl kapatıldığı:
//
//	a) Arşiv çıkarma root olarak yapılırsa jail kaçışı olur. Çıkarma
//	   internal/archivex ile yapılır: üye ön-taraması (mutlak yol/".."/symlink
//	   reddi) + `runuser -u <tenant>` (DAC). Hedef dizinin kendisi ise
//	   internal/jailpath ile symlink-güvenli açılır/oluşturulur; "önce temizle"
//	   seçeneği jailpath.IceriginiSil (fd-göreli, TOCTOU'ya kapalı) kullanır —
//	   os.RemoveAll ile YAPILMAZ (bkz. 0.4.0'daki kritik düzeltme).
//
//	b) SQL dump'ı MySQL root'una akıtmak DB sunucusunun tamamını verir.
//	   internal/sqlimport dump'ı hedef veritabanının KENDİ düşük yetkili
//	   kullanıcısıyla uygular (paket açıklamasındaki tehdit modeline bakın).
package iceaktarim

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/jailpath"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sys/unix"

	"sanalcp/internal/osfam"
)

type Handlers struct{ DB *sql.DB }

const (
	// MaxArsivBayt: tek site arşivi için üst sınır (dosya yöneticisiyle aynı).
	MaxArsivBayt = int64(10 << 30)
	// MaxDumpBayt: SQL dump üst sınırı (sıkıştırılmış ve açılmış hâli ayrı ayrı).
	MaxDumpBayt = int64(4 << 30)
	// stagingRel: yüklenen arşivin tenant ev dizinindeki geçici yeri. Tenant'ın
	// kendi diskinde tutulur → kotasına yazılır ve unzip/bsdtar tenant kimliğiyle
	// okuyabilir.
	stagingRel = ".sanalcp-ice-aktarim"
	// stagingOmur: bu yaştan eski staging dosyaları her istekte temizlenir
	// (yarım kalan aktarımlar disk doldurmasın).
	stagingOmur = 24 * time.Hour
)

var (
	errDemo    = errors.New("demo aboneliğinde içe aktarma yapılamaz")
	errBadUser = errors.New("domain için geçerli bir sistem kullanıcısı yok")

	// uzantiDeseni: kabul edilen arşiv uzantıları. Hem yüklemede hem staging
	// kimliği doğrulamasında AYNI liste kullanılır — böylece staging alanına
	// yalnız bu uzantılarla dosya girer ve yalnız bu uzantılarla referans verilebilir.
	uzantiDeseni = `(zip|tar|tar\.gz|tgz|tar\.bz2|tbz2|tar\.xz|txz|rar)`
	reUzantiSon  = regexp.MustCompile(`\.` + uzantiDeseni + `$`)
	reStageID    = regexp.MustCompile(`^[0-9a-f]{32}\.` + uzantiDeseni + `$`)
)

// domain: URL'deki domain id'sinden ev dizinini ve sistem kullanıcısını çözer.
func (h *Handlers) domain(r *http.Request) (id int64, home, sk string, err error) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", "", os.ErrNotExist
	}
	if err != nil {
		return 0, "", "", err
	}
	if isDemo == 1 {
		return 0, "", "", errDemo
	}
	home, err = jailpath.TenantHome(sk)
	if err != nil {
		return 0, "", "", errBadUser
	}
	return id, home, sk, nil
}

// stageYaz: yüklenen arşivi tenant ev dizinindeki staging alanına AKITARAK
// (RAM'e almadan) yazar ve tenant'a devreder. Dönen ad staging kimliğidir.
func stageYaz(home, sk, dosyaAdi string, src io.Reader) (string, int64, error) {
	if err := jailpath.DizinOlustur(home, stagingRel, sk); err != nil {
		return "", 0, fmt.Errorf("staging dizini oluşturulamadı: %w", err)
	}
	uzanti := reUzantiSon.FindString(strings.ToLower(dosyaAdi))
	if uzanti == "" {
		return "", 0, errors.New("desteklenmeyen format (zip, tar, tar.gz/tgz, tar.bz2, tar.xz, rar)")
	}
	ham := make([]byte, 16)
	if _, err := rand.Read(ham); err != nil {
		return "", 0, err
	}
	stageID := hex.EncodeToString(ham) + uzanti
	f, err := jailpath.Ac(home, path.Join(stagingRel, stageID),
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("staging dosyası açılamadı: %w", err)
	}
	defer f.Close()
	if uid, gid, ok := jailpath.TenantIDs(sk); ok {
		_ = unix.Fchown(int(f.Fd()), uid, gid)
	}
	n, err := io.Copy(f, io.LimitReader(src, MaxArsivBayt+1))
	if err != nil {
		_ = jailpath.Sil(home, path.Join(stagingRel, stageID))
		return "", 0, err
	}
	if n > MaxArsivBayt {
		_ = jailpath.Sil(home, path.Join(stagingRel, stageID))
		return "", 0, fmt.Errorf("arşiv %d GiB sınırını aşıyor", MaxArsivBayt>>30)
	}
	return stageID, n, nil
}

// stageYol: staging kimliğini doğrular ve MUTLAK yolunu döndürür.
//
// Kimlik deseni katı (32 hex + bilinen uzantı) olduğu için yol bileşeni
// içeremez; ayrıca dosya jailpath ile açılarak gerçekten staging dizininde ve
// symlink'siz olduğu doğrulanır.
func stageYol(home, stageID string) (string, error) {
	if !reStageID.MatchString(stageID) {
		return "", errors.New("geçersiz yükleme kimliği")
	}
	rel := path.Join(stagingRel, stageID)
	f, err := jailpath.Ac(home, rel, unix.O_RDONLY, 0)
	if err != nil {
		return "", errors.New("yüklenen arşiv bulunamadı (süresi dolmuş olabilir)")
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return "", errors.New("yüklenen arşiv okunamadı")
	}
	return path.Join(home, rel), nil
}

// stageTemizle: staging alanındaki eski dosya/dizinleri siler. Her istekte
// çağrılır; yarım kalan aktarımlar tenant diskinde birikmesin.
func stageTemizle(home string) {
	adlar, err := jailpath.Adlari(home, stagingRel)
	if err != nil {
		return
	}
	simdi := time.Now()
	for _, ad := range adlar {
		rel := path.Join(stagingRel, ad)
		f, err := jailpath.Ac(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		fi, serr := f.Stat()
		f.Close()
		if serr != nil || simdi.Sub(fi.ModTime()) < stagingOmur {
			continue
		}
		_ = jailpath.Sil(home, rel)
	}
}

// sahiplendir: çıkarılan içeriği tenant'a devreder ve SELinux etiketini onarır.
// Her ikisi de "en iyi çaba"dır — kurulumda SELinux kapalı olabilir.
func sahiplendir(mutlakYol, sk string) {
	_, _ = exec.Command("chown", "-R", sk+":"+sk, mutlakYol).CombinedOutput()
	_, _ = exec.Command("restorecon", "-R", mutlakYol).CombinedOutput()
	// Per-user izin modeli: nginx okuma-ACL'i (dosya yöneticisi Extract ile aynı).
	// setfacl yoksa sessiz atlanır.
	if _, err := exec.LookPath("setfacl"); err == nil {
		wk := osfam.WebKullanici() // RHEL nginx · Debian www-data
		_, _ = exec.Command("setfacl", "-R", "-m", "u:"+wk+":rX", mutlakYol).CombinedOutput()
		_, _ = exec.Command("setfacl", "-R", "-d", "-m", "u:"+wk+":rX", mutlakYol).CombinedOutput()
	}
}

func durumKodu(err error) int {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, errDemo):
		return http.StatusForbidden
	case errors.Is(err, errBadUser):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
