// Package phpbb, apps.Uygulama arayüzünün phpBB implementasyonudur.
package phpbb

import (
	"context"
	"encoding/json"
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

func (Surucu) Slug() string              { return "phpbb" }
func (Surucu) Ad() string                { return "phpBB" }
func (Surucu) DBOnEki() string           { return "phpbb" }
func (Surucu) MarkerDosya() string       { return "phpbb/class_loader.php" }
func (Surucu) GuncelleDesteklenir() bool { return false }
func (Surucu) MinimumPHPSurum() string   { return "7.4" }
func (Surucu) MaksimumPHPSurum() string  { return "8.4" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "forum_adi", Etiket: "Forum Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)^\s*\$db_name\s*=\s*['"]([^'"]+)['"]\s*;`)

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
	return ad, hesaplar.GecerliDBKimlik(ad) && strings.HasPrefix(ad, "phpbb_")
}

func yamlDeger(s string) string { b, _ := json.Marshal(s); return string(b) }

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	if err := indirVeDogrula(ctx, i.Hedef); err != nil {
		return apps.KurulumSonuc{}, err
	}
	u, err := url.Parse(i.URL)
	if err != nil || u.Hostname() == "" {
		return apps.KurulumSonuc{}, fmt.Errorf("phpBB site URL'si geçersiz")
	}
	adminParola := hesaplar.RandomParola(20)
	protokol := u.Scheme + "://"
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	scriptPath := strings.TrimRight(u.EscapedPath(), "/")
	if scriptPath == "" {
		scriptPath = "/"
	}
	yaml := fmt.Sprintf(`installer:
    admin:
        name: %s
        password: %s
        email: %s
    board:
        lang: en
        name: %s
        description: ""
    database:
        dbms: mysqli
        dbhost: localhost
        dbport: ""
        dbuser: %s
        dbpasswd: %s
        dbname: %s
        table_prefix: phpbb_
    email:
        enabled: false
    server:
        cookie_secure: %t
        server_protocol: %s
        force_server_vars: true
        server_name: %s
        server_port: %s
        script_path: %s
    extensions: []
`, yamlDeger(i.Alanlar["admin_kullanici"]), yamlDeger(adminParola), yamlDeger(i.Alanlar["admin_email"]),
		yamlDeger(i.Alanlar["forum_adi"]), yamlDeger(i.DBKullanici), yamlDeger(i.DBParola), yamlDeger(i.DBAdi),
		i.SSL, yamlDeger(protokol), yamlDeger(u.Hostname()), yamlDeger(port), yamlDeger(scriptPath))
	konfig := filepath.Join(i.Hedef, "install", "sanalcp-install.yml")
	if err := os.WriteFile(konfig, []byte(yaml), 0o600); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("phpBB kurulum yapılandırması yazılamadı: %w", err)
	}
	defer os.Remove(konfig)
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("phpBB dosya izinleri", out, err)
	}
	out, err := phpKomut(ctx, i.SK, i.Hedef, "install/phpbbcli.php", "install", "install/sanalcp-install.yml", "--no-interaction")
	if err != nil {
		return apps.KurulumSonuc{}, komutHatasi("phpBB kurulumu", out, err)
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "config.php")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("phpBB kurulumu config.php oluşturmadı")
	}
	if err := os.RemoveAll(filepath.Join(i.Hedef, "install")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("phpBB install dizini kaldırılamadı: %w", err)
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/adm/index.php",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: sabitSurum}, nil
}

func phpKomut(ctx context.Context, sk, dizin string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk, "/usr/bin/php"}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	cmd.Dir = dizin
	return cmd.CombinedOutput()
}

func (Surucu) Bilgi(_ context.Context, _, dizin, rawURL string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: rawURL, AdminURL: rawURL + "/adm/index.php", Surum: yerelSurumOku(dizin), SonSurum: sabitSurum, Durum: "bilinmiyor"}
	if k.Surum == sabitSurum {
		k.Durum = "guncel"
	} else if k.Surum != "" {
		k.Durum = "eski"
	}
	if fi, err := os.Stat(filepath.Join(dizin, "config.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("phpBB güncellemesi desteklenmiyor")
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

var reSurum = regexp.MustCompile(`(?m)^\s*@?define\s*\(\s*['"]PHPBB_VERSION['"]\s*,\s*['"]([0-9]+\.[0-9]+\.[0-9]+)['"]\s*\)`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "includes", "constants.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
