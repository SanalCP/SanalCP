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

func TestPsDagitimZipAcIcPaketiAcar(t *testing.T) {
	ic := zipOlustur(t, map[string]string{
		"index.php":               "<?php",
		"install/index_cli.php":   "<?php",
		"config/settings.inc.php": "<?php",
	})
	icB, err := os.ReadFile(ic)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("prestashop.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(icB); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dis := filepath.Join(t.TempDir(), "distribution.zip")
	if err := os.WriteFile(dis, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	hedef := t.TempDir()
	if err := psDagitimZipAc(dis, hedef); err != nil {
		t.Fatalf("psDagitimZipAc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "install", "index_cli.php")); err != nil {
		t.Fatalf("iç paket hedefe açılmalıydı: %v", err)
	}
}

func TestPsDagitimZipAcIcPaketYoksaReddeder(t *testing.T) {
	dis := zipOlustur(t, map[string]string{"index.php": "wrapper"})
	if err := psDagitimZipAc(dis, t.TempDir()); err == nil {
		t.Fatal("prestashop.zip içermeyen dağıtım reddedilmeli")
	}
}

func TestPsResmiURL(t *testing.T) {
	gecerli := "https://api.prestashop-project.org/assets/prestashop-classic/9.1.5-5.0/prestashop.zip"
	if !psResmiURL(gecerli) {
		t.Fatal("resmî URL kabul edilmeliydi")
	}
	for _, raw := range []string{
		"http://api.prestashop-project.org/assets/prestashop-classic/x.zip",
		"https://evil.example/assets/prestashop-classic/x.zip",
		"https://api.prestashop-project.org/other/x.zip",
	} {
		if psResmiURL(raw) {
			t.Fatalf("güvensiz URL kabul edildi: %s", raw)
		}
	}
}

// Büyük resmî paket normal CI'ya indirilmez. Yayın/entegrasyon doğrulamasında
// PRESTASHOP_DAGITIM_ZIP verilerek dış sarmal ve gerçek 40K+ dosyalı iç paket
// aynı güvenli açıcıdan geçirilir.
func TestPsDagitimZipAcResmiPaket(t *testing.T) {
	yol := os.Getenv("PRESTASHOP_DAGITIM_ZIP")
	if yol == "" {
		t.Skip("PRESTASHOP_DAGITIM_ZIP ayarlı değil")
	}
	hedef := t.TempDir()
	if err := psDagitimZipAc(yol, hedef); err != nil {
		t.Fatalf("resmî dağıtım açılamadı: %v", err)
	}
	for _, beklenen := range []string{"index.php", filepath.Join("install", "index_cli.php"), filepath.Join("config", "defines.inc.php")} {
		if _, err := os.Stat(filepath.Join(hedef, beklenen)); err != nil {
			t.Fatalf("resmî pakette %s bulunamadı: %v", beklenen, err)
		}
	}
}
