// Package mediawiki, apps.Uygulama arayüzünün MediaWiki implementasyonudur.
package mediawiki

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

func init() { apps.Kaydet(Surucu{}) }

type Surucu struct{}

func (Surucu) Slug() string              { return "mediawiki" }
func (Surucu) Ad() string                { return "MediaWiki" }
func (Surucu) DBOnEki() string           { return "mediawiki" }
func (Surucu) MarkerDosya() string       { return "LocalSettings.php" }
func (Surucu) GuncelleDesteklenir() bool { return false }
func (Surucu) MinimumPHPSurum() string   { return "8.3" }
func (Surucu) MaksimumPHPSurum() string  { return "8.5" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "wiki_adi", Etiket: "Wiki Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)^\s*\$wgDBname\s*=\s*["']([^"']+)["']\s*;`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "LocalSettings.php"))
	if err != nil {
		return "", false
	}
	m := reDBAdi.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	ad := string(m[1])
	return ad, hesaplar.GecerliDBKimlik(ad) && strings.HasPrefix(ad, "mediawiki_")
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	if err := indirVeDogrula(ctx, i.Hedef); err != nil {
		return apps.KurulumSonuc{}, err
	}
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("MediaWiki dosya izinleri", out, err)
	}
	u, err := url.Parse(i.URL)
	if err != nil || u.Host == "" {
		return apps.KurulumSonuc{}, fmt.Errorf("MediaWiki site URL'si geçersiz")
	}
	scriptPath := strings.TrimRight(u.EscapedPath(), "/")
	adminParola := hesaplar.RandomParola(20)
	args := []string{"maintenance/run.php", "install", "--quiet", "--lang=tr",
		"--dbtype=mysql", "--dbserver=localhost", "--dbname=" + i.DBAdi,
		"--dbuser=" + i.DBKullanici, "--dbpass=" + i.DBParola,
		"--installdbuser=" + i.DBKullanici, "--installdbpass=" + i.DBParola,
		"--server=" + u.Scheme + "://" + u.Host, "--scriptpath=" + scriptPath,
		"--pass=" + adminParola, i.Alanlar["wiki_adi"], i.Alanlar["admin_kullanici"]}
	out, err := phpKomut(ctx, i.SK, i.Hedef, args...)
	if err != nil {
		return apps.KurulumSonuc{}, komutHatasi("MediaWiki kurulumu", out, err)
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "LocalSettings.php")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("MediaWiki kurulumu LocalSettings.php oluşturmadı")
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/index.php?title=Special:UserLogin",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: sabitSurum}, nil
}

func phpKomut(ctx context.Context, sk, dizin string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk, "/usr/bin/php"}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	cmd.Dir = dizin
	return cmd.CombinedOutput()
}

func (Surucu) Bilgi(_ context.Context, _, dizin, rawURL string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: rawURL, AdminURL: rawURL + "/index.php?title=Special:UserLogin",
		Surum: yerelSurumOku(dizin), SonSurum: sabitSurum, Durum: "bilinmiyor"}
	if k.Surum == sabitSurum {
		k.Durum = "guncel"
	} else if k.Surum != "" {
		k.Durum = "eski"
	}
	if fi, err := os.Stat(filepath.Join(dizin, "LocalSettings.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("MediaWiki güncellemesi desteklenmiyor")
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

var reSurum = regexp.MustCompile(`(?m)^\s*define\s*\(\s*['"]MW_VERSION['"]\s*,\s*['"]([0-9]+\.[0-9]+\.[0-9]+)['"]\s*\)`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "includes", "Defines.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
