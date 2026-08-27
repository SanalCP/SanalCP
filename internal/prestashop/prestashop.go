package prestashop

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"sanalcp/internal/apps"
	"sanalcp/internal/hesaplar"
)

func init() {
	apps.Kaydet(Surucu{})
}

type Surucu struct{}

func (Surucu) Slug() string              { return "prestashop" }
func (Surucu) Ad() string                { return "PrestaShop" }
func (Surucu) DBOnEki() string           { return "prestashop" }
func (Surucu) MarkerDosya() string       { return filepath.Join("config", "settings.inc.php") }
func (Surucu) GuncelleDesteklenir() bool { return false } // spec kararı: resmi CLI güncelleyici yok
func (Surucu) MinimumPHPSurum() string   { return "8.1" }
func (Surucu) MaksimumPHPSurum() string  { return "8.5" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "magaza_adi", Etiket: "Mağaza Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Admin E-posta", Tur: "email", Zorunlu: true},
		{Anahtar: "admin_ad", Etiket: "Admin Ad", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_soyad", Etiket: "Admin Soyad", Tur: "text", Zorunlu: true},
	}
}

var reDBNamePS = regexp.MustCompile(`define\(\s*'_DB_NAME_',\s*'([^']+)'\s*\)`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "config", "settings.inc.php"))
	if err != nil {
		return "", false
	}
	m := reDBNamePS.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	dbName := string(m[1])
	if !hesaplar.GecerliDBKimlik(dbName) || !strings.HasPrefix(dbName, "prestashop_") {
		return "", false
	}
	return dbName, true
}

// psAdminDizinBul: PrestaShop kurulum sonrası admin/ dizinini güvenlik için
// rastgele isimlendirir (adminXXXXXXXXXX/). Stdout ayrıştırmak yerine hedef
// dizini tarayarak buluyoruz — sürüme göre değişebilecek stdout metnine
// bağımlı olmadan güvenilir.
func psAdminDizinBul(hedef string) string {
	entries, err := os.ReadDir(hedef)
	if err != nil {
		return "admin"
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "admin" && strings.HasPrefix(e.Name(), "admin") {
			return e.Name()
		}
	}
	return "admin"
}

var rePSVersion = regexp.MustCompile(`define\(\s*'_PS_VERSION_',\s*'([^']+)'\s*\)`)

// psSurumDosyadanOku: kurulu sürümü config dosyalarından okumaya çalışır
// (dosya adı/konumu sürüme göre değişebilir — best-effort, bulunamazsa "").
func psSurumDosyadanOku(dizin string) string {
	for _, yol := range []string{
		filepath.Join(dizin, "config", "defines.inc.php"),
		filepath.Join(dizin, "config", "settings.inc.php"),
	} {
		b, err := os.ReadFile(yol)
		if err != nil {
			continue
		}
		if m := rePSVersion.FindSubmatch(b); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := psSonSurum(ctx)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	if err := psIndirVeAc(ctx, surum, i.Hedef); err != nil {
		return apps.KurulumSonuc{}, err
	}
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return apps.KurulumSonuc{}, fmt.Errorf("PrestaShop dosya izinleri: %s", msg)
	}

	adminParola := hesaplar.RandomParola(18)
	baseURI := psBaseURI(i.URL)
	ssl := "0"
	if i.SSL {
		ssl = "1"
	}
	args := []string{
		filepath.Join(i.Hedef, "install", "index_cli.php"),
		"--domain=" + i.AlanAdi,
		"--base_uri=" + baseURI,
		"--db_server=localhost",
		"--db_name=" + i.DBAdi,
		"--db_user=" + i.DBKullanici,
		"--db_password=" + i.DBParola,
		"--db_create=0",
		"--prefix=ps_",
		"--name=" + i.Alanlar["magaza_adi"],
		"--email=" + i.Alanlar["admin_email"],
		"--password=" + adminParola,
		"--firstname=" + i.Alanlar["admin_ad"],
		"--lastname=" + i.Alanlar["admin_soyad"],
		"--language=tr",
		"--country=tr",
		"--all_languages=0",
		"--fixtures=0",
		"--ssl=" + ssl,
		"--rewrite=1",
		"--license=0",
	}
	// Resmî PrestaShop 9 CLI sözleşmesi başarıda "Installation successful!"
	// üretir. Substring kontrolü noktalama değişikliklerine toleranslıdır.
	out, err := psKomut(ctx, i.SK, args...)
	if err != nil || !strings.Contains(string(out), "Installation successful") {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return apps.KurulumSonuc{}, fmt.Errorf("PrestaShop kurulum: %s", msg)
	}

	adminDizin := psAdminDizinBul(i.Hedef)
	return apps.KurulumSonuc{
		SiteURL:        i.URL,
		AdminURL:       i.URL + "/" + adminDizin,
		AdminKullanici: i.Alanlar["admin_email"],
		AdminParola:    adminParola,
		Surum:          surum,
		Ekstra:         map[string]string{"admin_dizini": adminDizin},
	}, nil
}

func psBaseURI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return "/"
	}
	p := "/" + strings.Trim(u.EscapedPath(), "/")
	if p == "/" {
		return p
	}
	return p + "/"
}

func (Surucu) Bilgi(ctx context.Context, sk, dizin, url string) (apps.Kurulum, error) {
	adminDizin := psAdminDizinBul(dizin)
	kurulumTarihi := ""
	if fi, err := os.Stat(filepath.Join(dizin, "config", "settings.inc.php")); err == nil {
		kurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	k := apps.Kurulum{
		Surum: psSurumDosyadanOku(dizin), Durum: "bilinmiyor",
		SiteURL: url, AdminURL: url + "/" + adminDizin,
		KurulumTarihi: kurulumTarihi,
	}
	if rel, err := psRelease(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	return k, nil
}

func (Surucu) Guncelle(ctx context.Context, sk, dizin string) error {
	return fmt.Errorf("PrestaShop için otomatik güncelleme desteklenmiyor")
}
