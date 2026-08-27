package prestashop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuTemelBilgiler(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "prestashop" {
		t.Fatalf("Slug() = %q, beklenen prestashop", s.Slug())
	}
	if s.DBOnEki() != "prestashop" {
		t.Fatalf("DBOnEki() = %q, beklenen prestashop", s.DBOnEki())
	}
	if s.MarkerDosya() != filepath.Join("config", "settings.inc.php") {
		t.Fatalf("MarkerDosya() = %q", s.MarkerDosya())
	}
	if s.GuncelleDesteklenir() != false {
		t.Fatal("PrestaShop güncelleme DESTEKLENMEMELİ (resmi CLI güncelleyici yok — spec kararı)")
	}
	if s.MinimumPHPSurum() != "8.1" || s.MaksimumPHPSurum() != "8.5" {
		t.Fatalf("PHP aralığı yanlış: %s-%s", s.MinimumPHPSurum(), s.MaksimumPHPSurum())
	}
	var eposta bool
	for _, fa := range s.FormAlanlari() {
		if fa.Anahtar == "admin_email" && fa.Tur == "email" && fa.Zorunlu {
			eposta = true
		}
	}
	if !eposta {
		t.Error("form alanlarında zorunlu email tipli admin_email eksik")
	}
}

func TestSurucuDBAdiOkuGecersizDosya(t *testing.T) {
	s := Surucu{}
	if _, bulundu := s.DBAdiOku(t.TempDir()); bulundu {
		t.Fatal("config/settings.inc.php olmayan dizinde bulundu=false olmalı")
	}
}

func TestSurucuDBAdiOkuGecerliDosya(t *testing.T) {
	s := Surucu{}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	icerik := "<?php\ndefine('_DB_NAME_', 'prestashop_a1b2c3d4');\ndefine('_DB_PREFIX_', 'ps_');\n"
	if err := os.WriteFile(filepath.Join(d, "config", "settings.inc.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	dbAdi, bulundu := s.DBAdiOku(d)
	if !bulundu {
		t.Fatal("geçerli dosyada bulundu=true olmalı")
	}
	if dbAdi != "prestashop_a1b2c3d4" {
		t.Fatalf("dbAdi = %q, beklenen prestashop_a1b2c3d4", dbAdi)
	}
}

func TestSurucuDBAdiOkuYanlisOnekReddedilir(t *testing.T) {
	s := Surucu{}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	// başka bir kiracının/uygulamanın DB'sine benzer isim — DBOnEki ("prestashop_")
	// önekini taşımıyor, guard reddetmeli.
	icerik := "<?php\ndefine('_DB_NAME_', 'wp_baskatenant');\n"
	if err := os.WriteFile(filepath.Join(d, "config", "settings.inc.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, bulundu := s.DBAdiOku(d); bulundu {
		t.Fatal("prestashop_ önekini taşımayan DB adı reddedilmeli")
	}
}

func TestPsAdminDizinBul(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "adminxyz123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := psAdminDizinBul(d); got != "adminxyz123" {
		t.Fatalf("psAdminDizinBul = %q, beklenen adminxyz123", got)
	}
}

func TestPsAdminDizinBulBulunamazsaVarsayilan(t *testing.T) {
	d := t.TempDir()
	if got := psAdminDizinBul(d); got != "admin" {
		t.Fatalf("psAdminDizinBul boş dizinde 'admin' dönmeli, döndü: %q", got)
	}
}

func TestPsBaseURI(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://ornek.test", "/"},
		{"https://ornek.test/magaza", "/magaza/"},
		{"https://ornek.test/alt/dizin/", "/alt/dizin/"},
	} {
		if got := psBaseURI(tc.raw); got != tc.want {
			t.Errorf("psBaseURI(%q) = %q, beklenen %q", tc.raw, got, tc.want)
		}
	}
}
