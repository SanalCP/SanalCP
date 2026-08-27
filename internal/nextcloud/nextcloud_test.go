package nextcloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSurucuOzellikleri(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "nextcloud" || s.MarkerDosya() != "config/config.php" {
		t.Fatal("Nextcloud sürücü özellikleri hatalı")
	}
	if s.MinimumPHPSurum() != "8.2" || s.MaksimumPHPSurum() != "8.5" {
		t.Fatal("Nextcloud PHP sınırları hatalı")
	}
	if s.GuncelleDesteklenir() || s.KurulumZamanAsimi() != 15*time.Minute {
		t.Fatal("Nextcloud güncelleme/zaman aşımı özellikleri hatalı")
	}
}

func TestDBAdiVeSurumOku(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `<?php $CONFIG = array ('dbname' => 'nextcloud_a1b2c3d4', 'datadirectory' => '/safe');`
	if err := os.WriteFile(filepath.Join(d, "config", "config.php"), []byte(config), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "version.php"), []byte("<?php\n$OC_Version = array(34, 0, 3, 0);\n$OC_VersionString = '34.0.3';\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := (Surucu{}).DBAdiOku(d); !ok || got != "nextcloud_a1b2c3d4" {
		t.Fatalf("DB adı = %q, bulundu=%v", got, ok)
	}
	if got := yerelSurumOku(d); got != "34.0.3" {
		t.Fatalf("sürüm = %q", got)
	}
}

func TestVeriDiziniWebKokuDisindaVeKararli(t *testing.T) {
	a := veriDiziniYolu("c_test", "/home/c_test/public_html/cloud")
	b := veriDiziniYolu("c_test", "/home/c_test/public_html/cloud")
	if a != b || !strings.HasPrefix(a, "/home/c_test/.sanalcp-nextcloud-data-") {
		t.Fatalf("güvensiz/kararsız veri dizini: %q / %q", a, b)
	}
	if strings.Contains(a, "/public_html/") {
		t.Fatalf("veri dizini web kökünde: %q", a)
	}
}
