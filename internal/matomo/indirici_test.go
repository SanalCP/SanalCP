package matomo

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

func TestZipAcKokDiziniSoyer(t *testing.T) {
	arsiv := zipOlustur(t, map[string]string{"README.md": "alma", "matomo/index.php": "site", "matomo/console": "cli"})
	hedef := t.TempDir()
	if err := zipAc(arsiv, hedef); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "README.md")); !os.IsNotExist(err) {
		t.Fatal("paket kökü çıkarıldı")
	}
	b, err := os.ReadFile(filepath.Join(hedef, "index.php"))
	if err != nil || string(b) != "site" {
		t.Fatalf("çıktı = %q, hata = %v", b, err)
	}
}

func TestZipAcDizinAsiminiReddeder(t *testing.T) {
	arsiv := zipOlustur(t, map[string]string{"matomo/console": "cli", "matomo/../../kacak": "x"})
	if err := zipAc(arsiv, t.TempDir()); err == nil {
		t.Fatal("dizin aşımı kabul edildi")
	}
}

func TestZipAcConsoleZorunlu(t *testing.T) {
	if err := zipAc(zipOlustur(t, map[string]string{"matomo/index.php": "x"}), t.TempDir()); err == nil {
		t.Fatal("console içermeyen paket kabul edildi")
	}
}

func TestZipAcResmiPaket(t *testing.T) {
	arsiv := os.Getenv("MATOMO_DAGITIM_ZIP")
	if arsiv == "" {
		t.Skip("MATOMO_DAGITIM_ZIP ayarlanmadı")
	}
	hedef := t.TempDir()
	if err := zipAc(arsiv, hedef); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != "5.13.0" {
		t.Fatalf("sürüm = %q", got)
	}
}

func TestIndirVeDogrulaResmiPaket(t *testing.T) {
	if os.Getenv("MATOMO_CANLI_TEST") == "" {
		t.Skip("MATOMO_CANLI_TEST ayarlanmadı")
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
