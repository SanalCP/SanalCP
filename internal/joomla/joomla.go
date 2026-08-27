// Package joomla, apps.Uygulama arayüzünün Joomla implementasyonudur.
package joomla

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

func (Surucu) Slug() string              { return "joomla" }
func (Surucu) Ad() string                { return "Joomla" }
func (Surucu) DBOnEki() string           { return "joomla" }
func (Surucu) MarkerDosya() string       { return "configuration.php" }
func (Surucu) GuncelleDesteklenir() bool { return true }
func (Surucu) MinimumPHPSurum() string   { return "8.3" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "site_adi", Etiket: "Site Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_ad", Etiket: "Yönetici Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)public\s+\$db\s*=\s*['"]([^'"]+)['"]\s*;`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "configuration.php"))
	if err != nil {
		return "", false
	}
	m := reDBAdi.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	ad := string(m[1])
	if !hesaplar.GecerliDBKimlik(ad) || !strings.HasPrefix(ad, "joomla_") {
		return "", false
	}
	return ad, true
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	adminParola := hesaplar.RandomParola(18)
	args := []string{
		filepath.Join(i.Hedef, "installation", "joomla.php"), "install",
		"--site-name=" + i.Alanlar["site_adi"],
		"--admin-user=" + i.Alanlar["admin_ad"],
		"--admin-username=" + i.Alanlar["admin_kullanici"],
		"--admin-password=" + adminParola,
		"--admin-email=" + i.Alanlar["admin_email"],
		"--db-type=mysqli", "--db-host=localhost",
		"--db-user=" + i.DBKullanici, "--db-pass=" + i.DBParola,
		"--db-name=" + i.DBAdi, "--db-prefix=jml_", "--db-encryption=0",
		"--no-interaction", "--no-ansi",
	}
	out, err := komut(ctx, i.SK, args...)
	if err != nil {
		return apps.KurulumSonuc{}, komutHatasi("Joomla kurulumu", out, err)
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "configuration.php")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("Joomla kurulumu configuration.php oluşturmadı")
	}
	return apps.KurulumSonuc{
		SiteURL: i.URL, AdminURL: i.URL + "/administrator",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola,
		Surum: surum,
	}, nil
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, url string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: url, AdminURL: url + "/administrator", Durum: "bilinmiyor"}
	k.Surum = yerelSurumOku(dizin)
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		switch {
		case k.Surum == "":
			k.Durum = "bilinmiyor"
		case k.Surum == rel.Surum:
			k.Durum = "guncel"
		default:
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "configuration.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(ctx context.Context, sk, dizin string) error {
	out, err := komut(ctx, sk, filepath.Join(dizin, "cli", "joomla.php"),
		"core:update", "--no-interaction", "--no-ansi")
	if err != nil {
		return komutHatasi("Joomla güncelleme", out, err)
	}
	return nil
}

func komut(ctx context.Context, sk string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk,
		"TMPDIR=/home/" + sk, "/usr/bin/php"}, args...)
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

var (
	reMajor = regexp.MustCompile(`(?m)public const MAJOR_VERSION\s*=\s*([0-9]+)\s*;`)
	reMinor = regexp.MustCompile(`(?m)public const MINOR_VERSION\s*=\s*([0-9]+)\s*;`)
	rePatch = regexp.MustCompile(`(?m)public const PATCH_VERSION\s*=\s*([0-9]+)\s*;`)
)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "libraries", "src", "Version.php"))
	if err != nil {
		return ""
	}
	ma, mi, pa := reMajor.FindSubmatch(b), reMinor.FindSubmatch(b), rePatch.FindSubmatch(b)
	if ma == nil || mi == nil || pa == nil {
		return ""
	}
	return string(ma[1]) + "." + string(mi[1]) + "." + string(pa[1])
}
