// Package drupal, apps.Uygulama arayüzünün Drupal implementasyonudur.
package drupal

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

func (Surucu) Slug() string              { return "drupal" }
func (Surucu) Ad() string                { return "Drupal" }
func (Surucu) DBOnEki() string           { return "drupal" }
func (Surucu) MarkerDosya() string       { return "sites/default/settings.php" }
func (Surucu) GuncelleDesteklenir() bool { return false }
func (Surucu) MinimumPHPSurum() string   { return "8.3" }
func (Surucu) MaksimumPHPSurum() string  { return "8.5" }

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "site_adi", Etiket: "Site Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Yönetici Kullanıcı Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Yönetici E-posta", Tur: "email", Zorunlu: true},
	}
}

var reDBAdi = regexp.MustCompile(`(?m)['"]database['"]\s*=>\s*['"]([^'"]+)['"]`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "sites", "default", "settings.php"))
	if err != nil {
		return "", false
	}
	for _, m := range reDBAdi.FindAllSubmatch(b, -1) {
		ad := string(m[1])
		if hesaplar.GecerliDBKimlik(ad) && strings.HasPrefix(ad, "drupal_") {
			return ad, true
		}
	}
	return "", false
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := indirVeDogrula(ctx, i.Hedef)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	adminParola := hesaplar.RandomParola(18)
	kurucu := filepath.Join(i.Hedef, ".sanalcp-drupal-install.php")
	if err := os.WriteFile(kurucu, []byte(kurucuPHP), 0o600); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("Drupal kurucusu yazılamadı: %w", err)
	}
	defer os.Remove(kurucu)
	if out, err := exec.CommandContext(ctx, "chown", "-R", i.SK+":"+i.SK, i.Hedef).CombinedOutput(); err != nil {
		return apps.KurulumSonuc{}, komutHatasi("Drupal dosya izinleri", out, err)
	}
	cmd := exec.CommandContext(ctx, "runuser", "-u", i.SK, "--", "env",
		"HOME=/home/"+i.SK, "TMPDIR=/home/"+i.SK,
		"DRUPAL_DB_NAME="+i.DBAdi, "DRUPAL_DB_USER="+i.DBKullanici, "DRUPAL_DB_PASS="+i.DBParola,
		"DRUPAL_SITE_NAME="+i.Alanlar["site_adi"], "DRUPAL_ADMIN_NAME="+i.Alanlar["admin_kullanici"],
		"DRUPAL_ADMIN_MAIL="+i.Alanlar["admin_email"], "DRUPAL_ADMIN_PASS="+adminParola,
		"/usr/bin/php", kurucu)
	cmd.Dir = i.Hedef
	out, err := cmd.CombinedOutput()
	if err != nil {
		return apps.KurulumSonuc{}, komutHatasi("Drupal kurulumu", out, err)
	}
	if _, err := os.Stat(filepath.Join(i.Hedef, "sites", "default", "settings.php")); err != nil {
		return apps.KurulumSonuc{}, fmt.Errorf("Drupal kurulumu settings.php oluşturmadı")
	}
	return apps.KurulumSonuc{SiteURL: i.URL, AdminURL: i.URL + "/user/login",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola, Surum: surum}, nil
}

func (Surucu) Bilgi(ctx context.Context, _, dizin, url string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: url, AdminURL: url + "/user/login", Durum: "bilinmiyor"}
	k.Surum = yerelSurumOku(dizin)
	if rel, err := sonSurum(ctx); err == nil {
		k.SonSurum = rel.Surum
		if k.Surum == rel.Surum {
			k.Durum = "guncel"
		} else if k.Surum != "" {
			k.Durum = "eski"
		}
	}
	if fi, err := os.Stat(filepath.Join(dizin, "sites", "default", "settings.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Surucu) Guncelle(context.Context, string, string) error {
	return fmt.Errorf("Drupal güncellemesi desteklenmiyor")
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

var reSurum = regexp.MustCompile(`(?m)const VERSION\s*=\s*['"]([0-9]+\.[0-9]+\.[0-9]+)['"]\s*;`)

func yerelSurumOku(dizin string) string {
	b, err := os.ReadFile(filepath.Join(dizin, "core", "lib", "Drupal.php"))
	if err != nil {
		return ""
	}
	m := reSurum.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}

const kurucuPHP = `<?php
declare(strict_types=1);
chdir(__DIR__);
define('MAINTENANCE_MODE', 'install');
$class_loader = require __DIR__ . '/autoload.php';
require_once __DIR__ . '/core/includes/install.core.inc';
$driver = 'Drupal\\mysql\\Driver\\Database\\mysql';
$parameters = [
  'interactive' => false,
  'parameters' => ['profile' => 'standard', 'langcode' => 'en'],
  'forms' => [
    'install_settings_form' => [
      'driver' => $driver,
      $driver => [
        'database' => getenv('DRUPAL_DB_NAME'),
        'username' => getenv('DRUPAL_DB_USER'),
        'password' => getenv('DRUPAL_DB_PASS'),
        'host' => 'localhost', 'port' => '3306', 'prefix' => '',
      ],
    ],
    'install_configure_form' => [
      'site_name' => getenv('DRUPAL_SITE_NAME'),
      'site_mail' => getenv('DRUPAL_ADMIN_MAIL'),
      'account' => [
        'name' => getenv('DRUPAL_ADMIN_NAME'),
        'mail' => getenv('DRUPAL_ADMIN_MAIL'),
        'pass' => ['pass1' => getenv('DRUPAL_ADMIN_PASS'), 'pass2' => getenv('DRUPAL_ADMIN_PASS')],
      ],
      'enable_update_status_module' => true,
      'enable_update_status_emails' => null,
    ],
  ],
];
install_drupal($class_loader, $parameters);
fwrite(STDOUT, "Drupal installation successful.\n");
`
