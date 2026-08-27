package opencart

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func zipOlustur(t *testing.T, girdiler map[string]string) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), "paket.zip")
	f, err := os.Create(yol)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for ad, icerik := range girdiler {
		w, err := zw.Create(ad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(icerik)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestZipAcYalnizUploadIceriginiCikarir(t *testing.T) {
	arsiv := zipOlustur(t, map[string]string{
		"README.md": "alma", "upload/index.php": "site", "upload/install/cli_install.php": "cli",
	})
	hedef := t.TempDir()
	if err := zipAc(arsiv, hedef); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "README.md")); !os.IsNotExist(err) {
		t.Fatal("paket kökü web dizinine çıkarıldı")
	}
	b, err := os.ReadFile(filepath.Join(hedef, "index.php"))
	if err != nil || string(b) != "site" {
		t.Fatalf("çıktı = %q, hata = %v", b, err)
	}
}

func TestZipAcDizinAsiminiReddeder(t *testing.T) {
	arsiv := zipOlustur(t, map[string]string{"upload/install/cli_install.php": "cli", "upload/../../kacak": "x"})
	if err := zipAc(arsiv, t.TempDir()); err == nil {
		t.Fatal("dizin aşımı kabul edildi")
	}
}

func TestZipAcCLIZorunlu(t *testing.T) {
	arsiv := zipOlustur(t, map[string]string{"upload/index.php": "site"})
	if err := zipAc(arsiv, t.TempDir()); err == nil {
		t.Fatal("CLI içermeyen paket kabul edildi")
	}
}

func TestSurumKarsilastir(t *testing.T) {
	if surumKarsilastir("4.1.0.4", "4.1.0.3") <= 0 || surumKarsilastir("4.0.2.3", "4.1.0.0") >= 0 || surumKarsilastir("4.1.0.4", "4.1.0.4") != 0 {
		t.Fatal("OpenCart sürüm karşılaştırması hatalı")
	}
}

func TestZipAcResmiPaket(t *testing.T) {
	arsiv := os.Getenv("OPENCART_DAGITIM_ZIP")
	if arsiv == "" {
		t.Skip("OPENCART_DAGITIM_ZIP ayarlanmadı")
	}
	hedef := t.TempDir()
	if err := zipAc(arsiv, hedef); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != "4.1.0.4" {
		t.Fatalf("resmî paket sürümü = %q", got)
	}
}

func TestIndirVeDogrulaResmiPaket(t *testing.T) {
	if os.Getenv("OPENCART_CANLI_TEST") == "" {
		t.Skip("OPENCART_CANLI_TEST ayarlanmadı")
	}
	hedef := t.TempDir()
	surum, err := indirVeDogrula(context.Background(), hedef)
	if err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != surum || !kararliSurum.MatchString(got) {
		t.Fatalf("indirilen = %q, bildirilen = %q", got, surum)
	}
}
