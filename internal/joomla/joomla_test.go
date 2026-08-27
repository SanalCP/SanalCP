package joomla

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuTemelBilgiler(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "joomla" || s.Ad() != "Joomla" || s.DBOnEki() != "joomla" {
		t.Fatalf("beklenmeyen sürücü bilgisi: slug=%q ad=%q db=%q", s.Slug(), s.Ad(), s.DBOnEki())
	}
	if s.MarkerDosya() != "configuration.php" || !s.GuncelleDesteklenir() {
		t.Fatal("Joomla marker/güncelleme sözleşmesi yanlış")
	}
	var email, kullanici bool
	for _, a := range s.FormAlanlari() {
		if a.Anahtar == "admin_email" && a.Tur == "email" && a.Zorunlu {
			email = true
		}
		if a.Anahtar == "admin_kullanici" && a.Zorunlu {
			kullanici = true
		}
	}
	if !email || !kullanici {
		t.Fatal("zorunlu yönetici alanları eksik")
	}
}

func TestDBAdiOku(t *testing.T) {
	s := Surucu{}
	t.Run("gecerli", func(t *testing.T) {
		d := t.TempDir()
		icerik := "<?php\nclass JConfig { public $db = 'joomla_a1b2c3d4'; }\n"
		if err := os.WriteFile(filepath.Join(d, "configuration.php"), []byte(icerik), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, ok := s.DBAdiOku(d); !ok || got != "joomla_a1b2c3d4" {
			t.Fatalf("DBAdiOku() = %q, %v", got, ok)
		}
	})
	t.Run("baska uygulama reddedilir", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "configuration.php"), []byte("<?php public $db = \"wp_baska\";"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.DBAdiOku(d); ok {
			t.Fatal("joomla_ öneki olmayan DB reddedilmeli")
		}
	})
	if _, ok := s.DBAdiOku(t.TempDir()); ok {
		t.Fatal("configuration.php yokken bulundu=false olmalı")
	}
}

func TestYerelSurumOku(t *testing.T) {
	d := t.TempDir()
	yol := filepath.Join(d, "libraries", "src")
	if err := os.MkdirAll(yol, 0o755); err != nil {
		t.Fatal(err)
	}
	icerik := `<?php
final class Version {
 public const MAJOR_VERSION = 6;
 public const MINOR_VERSION = 1;
 public const PATCH_VERSION = 3;
}`
	if err := os.WriteFile(filepath.Join(yol, "Version.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(d); got != "6.1.3" {
		t.Fatalf("yerelSurumOku() = %q", got)
	}
}
