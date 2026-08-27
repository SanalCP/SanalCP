package matomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurucuOzellikleri(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "matomo" || s.MarkerDosya() != "config/config.ini.php" || s.MinimumPHPSurum() != "8.1" {
		t.Fatal("Matomo sürücü özellikleri hatalı")
	}
	if s.GuncelleDesteklenir() {
		t.Fatal("Matomo paket güncellemesi desteklenmemeli")
	}
}

func TestYerelSurumOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "core")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "Version.php"), []byte("public const VERSION = '5.13.0';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(d); got != "5.13.0" {
		t.Fatalf("sürüm = %q", got)
	}
}

func TestDBAdiOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "config")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "config.ini.php"), []byte("[database]\ndbname = \"matomo_ab12cd34\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := (Surucu{}).DBAdiOku(d)
	if !ok || got != "matomo_ab12cd34" {
		t.Fatalf("DB = %q, bulundu = %v", got, ok)
	}
}

func TestDBAdiOkuYabanciOnekiReddeder(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "config")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "config.ini.php"), []byte("dbname = opencart_ab12cd34\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := (Surucu{}).DBAdiOku(d); ok {
		t.Fatal("yabancı DB öneki kabul edildi")
	}
}

func TestSonLogSiniri(t *testing.T) {
	got := sonLog(strings.Repeat("x", 800))
	if len(got) > 600 {
		t.Fatalf("log uzunluğu = %d", len(got))
	}
}
