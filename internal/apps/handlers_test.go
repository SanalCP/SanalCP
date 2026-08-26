package apps

import "testing"

func TestAlanlariDogrula(t *testing.T) {
	alanlar := []FormAlan{
		{Anahtar: "ad", Etiket: "Ad", Tur: "text", Zorunlu: true},
		{Anahtar: "eposta", Etiket: "E-posta", Tur: "email", Zorunlu: true},
		{Anahtar: "notlar", Etiket: "Notlar", Tur: "text", Zorunlu: false},
	}

	t.Run("zorunlu alan eksikse hata doner", func(t *testing.T) {
		girdi := map[string]string{"eposta": "a@b.com"}
		if _, hata := alanlariDogrula(alanlar, girdi); hata == "" {
			t.Fatal("zorunlu 'ad' eksik — hata dönmeli")
		}
	})

	t.Run("gecersiz email hata doner", func(t *testing.T) {
		girdi := map[string]string{"ad": "x", "eposta": "gecersiz"}
		if _, hata := alanlariDogrula(alanlar, girdi); hata == "" {
			t.Fatal("geçersiz e-posta — hata dönmeli")
		}
	})

	t.Run("opsiyonel alan bos gecebilir", func(t *testing.T) {
		girdi := map[string]string{"ad": "x", "eposta": "a@b.com"}
		temiz, hata := alanlariDogrula(alanlar, girdi)
		if hata != "" {
			t.Fatalf("geçerli girdi hata dönmemeli: %q", hata)
		}
		if temiz["notlar"] != "" {
			t.Fatalf("boş opsiyonel alan boş kalmalı, geldi: %q", temiz["notlar"])
		}
	})

	t.Run("bosluklar kirpilir", func(t *testing.T) {
		girdi := map[string]string{"ad": "  x  ", "eposta": " a@b.com "}
		temiz, hata := alanlariDogrula(alanlar, girdi)
		if hata != "" {
			t.Fatalf("geçerli girdi hata dönmemeli: %q", hata)
		}
		if temiz["ad"] != "x" || temiz["eposta"] != "a@b.com" {
			t.Fatalf("boşluklar kırpılmalı: %+v", temiz)
		}
	})
}
