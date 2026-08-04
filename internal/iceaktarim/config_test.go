package iceaktarim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sanalcp/internal/archivex"
	"sanalcp/internal/sqlimport"
)

// Test parolası bilerek zor: hem PHP tek-tırnak kaçışını, hem Go regexp
// şablonundaki "$" grup-referansını, hem dotenv tırnaklamasını zorlar.
var testKimlik = sqlimport.Hedef{
	DBAdi:     "c_yeni_wp",
	Kullanici: "c_yeni_wpu",
	Parola:    `a'b\c$1d"e f`,
	Host:      "localhost",
}

func sahteHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "public_html"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestWpGuncelleDBSabitleriniDegistirir(t *testing.T) {
	home := sahteHome(t)
	orijinal := `<?php
define( 'DB_NAME', 'eski_veritabani' );
define('DB_USER', "eski_kullanici");
define('DB_PASSWORD', 'eski\'parola');
define('DB_HOST', 'mysql.eskisunucu.com');
define('AUTH_KEY', 'dokunulmamali');
$table_prefix = 'wp_';
`
	yol := filepath.Join(home, "public_html", "wp-config.php")
	if err := os.WriteFile(yol, []byte(orijinal), 0o644); err != nil {
		t.Fatal(err)
	}

	g, ok := wpGuncelle(home, "c_yok", "public_html", testKimlik)
	if !ok {
		t.Fatal("wp-config.php bulunamadı")
	}
	if !g.Uygulandi {
		t.Fatalf("güncelleme uygulanmadı: %s", g.Not)
	}
	if len(g.Alanlar) != 4 {
		t.Errorf("4 alan güncellenmeliydi: %v", g.Alanlar)
	}

	b, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	yeni := string(b)

	// Eski değerlerin hiçbiri kalmamalı.
	for _, eski := range []string{"eski_veritabani", "eski_kullanici", "mysql.eskisunucu.com"} {
		if strings.Contains(yeni, eski) {
			t.Errorf("eski değer kaldı: %q\n%s", eski, yeni)
		}
	}
	// Yeni değerler PHP kaçışlı olarak yazılmalı.
	if !strings.Contains(yeni, `define( 'DB_NAME', 'c_yeni_wp' )`) {
		t.Errorf("DB_NAME yazılmadı:\n%s", yeni)
	}
	if !strings.Contains(yeni, `'a\'b\\c$1d"e f'`) {
		t.Errorf("parola PHP kaçışıyla yazılmadı (veya $1 grup referansı yenildi):\n%s", yeni)
	}
	// İlgisiz sabit korunmalı.
	if !strings.Contains(yeni, `define('AUTH_KEY', 'dokunulmamali')`) {
		t.Error("ilgisiz sabit bozuldu")
	}
}

func TestWpGuncelleDinamikConfigeDokunmaz(t *testing.T) {
	home := sahteHome(t)
	orijinal := "<?php\ndefine('DB_NAME', getenv('DB_NAME'));\n"
	yol := filepath.Join(home, "public_html", "wp-config.php")
	if err := os.WriteFile(yol, []byte(orijinal), 0o644); err != nil {
		t.Fatal(err)
	}
	g, ok := wpGuncelle(home, "c_yok", "public_html", testKimlik)
	if !ok {
		t.Fatal("dosya bulunamadı")
	}
	if g.Uygulandi {
		t.Error("düz metin olmayan config değiştirilmemeliydi")
	}
	if g.Not == "" {
		t.Error("kullanıcıya elle güncelleme notu verilmeliydi")
	}
	b, _ := os.ReadFile(yol)
	if string(b) != orijinal {
		t.Error("dosya değiştirilmiş olmamalıydı")
	}
}

func TestEnvGuncelleLaravel(t *testing.T) {
	home := sahteHome(t)
	dizin := filepath.Join(home, "public_html")
	if err := os.WriteFile(filepath.Join(dizin, "artisan"), []byte("#!/usr/bin/env php"), 0o644); err != nil {
		t.Fatal(err)
	}
	orijinal := strings.Join([]string{
		"APP_NAME=Site",
		"# DB_DATABASE=yorumdaki_deger",
		"DB_CONNECTION=pgsql",
		"DB_DATABASE=eski_db",
		"DB_USERNAME=eski_user",
		"MAIL_HOST=smtp.example.com",
		"",
	}, "\n")
	yol := filepath.Join(dizin, ".env")
	if err := os.WriteFile(yol, []byte(orijinal), 0o644); err != nil {
		t.Fatal(err)
	}

	g, ok := envGuncelle(home, "c_yok", "public_html", testKimlik)
	if !ok || !g.Uygulandi {
		t.Fatalf("env güncellenmedi: ok=%v not=%s", ok, g.Not)
	}
	b, _ := os.ReadFile(yol)
	yeni := string(b)

	for _, beklenen := range []string{
		"DB_CONNECTION=mysql",
		"DB_DATABASE=c_yeni_wp",
		"DB_USERNAME=c_yeni_wpu",
		"DB_HOST=localhost",
	} {
		if !strings.Contains(yeni, beklenen) {
			t.Errorf("%q yazılmadı:\n%s", beklenen, yeni)
		}
	}
	// Parola boşluk/tırnak/$ içeriyor → tırnaklanıp kaçırılmalı.
	if !strings.Contains(yeni, `DB_PASSWORD="a'b\\c\$1d\"e f"`) {
		t.Errorf("parola dotenv kaçışıyla yazılmadı:\n%s", yeni)
	}
	if strings.Contains(yeni, "eski_db") || strings.Contains(yeni, "DB_CONNECTION=pgsql") {
		t.Errorf("eski değer kaldı:\n%s", yeni)
	}
	// İlgisiz satır ve yorum korunmalı.
	if !strings.Contains(yeni, "MAIL_HOST=smtp.example.com") ||
		!strings.Contains(yeni, "# DB_DATABASE=yorumdaki_deger") {
		t.Errorf("ilgisiz satırlar bozuldu:\n%s", yeni)
	}
}

// .env'i olan ama Laravel olmayan bir projeye (artisan yok, DB_* yok)
// dokunulmamalı.
func TestEnvGuncelleLaravelOlmayaniAtlar(t *testing.T) {
	home := sahteHome(t)
	yol := filepath.Join(home, "public_html", ".env")
	if err := os.WriteFile(yol, []byte("NODE_ENV=production\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := envGuncelle(home, "c_yok", "public_html", testKimlik); ok {
		t.Error("Laravel olmayan .env değiştirildi")
	}
}

func TestHedefDogrula(t *testing.T) {
	gecerli := map[string]string{
		"":                    "public_html",
		"public_html":         "public_html",
		"/public_html/":       "public_html",
		"public_html/alt":     "public_html/alt",
		"../../etc":           "etc", // jail'e indirgenir, kaçış değil
		"public_html/../depo": "depo",
	}
	for girdi, beklenen := range gecerli {
		got, err := hedefDogrula(girdi)
		if err != nil {
			t.Errorf("hedefDogrula(%q) hata verdi: %v", girdi, err)
			continue
		}
		if got != beklenen {
			t.Errorf("hedefDogrula(%q) = %q, beklenen %q", girdi, got, beklenen)
		}
	}
	for _, kotu := range []string{"/", ".", stagingRel, stagingRel + "/alt"} {
		if _, err := hedefDogrula(kotu); err == nil {
			t.Errorf("hedefDogrula(%q) kabul edildi, reddedilmeliydi", kotu)
		}
	}
}

func TestStageIDDeseni(t *testing.T) {
	gecerli := []string{
		strings.Repeat("a", 32) + ".zip",
		strings.Repeat("0", 32) + ".tar.gz",
	}
	for _, s := range gecerli {
		if !reStageID.MatchString(s) {
			t.Errorf("geçerli stage id reddedildi: %q", s)
		}
	}
	kotu := []string{
		"../../etc/passwd",
		strings.Repeat("a", 32) + "/../x.zip",
		strings.Repeat("a", 31) + ".zip",
		strings.Repeat("A", 32) + ".zip", // büyük harf hex değil
		strings.Repeat("a", 32) + ".sh",
	}
	for _, s := range kotu {
		if reStageID.MatchString(s) {
			t.Errorf("geçersiz stage id kabul edildi: %q", s)
		}
	}
}

func TestUygulamaTespit(t *testing.T) {
	oz := archivex.Ozet{Isaretler: map[string][]string{
		"index.php":     {"public_html", "public_html/wp-admin"},
		"wp-config.php": {"public_html/derin/wp", "public_html"},
	}}
	uyg, dizin := uygulamaTespit(oz)
	if uyg != "wordpress" {
		t.Errorf("wordpress bekleniyordu, %q", uyg)
	}
	// En sığ eşleşme seçilmeli.
	if dizin != "public_html" {
		t.Errorf("en sığ dizin public_html olmalıydı, %q", dizin)
	}

	bos := archivex.Ozet{Isaretler: map[string][]string{"index.php": {"x"}}}
	if uyg, _ := uygulamaTespit(bos); uyg != "" {
		t.Errorf("uygulama tespit edilmemeliydi, %q", uyg)
	}
}
