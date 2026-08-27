// Package nextcloud, apps.Uygulama arayüzünün Nextcloud implementasyonudur.
package nextcloud

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sanalcp/internal/apps"
	"sanalcp/internal/hesaplar"
)

func init() { apps.Kaydet(Surucu{}) }

type Surucu struct{}

func (Surucu) Slug() string                     { return "nextcloud" }
func (Surucu) Ad() string                       { return "Nextcloud" }
func (Surucu) DBOnEki() string                  { return "nextcloud" }
func (Surucu) MarkerDosya() string              { return "config/config.php" }
func (Surucu) GuncelleDesteklenir() bool        { return false }
func (Surucu) MinimumPHPSurum() string          { return "8.2" }
func (Surucu) MaksimumPHPSurum() string         { return "8.5" }
func (Surucu) KurulumZamanAsimi() time.Duration { return 15 * time.Minute }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)['"]dbname['"]\s*=>\s*['"]([^'"]+)['"]`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "config", "config.php"))
	if err != nil {
		return "", false
	}
	for _, m := range reDBAdi.FindAllSubmatch(b, -1) {
		ad := string(m[1])
		if hesaplar.GecerliDBKimlik(ad) && strings.HasPrefix(ad, "nextcloud_") {
			return ad, true
		}
	}
	return "", false
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (sonuc apps.KurulumSonuc, err error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return sonuc, err
	}
	veriDizini := veriDiziniYolu(i.SK, i.Hedef)
	if err := os.Mkdir(veriDizini, 0o750); err != nil {
		return sonuc, fmt.Errorf("Nextcloud veri dizini oluşturulamadı: %w", err)
	}
	basarili := false
	defer func() {
		if !basarili {
			_ = os.RemoveAll(veriDizini)
			_ = os.Remove(cronDosyasiYolu(i.SK, i.Hedef))
		}
	}()
	if out, chownErr := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef, veriDizini).CombinedOutput(); chownErr != nil {
		return sonuc, komutHatasi("Nextcloud dosya izinleri", out, chownErr)
	}
	adminParola := hesaplar.RandomParola(20)
	out, err := occ(ctx, i.SK, i.Hedef, "maintenance:install", "--no-interaction",
		"--database=mysql", "--database-host=localhost", "--database-name="+i.DBAdi,
		"--database-user="+i.DBKullanici, "--database-pass="+i.DBParola,
		"--admin-user="+i.Alanlar["admin_kullanici"], "--admin-pass="+adminParola,
		"--admin-email="+i.Alanlar["admin_email"], "--data-dir="+veriDizini)
	if err != nil {
		return sonuc, komutHatasi("Nextcloud kurulumu", out, err)
	}
	ayarlar := [][]string{
		{"config:system:set", "trusted_domains", "1", "--value=" + i.AlanAdi},
		{"config:system:set", "overwrite.cli.url", "--value=" + i.URL},
		{"background:cron"},
	}
	for _, args := range ayarlar {
		if out, err = occ(ctx, i.SK, i.Hedef, args...); err != nil {
			return sonuc, komutHatasi("Nextcloud yapılandırması", out, err)
		}
	}
	if err := cronKur(i.SK, i.Hedef); err != nil {
		return sonuc, err
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "config", "config.php")); err != nil {
		return sonuc, fmt.Errorf("Nextcloud kurulumu config.php oluşturmadı")
	}
	basarili = true
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/index.php/login",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: surum,
		Ekstra: map[string]string{"veri_dizini": veriDizini}}, nil
}

func cronDosyasiYolu(sk, hedef string) string {
	ozet := sha256.Sum256([]byte(filepath.Clean(hedef)))
	return fmt.Sprintf("/etc/cron.d/sanalcp-nextcloud-%s-%x", sk, ozet[:6])
}

func cronKur(sk, hedef string) error {
	icerik := fmt.Sprintf("*/5 * * * * %s /usr/bin/php %s/cron.php >/dev/null 2>&1\n", sk, filepath.Clean(hedef))
	if err := os.WriteFile(cronDosyasiYolu(sk, hedef), []byte(icerik), 0o644); err != nil {
		return fmt.Errorf("Nextcloud cron kaydı oluşturulamadı: %w", err)
	}
	return nil
}

func (Surucu) SilmeOncesi(_ context.Context, sk, dizin string) error {
	beklenen := veriDiziniYolu(sk, dizin)
	b, err := os.ReadFile(filepath.Join(dizin, "config", "config.php"))
	if err != nil {
		return fmt.Errorf("Nextcloud yapılandırması okunamadı: %w", err)
	}
	re := regexp.MustCompile(`(?m)['"]datadirectory['"]\s*=>\s*['"]([^'"]+)['"]`)
	m := re.FindSubmatch(b)
	if m == nil || filepath.Clean(string(m[1])) != beklenen {
		return fmt.Errorf("veri dizini beklenen güvenli yolla eşleşmiyor")
	}
	if err := os.RemoveAll(beklenen); err != nil {
		return fmt.Errorf("Nextcloud veri dizini silinemedi: %w", err)
	}
	if err := os.Remove(cronDosyasiYolu(sk, dizin)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Nextcloud cron kaydı silinemedi: %w", err)
	}
	return nil
}

func veriDiziniYolu(sk, hedef string) string {
	ozet := sha256.Sum256([]byte(filepath.Clean(hedef)))
	return fmt.Sprintf("/home/%s/.sanalcp-nextcloud-data-%x", sk, ozet[:6])
}

func occ(ctx context.Context, sk, dizin string, args ...string) ([]byte, error) {
	full := []string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk,
		"/usr/bin/php", filepath.Join(dizin, "occ")}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	cmd.Dir = dizin
	return cmd.CombinedOutput()
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, rawURL string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: rawURL, AdminURL: rawURL + "/index.php/login", Durum: "bilinmiyor", Surum: yerelSurumOku(dizin)}
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "config", "config.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("Nextcloud güncellemesi desteklenmiyor")
}

func komutHatasi(asama string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if len(msg) > 600 {
		msg = msg[len(msg)-600:]
	}
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s: %s", asama, msg)
}

var reSurum = regexp.MustCompile(`(?m)^\s*\$OC_VersionString\s*=\s*['"]([0-9]+\.[0-9]+\.[0-9]+)['"]\s*;`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "version.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
