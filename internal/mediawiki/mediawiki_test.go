package mediawiki

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuVeOkuyucular(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "mediawiki" || s.MinimumPHPSurum() != "8.3" || s.MaksimumPHPSurum() != "8.5" {
		t.Fatal("MediaWiki özellikleri hatalı")
	}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "includes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "LocalSettings.php"), []byte("<?php\n$wgDBname = \"mediawiki_a1b2c3d4\";\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "includes", "Defines.php"), []byte("<?php\ndefine( 'MW_VERSION', '1.46.0' );\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if db, ok := s.DBAdiOku(d); !ok || db != "mediawiki_a1b2c3d4" {
		t.Fatalf("db=%q ok=%v", db, ok)
	}
	if got := yerelSurumOku(d); got != sabitSurum {
		t.Fatalf("sürüm=%q", got)
	}
}

func TestIndirVeDogrulaCanli(t *testing.T) {
	if os.Getenv("MEDIAWIKI_CANLI_TEST") == "" {
		t.Skip("MEDIAWIKI_CANLI_TEST ayarlanmadı")
	}
	hedef := t.TempDir()
	if err := indirVeDogrula(t.Context(), hedef); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != sabitSurum {
		t.Fatalf("sürüm=%q", got)
	}
}
