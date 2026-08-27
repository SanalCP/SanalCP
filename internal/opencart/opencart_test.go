package opencart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuOzellikleri(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "opencart" || s.MarkerDosya() != "config.php" || s.MinimumPHPSurum() != "8.1" {
		t.Fatal("OpenCart sürücü özellikleri hatalı")
	}
	if s.GuncelleDesteklenir() {
		t.Fatal("OpenCart otomatik güncelleme sunmamalı")
	}
}

func TestYerelSurumOku(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "index.php"), []byte("define('VERSION', '4.1.0.4');"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(d); got != "4.1.0.4" {
		t.Fatalf("sürüm = %q", got)
	}
}

func TestDBAdiOku(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "config.php"), []byte("define('DB_DATABASE', 'opencart_ab12cd34');"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := (Surucu{}).DBAdiOku(d)
	if !ok || got != "opencart_ab12cd34" {
		t.Fatalf("DB = %q, bulundu = %v", got, ok)
	}
}

func TestDBAdiOkuYabanciOnekiReddeder(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "config.php"), []byte("define('DB_DATABASE', 'drupal_ab12cd34');"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := (Surucu{}).DBAdiOku(d); ok {
		t.Fatal("yabancı DB öneki kabul edildi")
	}
}
