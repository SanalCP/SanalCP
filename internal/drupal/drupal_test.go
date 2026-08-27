package drupal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuOzellikleri(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "drupal" || s.MarkerDosya() != "sites/default/settings.php" {
		t.Fatal("Drupal sürücü kimliği hatalı")
	}
	if s.MinimumPHPSurum() != "8.3" || s.MaksimumPHPSurum() != "8.5" {
		t.Fatal("Drupal PHP aralığı hatalı")
	}
	if s.GuncelleDesteklenir() {
		t.Fatal("paket tabanlı Drupal kurulumu otomatik güncelleme sunmamalı")
	}
}

func TestYerelSurumOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "core", "lib")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "Drupal.php"), []byte("const VERSION = '11.4.5';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(d); got != "11.4.5" {
		t.Fatalf("sürüm = %q", got)
	}
}

func TestDBAdiOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "sites", "default")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	icerik := "// 'database' => 'database_name'\n$databases['default']['default'] = ['database' => 'drupal_ab12cd34'];"
	if err := os.WriteFile(filepath.Join(yol, "settings.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := (Surucu{}).DBAdiOku(d)
	if !ok || got != "drupal_ab12cd34" {
		t.Fatalf("DB = %q, bulundu = %v", got, ok)
	}
}

func TestDBAdiOkuYabanciOnekiReddeder(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "sites", "default")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "settings.php"), []byte("'database' => 'wordpress_ab12cd34'"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := (Surucu{}).DBAdiOku(d); ok {
		t.Fatal("yabancı DB öneki kabul edildi")
	}
}
