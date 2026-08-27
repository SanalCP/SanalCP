package joomla

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func tarOlustur(t *testing.T, girdiler map[string]string) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), "test.tar.gz")
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

func TestTarGZAc(t *testing.T) {
	arsiv := tarOlustur(t, map[string]string{
		"index.php":                 "<?php",
		"libraries/src/Version.php": "version",
	})
	hedef := t.TempDir()
	if err := tarGZAc(arsiv, hedef); err != nil {
		t.Fatalf("tarGZAc: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(hedef, "libraries", "src", "Version.php")); err != nil || string(b) != "version" {
		t.Fatalf("çıkarılan dosya yanlış: %q, %v", b, err)
	}
}

func TestTarGZAcDizinAsimiEngellenir(t *testing.T) {
	arsiv := tarOlustur(t, map[string]string{"../../etc/passwd": "kotu"})
	if err := tarGZAc(arsiv, t.TempDir()); err == nil {
		t.Fatal("dizin aşımı reddedilmeliydi")
	}
}

func TestTarGZAcSembolikBagEngellenir(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "symlink.tar.gz")
	f, err := os.Create(yol)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	if err := tarGZAc(yol, t.TempDir()); err == nil {
		t.Fatal("sembolik bağ reddedilmeliydi")
	}
}
