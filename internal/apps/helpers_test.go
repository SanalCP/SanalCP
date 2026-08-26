package apps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestZatenKuruluMu(t *testing.T) {
	t.Run("olmayan dizin temiz", func(t *testing.T) {
		yol := filepath.Join(t.TempDir(), "yok")
		if _, kurulu := zatenKuruluMu(yol, "marker.txt"); kurulu {
			t.Fatal("olmayan dizin temiz sayılmalı")
		}
	})

	t.Run("bos dizin temiz", func(t *testing.T) {
		if _, kurulu := zatenKuruluMu(t.TempDir(), "marker.txt"); kurulu {
			t.Fatal("boş dizin temiz sayılmalı")
		}
	})

	t.Run("sadece placeholder temiz", func(t *testing.T) {
		d := t.TempDir()
		for _, f := range []string{"index.html", "favicon.ico", ".htaccess", "robots.txt"} {
			if err := os.WriteFile(filepath.Join(d, f), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, kurulu := zatenKuruluMu(d, "marker.txt"); kurulu {
			t.Fatal("yalnız placeholder dosyalı dizin temiz sayılmalı")
		}
	})

	t.Run("marker dosyasi bloklar", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "marker.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		msg, kurulu := zatenKuruluMu(d, "marker.txt")
		if !kurulu || msg == "" {
			t.Fatalf("marker dosyası mevcut → kurulu:true beklenir (msg=%q kurulu=%v)", msg, kurulu)
		}
	})

	t.Run("baska icerik bloklar", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "baska.php"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, kurulu := zatenKuruluMu(d, "marker.txt"); !kurulu {
			t.Fatal("dolu dizin kurulu:true olmalı — mevcut içerik ezilmemeli")
		}
	})
}

func TestCozDizin(t *testing.T) {
	sk := "test_sk_apps"
	root := "/home/" + sk + "/public_html"
	_ = root // gerçek dosya sistemi kullanılmıyor — sadece yol-dışına-çıkma testleri

	t.Run("kok disina cikma engellenir", func(t *testing.T) {
		if _, err := cozDizin(sk, "../../etc/passwd", "marker.txt"); err == nil {
			t.Fatal("kök dizin dışına çıkan yol reddedilmeli")
		}
	})
}

func TestRandSlug(t *testing.T) {
	a := randSlug()
	b := randSlug()
	if len(a) != 8 || len(b) != 8 {
		t.Fatalf("randSlug 8 hex karakter dönmeli: a=%q(%d) b=%q(%d)", a, len(a), b, len(b))
	}
	if a == b {
		t.Fatal("iki ardışık randSlug çağrısı aynı değeri dönmemeli (rastgelelik bozuk)")
	}
}
