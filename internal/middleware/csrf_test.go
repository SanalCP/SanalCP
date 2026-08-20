package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func csrfSunucu() http.Handler {
	return CSRFKoruma(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func istek(t *testing.T, metot, host string, basliklar map[string]string) int {
	t.Helper()
	r := httptest.NewRequest(metot, "http://"+host+"/api/v1/domains", nil)
	r.Host = host
	for k, v := range basliklar {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	csrfSunucu().ServeHTTP(w, r)
	return w.Code
}

func TestCSRFGuvenliMetotlarMuaf(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		// Yabancı origin bile olsa okuma uçları engellenmez.
		if kod := istek(t, m, "panel.example.com", map[string]string{
			"Origin": "https://kotu.example.net",
		}); kod != http.StatusOK {
			t.Fatalf("%s: beklenen 200, alınan %d", m, kod)
		}
	}
}

func TestCSRFYabanciOriginReddedilir(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if kod := istek(t, m, "panel.example.com", map[string]string{
			"Origin": "https://kotu.example.net",
		}); kod != http.StatusForbidden {
			t.Fatalf("%s: yabancı origin geçti, alınan %d", m, kod)
		}
	}
}

func TestCSRFAyniOriginGecer(t *testing.T) {
	// Tarayıcı portlu Origin gönderir; nginx `Host $host` ile portsuz host verir.
	// Bu kombinasyonun geçmesi şart — aksi halde panelin tamamı kilitlenir.
	if kod := istek(t, http.MethodPost, "panel.example.com", map[string]string{
		"Origin": "https://panel.example.com:8443",
	}); kod != http.StatusOK {
		t.Fatalf("aynı host farklı port reddedildi: %d", kod)
	}
	if kod := istek(t, http.MethodPost, "panel.example.com:8443", map[string]string{
		"Origin": "https://panel.example.com:8443",
	}); kod != http.StatusOK {
		t.Fatalf("birebir aynı origin reddedildi: %d", kod)
	}
}

func TestCSRFBasliksizIstekGecer(t *testing.T) {
	// curl / API token'lı otomasyon / git webhook: Origin de Referer da yok.
	if kod := istek(t, http.MethodPost, "panel.example.com", nil); kod != http.StatusOK {
		t.Fatalf("Origin'siz istek reddedildi (otomasyon kırılır): %d", kod)
	}
}

func TestCSRFRefererFallback(t *testing.T) {
	if kod := istek(t, http.MethodPost, "panel.example.com", map[string]string{
		"Referer": "https://kotu.example.net/tuzak.html",
	}); kod != http.StatusForbidden {
		t.Fatalf("yabancı Referer geçti: %d", kod)
	}
	if kod := istek(t, http.MethodPost, "panel.example.com", map[string]string{
		"Referer": "https://panel.example.com:8443/domains",
	}); kod != http.StatusOK {
		t.Fatalf("kendi Referer'ı reddedildi: %d", kod)
	}
}

func TestCSRFBozukOriginReddedilir(t *testing.T) {
	for _, o := range []string{"null", "://", "panel.example.com"} {
		if kod := istek(t, http.MethodPost, "panel.example.com", map[string]string{
			"Origin": o,
		}); kod != http.StatusForbidden {
			t.Fatalf("bozuk Origin %q geçti: %d", o, kod)
		}
	}
}

func TestCSRFEnvIleBeyanEdilenOrigin(t *testing.T) {
	t.Setenv("PANEL_CSRF_ORIGIN", "https://panel.example.com:8443, yedek.example.com")
	h := CSRFKoruma(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cagir := func(origin string) int {
		r := httptest.NewRequest(http.MethodPost, "http://baska.example.org/api/v1/domains", nil)
		r.Host = "baska.example.org" // env verildiğinde r.Host DEĞİL liste geçerlidir
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if kod := cagir("https://panel.example.com:8443"); kod != http.StatusOK {
		t.Fatalf("beyan edilen origin reddedildi: %d", kod)
	}
	if kod := cagir("https://yedek.example.com"); kod != http.StatusOK {
		t.Fatalf("beyan edilen ikinci origin reddedildi: %d", kod)
	}
	if kod := cagir("https://baska.example.org"); kod != http.StatusForbidden {
		t.Fatalf("env verildiğinde r.Host'a düşülmemeli: %d", kod)
	}
}

func TestCSRFIPv6Host(t *testing.T) {
	if kod := istek(t, http.MethodPost, "[2001:db8::1]:8443", map[string]string{
		"Origin": "https://[2001:db8::1]:8443",
	}); kod != http.StatusOK {
		t.Fatalf("IPv6 aynı origin reddedildi: %d", kod)
	}
}
