// Package matomo, apps.Uygulama arayüzünün Matomo implementasyonudur.
package matomo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"sanalcp/internal/apps"
	"sanalcp/internal/hesaplar"
)

func init() { apps.Kaydet(Surucu{}) }

type Surucu struct{}

func (Surucu) Slug() string              { return "matomo" }
func (Surucu) Ad() string                { return "Matomo" }
func (Surucu) DBOnEki() string           { return "matomo" }
func (Surucu) MarkerDosya() string       { return "config/config.ini.php" }
func (Surucu) GuncelleDesteklenir() bool { return false }
func (Surucu) MinimumPHPSurum() string   { return "8.1" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
		{Anahtar: "izlenen_site_adi", Etiket: "İzlenecek Site Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "izlenen_site_url", Etiket: "İzlenecek Site URL", Tur: "text", Zorunlu: true, YerTutucu: "https://example.com"},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)^dbname\s*=\s*["']?([^"'\s;]+)["']?\s*$`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "config", "config.ini.php"))
	if err != nil {
		return "", false
	}
	m := reDBAdi.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	ad := string(m[1])
	if !hesaplar.GecerliDBKimlik(ad) || !strings.HasPrefix(ad, "matomo_") {
		return "", false
	}
	return ad, true
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("Matomo dosya izinleri", out, err)
	}
	adminParola := hesaplar.RandomParola(18)
	if err := webKur(ctx, i, adminParola); err != nil {
		return apps.KurulumSonuc{}, err
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "config", "config.ini.php")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("Matomo kurulumu config.ini.php oluşturmadı")
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/index.php",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: surum}, nil
}

func webKur(ctx context.Context, i apps.KurulumIstek, adminParola string) error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("Matomo kurulum portu ayrılamadı: %w", err)
	}
	adres := l.Addr().String()
	_ = l.Close()
	var log bytes.Buffer
	cmd := exec.CommandContext(ctx, "runuser", "-u", i.SK, "--", "env", "HOME=/home/"+i.SK,
		"TMPDIR=/home/"+i.SK, "/usr/bin/php", "-S", adres, "-t", i.Hedef)
	cmd.Stdout, cmd.Stderr = &log, &log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Matomo geçici kurulum sunucusu başlatılamadı: %w", err)
	}
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 45 * time.Second}
	taban := "http://" + adres + "/index.php?module=Installation&action="
	var hazir bool
	for n := 0; n < 50; n++ {
		resp, getErr := client.Get(taban + "welcome")
		if getErr == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 2<<20))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				hazir = true
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !hazir {
		return fmt.Errorf("Matomo kurulum sunucusu hazır olmadı: %s", sonLog(log.String()))
	}

	adimlar := []struct {
		ad        string
		form      url.Values
		kalanForm string
	}{
		{"databaseSetup", url.Values{"host": {"localhost"}, "username": {i.DBKullanici}, "password": {i.DBParola},
			"dbname": {i.DBAdi}, "tables_prefix": {"matomo_"}, "adapter": {"PDO\\MYSQL"}, "schema": {"Mariadb"}, "type": {"InnoDB"}, "submit": {"1"}}, "databasesetupform"},
		{"setupSuperUser", url.Values{"login": {i.Alanlar["admin_kullanici"]}, "email": {i.Alanlar["admin_email"]},
			"password": {adminParola}, "password_bis": {adminParola}, "submit": {"1"}}, "generalsetupform"},
		{"firstWebsiteSetup", url.Values{"siteName": {i.Alanlar["izlenen_site_adi"]}, "url": {i.Alanlar["izlenen_site_url"]},
			"timezone": {"Europe/Istanbul"}, "ecommerce": {"0"}, "submit": {"1"}}, "websitesetupform"},
		{"finished", url.Values{"submit": {"1"}}, ""},
	}
	for _, a := range adimlar {
		resp, err := client.PostForm(taban+a.ad, a.form)
		if err != nil {
			return fmt.Errorf("Matomo %s adımı: %w", a.ad, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("Matomo %s yanıtı okunamadı: %w", a.ad, readErr)
		}
		if resp.StatusCode != http.StatusOK || (a.kalanForm != "" && bytes.Contains(body, []byte(`id="`+a.kalanForm+`"`))) {
			return fmt.Errorf("Matomo %s adımı tamamlanamadı (HTTP %d)", a.ad, resp.StatusCode)
		}
	}
	return nil
}

func sonLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		s = s[len(s)-600:]
	}
	return s
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, rawURL string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: rawURL, AdminURL: rawURL + "/index.php", Durum: "bilinmiyor", Surum: yerelSurumOku(dizin)}
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "config", "config.ini.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("Matomo güncellemesi desteklenmiyor")
}

func komutHatasi(asama string, out []byte, err error) error {
	msg := sonLog(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s: %s", asama, msg)
}

var reSurum = regexp.MustCompile(`(?m)public const VERSION\s*=\s*['"]([0-9]+\.[0-9]+\.[0-9]+)['"]\s*;`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "core", "Version.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}
