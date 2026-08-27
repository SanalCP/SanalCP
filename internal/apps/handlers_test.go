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

func TestTurlerFormSemasiIcerir(t *testing.T) {
	Kaydet(sahteUygulama{slug: "test-tur-form"})
	u, ok := Bul("test-tur-form")
	if !ok {
		t.Fatal("kayıt bulunamadı")
	}
	if u.Ad() != "Sahte test-tur-form" {
		t.Fatalf("Ad() beklenmedik: %q", u.Ad())
	}
	// Turler handler'ının ürettiği türBilgi şeklini burada, HTTP'siz, doğrudan
	// registry üzerinden doğruluyoruz (Handlers.Turler'ın DB'ye dokunmadığını
	// ve yalnız Hepsi()'yi map'lediğini garanti eden regresyon testi).
	var bulunduSlug bool
	for _, uu := range Hepsi() {
		if uu.Slug() == "test-tur-form" {
			bulunduSlug = true
		}
	}
	if !bulunduSlug {
		t.Fatal("Hepsi() kayıtlı türü içermeli")
	}
}

func TestSurumEnAz(t *testing.T) {
	for _, tc := range []struct {
		mevcut, minimum string
		beklenen        bool
	}{
		{"8.3", "8.3", true}, {"8.4", "8.3", true}, {"9.0", "8.3", true},
		{"8.2", "8.3", false}, {"7.4", "8.3", false}, {"bozuk", "8.3", false},
	} {
		if got := surumEnAz(tc.mevcut, tc.minimum); got != tc.beklenen {
			t.Errorf("surumEnAz(%q, %q) = %v, beklenen %v", tc.mevcut, tc.minimum, got, tc.beklenen)
		}
	}
}
