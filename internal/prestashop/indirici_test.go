package prestashop

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func zipOlustur(t *testing.T, dosyalar map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for ad, icerik := range dosyalar {
		f, err := zw.Create(ad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(icerik)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	yol := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(yol, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestPsZipAcKokte(t *testing.T) {
	zipYolu := zipOlustur(t, map[string]string{
		"index.php":         "<?php",
		"install/index.php": "<?php",
	})
	hedef := t.TempDir()
	if err := psZipAc(zipYolu, hedef); err != nil {
		t.Fatalf("psZipAc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "index.php")); err != nil {
		t.Fatalf("kökteki index.php açılmalıydı: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "install", "index.php")); err != nil {
		t.Fatalf("install/index.php açılmalıydı: %v", err)
	}
}

func TestPsZipAcTekUstDizinliSoyulur(t *testing.T) {
	zipYolu := zipOlustur(t, map[string]string{
		"prestashop-9.1.5/index.php":         "<?php",
		"prestashop-9.1.5/install/index.php": "<?php",
	})
	hedef := t.TempDir()
	if err := psZipAc(zipYolu, hedef); err != nil {
		t.Fatalf("psZipAc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "prestashop-9.1.5")); err == nil {
		t.Fatal("üst dizin soyulmalıydı, ama hâlâ mevcut")
	}
	if _, err := os.Stat(filepath.Join(hedef, "index.php")); err != nil {
		t.Fatalf("üst dizin soyulduktan sonra index.php kökte olmalıydı: %v", err)
	}
}

func TestPsZipAcZipSlipEngellenir(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("kotu"))
	_ = zw.Close()
	yol := filepath.Join(t.TempDir(), "kotu.zip")
	if err := os.WriteFile(yol, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	hedef := t.TempDir()
	if err := psZipAc(yol, hedef); err == nil {
		t.Fatal("zip-slip girişimi reddedilmeliydi")
	}
}
