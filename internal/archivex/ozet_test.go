package archivex

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type uye struct {
	ad    string
	dizin bool
	icrk  string
}

func tarGzYaz(t *testing.T, yol string, uyeler []uye) {
	t.Helper()
	f, err := os.Create(yol)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, u := range uyeler {
		h := &tar.Header{Name: u.ad, Mode: 0o644, Size: int64(len(u.icrk))}
		if u.dizin {
			h.Typeflag = tar.TypeDir
			h.Mode = 0o755
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if !u.dizin {
			if _, err := tw.Write([]byte(u.icrk)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

// Tek kapsayıcı klasörü olan arşiv → KokKlasor dolu olmalı. Bu tespit olmadan
// yedek public_html/public_html/ altına açılır ve site çalışmaz.
func TestOzetleTekKokTespiti(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "yedek.tar.gz")
	tarGzYaz(t, yol, []uye{
		{ad: "public_html/", dizin: true},
		{ad: "public_html/index.php", icrk: "<?php"},
		{ad: "public_html/wp-config.php", icrk: "<?php define('DB_NAME','eski');"},
		{ad: "public_html/wp-content/", dizin: true},
		{ad: "public_html/wp-content/tema.css", icrk: "body{}"},
	})

	oz, err := Ozetle(yol, TurTarGz, []string{"wp-config.php", "index.php"})
	if err != nil {
		t.Fatal(err)
	}
	if oz.KokKlasor != "public_html" {
		t.Errorf("kök klasör public_html bekleniyordu, %q geldi", oz.KokKlasor)
	}
	if oz.UyeSayisi != 5 {
		t.Errorf("üye sayısı 5 bekleniyordu, %d", oz.UyeSayisi)
	}
	yollar := oz.Isaretler["wp-config.php"]
	if len(yollar) != 1 || yollar[0] != "public_html" {
		t.Errorf("wp-config.php public_html altında bulunmalıydı: %v", yollar)
	}
}

// Kökte dosya varsa tek kapsayıcı klasör YOKTUR — strip yapılırsa o dosyalar
// kaybolurdu, bu yüzden KokKlasor boş dönmeli.
func TestOzetleKoktekiDosyaTekKokuBozar(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "duz.tar.gz")
	tarGzYaz(t, yol, []uye{
		{ad: "index.php", icrk: "<?php"},
		{ad: "wp-admin/", dizin: true},
		{ad: "wp-admin/a.php", icrk: "x"},
	})
	oz, err := Ozetle(yol, TurTarGz, nil)
	if err != nil {
		t.Fatal(err)
	}
	if oz.KokKlasor != "" {
		t.Errorf("tek kök olmamalıydı, %q geldi", oz.KokKlasor)
	}
	if len(oz.Kokler) != 2 {
		t.Errorf("2 kök girdisi bekleniyordu: %v", oz.Kokler)
	}
}

// Ozetle güvenlik ön-taramasını da uygular: jail dışına çıkan üye REDDEDİLİR.
func TestOzetleJailDisiUyeyiReddeder(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "kotu.tar.gz")
	tarGzYaz(t, yol, []uye{
		{ad: "site/index.php", icrk: "x"},
		{ad: "../../etc/cron.d/pwn", icrk: "* * * * * root sh"},
	})
	if _, err := Ozetle(yol, TurTarGz, nil); err == nil {
		t.Fatal("jail dışına çıkan üye kabul edildi")
	}
}

func TestOzetleZipTekKok(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "yedek.zip")
	f, err := os.Create(yol)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, ad := range []string{"htdocs/domain.com/index.php", "htdocs/domain.com/.env"} {
		w, err := zw.Create(ad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("DB_DATABASE=eski\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	oz, err := Ozetle(yol, TurZip, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	if oz.KokKlasor != "htdocs" {
		t.Errorf("kök htdocs bekleniyordu, %q", oz.KokKlasor)
	}
	if v := oz.Isaretler[".env"]; len(v) != 1 || v[0] != "htdocs/domain.com" {
		t.Errorf(".env yolu yanlış: %v", v)
	}
}
