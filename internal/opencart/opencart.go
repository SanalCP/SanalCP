// Package opencart, apps.Uygulama arayüzünün OpenCart implementasyonudur.
package opencart

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"sanalcp/internal/apps"
	"sanalcp/internal/hesaplar"
)

func init() { apps.Kaydet(Surucu{}) }

type Surucu struct{}

func (Surucu) Slug() string              { return "opencart" }
func (Surucu) Ad() string                { return "OpenCart" }
func (Surucu) DBOnEki() string           { return "opencart" }
func (Surucu) MarkerDosya() string       { return "system/framework.php" }
func (Surucu) GuncelleDesteklenir() bool { return false }
func (Surucu) MinimumPHPSurum() string   { return "8.1" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)define\s*\(\s*['"]DB_DATABASE['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "config.php"))
	if err != nil {
		return "", false
	}
	m := reDBAdi.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	ad := string(m[1])
	if !hesaplar.GecerliDBKimlik(ad) || !strings.HasPrefix(ad, "opencart_") {
		return "", false
	}
	return ad, true
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	for _, cift := range [][2]string{{"config-dist.php", "config.php"}, {"admin/config-dist.php", "admin/config.php"}} {
		if err := os.Rename(filepath.Join(i.Hedef, cift[0]), filepath.Join(i.Hedef, cift[1])); err != nil {
			return apps.KurulumSonuc{}, fmt.Errorf("OpenCart yapılandırma şablonu hazırlanamadı: %w", err)
		}
	}
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("OpenCart dosya izinleri", out, err)
	}
	adminParola := hesaplar.RandomParola(18)
	args := []string{
		filepath.Join(i.Hedef, "install", "cli_install.php"), "install",
		"--username", i.Alanlar["admin_kullanici"], "--email", i.Alanlar["admin_email"],
		"--password", adminParola, "--http_server", strings.TrimRight(i.URL, "/") + "/",
		"--language", "en-gb", "--db_driver", "mysqli", "--db_hostname", "localhost",
		"--db_username", i.DBKullanici, "--db_password", i.DBParola,
		"--db_database", i.DBAdi, "--db_port", "3306", "--db_prefix", "oc_",
	}
	out, err := phpKomut(ctx, i.SK, args...)
	if err != nil || !strings.Contains(string(out), "SUCCESS! OpenCart successfully installed") {
		if err == nil {
			err = fmt.Errorf("başarı yanıtı alınamadı")
		}
		return apps.KurulumSonuc{}, komutHatasi("OpenCart kurulumu", out, err)
	}
	if err := os.RemoveAll(filepath.Join(i.Hedef, "install")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("OpenCart install dizini kaldırılamadı: %w", err)
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/admin",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: surum}, nil
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, url string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: url, AdminURL: url + "/admin", Durum: "bilinmiyor"}
	k.Surum = yerelSurumOku(dizin)
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "config.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("OpenCart güncellemesi desteklenmiyor")
}

func phpKomut(ctx context.Context, sk string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk, "/usr/bin/php"}, args...)
	return exec.CommandContext(ctx, "runuser", full...).CombinedOutput()
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

var reSurum = regexp.MustCompile(`(?m)define\s*\(\s*['"]VERSION['"]\s*,\s*['"]([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)['"]\s*\)`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "index.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
