package grav

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuOzellikleri(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "grav" || s.MarkerDosya() != "system/defines.php" || s.MinimumPHPSurum() != "8.3" {
		t.Fatal("Grav sürücü özellikleri hatalı")
	}
	if !s.GuncelleDesteklenir() {
		t.Fatal("Grav GPM güncellemesi desteklenmeli")
	}
	if s.VeritabaniGerekli() {
		t.Fatal("Grav için veritabanı oluşturulmamalı")
	}
	if _, ok := s.DBAdiOku(""); ok {
		t.Fatal("Grav veritabanı bildirdi")
	}
}

func TestYerelSurumOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "system")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yol, "defines.php"), []byte(`define("GRAV_VERSION", "2.0.21");`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(d); got != "2.0.21" {
		t.Fatalf("sürüm = %q", got)
	}
}
