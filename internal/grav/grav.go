// Package grav, apps.Uygulama arayüzünün Grav implementasyonudur.
package grav

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

func (Surucu) Slug() string                   { return "grav" }
func (Surucu) Ad() string                     { return "Grav" }
func (Surucu) DBOnEki() string                { return "grav" }
func (Surucu) MarkerDosya() string            { return "system/defines.php" }
func (Surucu) GuncelleDesteklenir() bool      { return true }
func (Surucu) MinimumPHPSurum() string        { return "8.3" }
func (Surucu) VeritabaniGerekli() bool        { return false }
func (Surucu) DBAdiOku(string) (string, bool) { return "", false }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "admin_ad", Etiket: "Yönetici Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("Grav dosya izinleri", out, err)
	}
	adminParola := hesaplar.RandomParola(18)
	out, err := phpKomut(ctx, i.SK, i.Hedef, "bin/plugin", "login", "new-user",
		"--user="+i.Alanlar["admin_kullanici"], "--password="+adminParola,
		"--email="+i.Alanlar["admin_email"], "--fullname="+i.Alanlar["admin_ad"],
		"--permissions=a", "--admin-type=api", "--state=enabled", "--language=tr", "--no-interaction", "--no-ansi")
	if err != nil || !strings.Contains(string(out), "Success!") {
		if err == nil {
			err = fmt.Errorf("başarı yanıtı alınamadı")
		}
		return apps.KurulumSonuc{}, komutHatasi("Grav yönetici oluşturma", out, err)
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/admin",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: surum}, nil
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, rawURL string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: rawURL, AdminURL: rawURL + "/admin", Durum: "bilinmiyor", Surum: yerelSurumOku(dizin)}
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "system", "defines.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(ctx context.Context, sk, dizin string) error {
	out, err := phpKomut(ctx, sk, dizin, "bin/gpm", "selfupgrade", "--all-yes", "--no-interaction", "--no-ansi")
	if err != nil {
		return komutHatasi("Grav güncelleme", out, err)
	}
	return nil
}

func phpKomut(ctx context.Context, sk, dizin string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk, "/usr/bin/php"}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	cmd.Dir = dizin
	return cmd.CombinedOutput()
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

var reSurum = regexp.MustCompile(`(?m)define\s*\(\s*["']GRAV_VERSION["']\s*,\s*["']([0-9]+\.[0-9]+\.[0-9]+)["']\s*\)`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "system", "defines.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
