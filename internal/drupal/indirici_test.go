package drupal

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func tarOlustur(t *testing.T, girdiler map[string]string) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), "paket.tar.gz")
	f, err := os.Create(yol)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for ad, icerik := range girdiler {
		b := []byte(icerik)
		if err := tw.WriteHeader(&tar.Header{Name: ad, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestTarGZAcKokDiziniSoyer(t *testing.T) {
	arsiv := tarOlustur(t, map[string]string{"drupal-11.4.5/index.php": "ok"})
	hedef := t.TempDir()
	if err := tarGZAc(arsiv, hedef, "drupal-11.4.5"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(hedef, "index.php"))
	if err != nil || string(b) != "ok" {
		t.Fatalf("çıktı = %q, hata = %v", b, err)
	}
}

func TestTarGZAcDizinAsiminiReddeder(t *testing.T) {
	arsiv := tarOlustur(t, map[string]string{"drupal-11.4.5/../../kacak": "x"})
	if err := tarGZAc(arsiv, t.TempDir(), "drupal-11.4.5"); err == nil {
		t.Fatal("dizin aşımı kabul edildi")
	}
}

func TestResmiPaketURL(t *testing.T) {
	gecerli := "https://ftp.drupal.org/files/projects/drupal-11.4.5.tar.gz"
	if !resmiPaketURL(gecerli) {
		t.Fatal("resmî URL reddedildi")
	}
	for _, raw := range []string{
		"http://ftp.drupal.org/files/projects/drupal-11.4.5.tar.gz",
		"https://evil.example/files/projects/drupal-11.4.5.tar.gz",
		"https://ftp.drupal.org/other/drupal-11.4.5.tar.gz",
	} {
		if resmiPaketURL(raw) {
			t.Fatalf("güvensiz URL kabul edildi: %s", raw)
		}
	}
}

func TestReleaseXMLAyrisma(t *testing.T) {
	const veri = `<project><releases><release><version>11.4.5</version><status>published</status><files><file><url>https://ftp.drupal.org/files/projects/drupal-11.4.5.tar.gz</url><archive_type>tar.gz</archive_type><md5>7026600fd95b8551ebdff78db9f840d8</md5><size>23141994</size></file></files><security covered="1">covered</security></release></releases></project>`
	var x releaseXML
	if err := xml.Unmarshal([]byte(veri), &x); err != nil {
		t.Fatal(err)
	}
	if len(x.Releases) != 1 || x.Releases[0].Security.Covered != "1" || x.Releases[0].Files[0].Boyut != 23141994 {
		t.Fatalf("XML yanlış ayrıştırıldı: %+v", x)
	}
}

func TestTarGZAcResmiPaket(t *testing.T) {
	arsiv := os.Getenv("DRUPAL_DAGITIM_TAR_GZ")
	if arsiv == "" {
		t.Skip("DRUPAL_DAGITIM_TAR_GZ ayarlanmadı")
	}
	hedef := t.TempDir()
	if err := tarGZAc(arsiv, hedef, "drupal-11.4.5"); err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != "11.4.5" {
		t.Fatalf("resmî paket sürümü = %q", got)
	}
}

func TestIndirVeDogrulaResmiPaket(t *testing.T) {
	if os.Getenv("DRUPAL_CANLI_TEST") == "" {
		t.Skip("DRUPAL_CANLI_TEST ayarlanmadı")
	}
	hedef := t.TempDir()
	surum, err := indirVeDogrula(context.Background(), hedef)
	if err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); got != surum || !kararliSurum.MatchString(got) {
		t.Fatalf("indirilen sürüm = %q, bildirilen = %q", got, surum)
	}
}
