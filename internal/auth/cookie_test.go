package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// oturumCerezindenToken: yanıttaki oturum çerezinin ham değerini döner.
// Çerez yoksa testi düşürür.
func oturumCerezindenToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == OturumCerezAdi {
			return c.Value
		}
	}
	t.Fatalf("yanıtta %q çerezi yok (Set-Cookie: %v)", OturumCerezAdi, w.Result().Header["Set-Cookie"])
	return ""
}

func oturumCerezi_(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == OturumCerezAdi {
			return c
		}
	}
	t.Fatalf("yanıtta %q çerezi yok", OturumCerezAdi)
	return nil
}

func TestOturumCerezOznitelikleri(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	OturumCerezYaz(w, r, "jwt-degeri", 3600)

	c := oturumCerezi_(t, w)
	if !c.HttpOnly {
		t.Error("HttpOnly set edilmemiş — JavaScript oturumu okuyabilir, değişikliğin tüm amacı bu")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, beklenen Strict", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, beklenen \"/\"", c.Path)
	}
	if c.MaxAge != 3600 {
		t.Errorf("MaxAge = %d, beklenen 3600", c.MaxAge)
	}
	if c.Value != "jwt-degeri" {
		t.Errorf("Value = %q, beklenen \"jwt-degeri\"", c.Value)
	}
}

// TestOturumCerezSecureSemasaBagli: düz HTTP üzerinden Secure set EDİLMEMELİ
// (taze kurulumda panel SSL kurulana kadar http:// üzerinden açılır ve
// koşulsuz Secure girişi tamamen kırardı), HTTPS'te ise set EDİLMELİ.
func TestOturumCerezSecureSemasaBagli(t *testing.T) {
	tablo := []struct {
		ad             string
		forwardedProto string
		tls            bool
		beklenenSecure bool
	}{
		{"düz http, başlık yok", "", false, false},
		{"nginx https", "https", false, true},
		{"nginx https büyük harf", "HTTPS", false, true},
		{"nginx https boşluklu", "  https  ", false, true},
		{"nginx http", "http", false, false},
		{"doğrudan TLS", "", true, true},
	}
	for _, c := range tablo {
		t.Run(c.ad, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			if c.forwardedProto != "" {
				r.Header.Set("X-Forwarded-Proto", c.forwardedProto)
			}
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			} else {
				r.TLS = nil
			}
			w := httptest.NewRecorder()
			OturumCerezYaz(w, r, "t", 60)
			if got := oturumCerezi_(t, w).Secure; got != c.beklenenSecure {
				t.Errorf("Secure = %v, beklenen %v", got, c.beklenenSecure)
			}
		})
	}
}

// TestOturumCerezSilAyniKapsam: silme çerezi, yazma çerezi ile AYNI
// Path/SameSite üçlüsünü kullanmalı. Tutmazsa tarayıcı eski çerezi yerinde
// bırakıp yanına ikincisini yazar ve kullanıcı çıkış yapamaz.
func TestOturumCerezSilAyniKapsam(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cikis", nil)
	r.Header.Set("X-Forwarded-Proto", "https")

	wYaz := httptest.NewRecorder()
	OturumCerezYaz(wYaz, r, "t", 3600)
	yazilan := oturumCerezi_(t, wYaz)

	wSil := httptest.NewRecorder()
	OturumCerezSil(wSil, r)
	silinen := oturumCerezi_(t, wSil)

	if silinen.Path != yazilan.Path {
		t.Errorf("Path ayrışıyor: sil=%q yaz=%q", silinen.Path, yazilan.Path)
	}
	if silinen.SameSite != yazilan.SameSite {
		t.Errorf("SameSite ayrışıyor: sil=%v yaz=%v", silinen.SameSite, yazilan.SameSite)
	}
	if silinen.Secure != yazilan.Secure {
		t.Errorf("Secure ayrışıyor: sil=%v yaz=%v", silinen.Secure, yazilan.Secure)
	}
	if !silinen.HttpOnly {
		t.Error("silme çerezi HttpOnly değil")
	}
	if silinen.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, çerezi geçersiz kılmak için negatif olmalı", silinen.MaxAge)
	}
	if silinen.Value != "" {
		t.Errorf("Value = %q, boş olmalı", silinen.Value)
	}
}

func TestOturumCerezDegeri(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if got := OturumCerezDegeri(r); got != "" {
		t.Errorf("çerezsiz istekte %q döndü, boş olmalı", got)
	}
	r.AddCookie(&http.Cookie{Name: OturumCerezAdi, Value: "abc"})
	if got := OturumCerezDegeri(r); got != "abc" {
		t.Errorf("OturumCerezDegeri = %q, beklenen \"abc\"", got)
	}
}
