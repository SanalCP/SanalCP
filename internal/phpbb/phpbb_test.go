package phpbb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuVeOkuyucular(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "phpbb" || s.MinimumPHPSurum() != "7.4" || s.MaksimumPHPSurum() != "8.4" {
		t.Fatal("phpBB özellikleri hatalı")
	}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "includes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "config.php"), []byte("<?php\n$db_name = 'phpbb_a1b2c3d4';\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "includes", "constants.php"), []byte("<?php\n@define('PHPBB_VERSION', '3.3.17');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if db, ok := s.DBAdiOku(d); !ok || db != "phpbb_a1b2c3d4" {
		t.Fatalf("db=%q ok=%v", db, ok)
	}
	if got := yerelSurumOku(d); got != sabitSurum {
		t.Fatalf("sürüm=%q", got)
	}
}
func TestYAMLDeger(t *testing.T) {
	if got := yamlDeger("a\"b\n"); got != `"a\"b\n"` {
		t.Fatalf("yaml=%s", got)
	}
}
func TestIndirVeDogrulaCanli(t *testing.T) {
	if os.Getenv("PHPBB_CANLI_TEST") == "" {
		t.Skip("PHPBB_CANLI_TEST ayarlanmadı")
	}
	hedef := t.TempDir()
	if err := indirVeDogrula(t.Context(), hedef); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != sabitSurum {
		t.Fatalf("sürüm=%q", got)
	}
}
