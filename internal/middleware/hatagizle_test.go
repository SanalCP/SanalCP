package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
)

func hataGizleCalistir(t *testing.T, rol string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/x", nil)
	if rol != "" {
		r = ClaimsIle(r, &auth.Claims{UserID: 7, Role: rol})
	}
	rec := httptest.NewRecorder()
	HataAyrintisiGizle(h).ServeHTTP(rec, r)
	return rec
}

const icAyrinti = "Error 1146: Table 'panel.gizli_tablo' doesn't exist"

func icHata(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusInternalServerError, "DB: "+icAyrinti)
}

func TestHataGizleMusteriGenelMesajAlir(t *testing.T) {
	for _, rol := range []string{RolMusteri, RolBayi} {
		rec := hataGizleCalistir(t, rol, icHata)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: durum %d, beklenen 500", rol, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "gizli_tablo") {
			t.Fatalf("%s: iç hata ayrıntısı sızdı: %s", rol, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "sunucu tarafında") {
			t.Fatalf("%s: genel mesaj yok: %s", rol, rec.Body.String())
		}
	}
}

func TestHataGizleAdminAyrintiyiGorur(t *testing.T) {
	rec := hataGizleCalistir(t, RolAdmin, icHata)
	if !strings.Contains(rec.Body.String(), "gizli_tablo") {
		t.Fatalf("admin ayrıntıyı görmeli: %s", rec.Body.String())
	}
}

func TestHataGizle4xxVeBasariDokunulmaz(t *testing.T) {
	rec := hataGizleCalistir(t, RolMusteri, func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz alan adı")
	})
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "geçersiz alan adı") {
		t.Fatalf("4xx mesajı korunmalı: %d %s", rec.Code, rec.Body.String())
	}
	rec = hataGizleCalistir(t, RolMusteri, func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("başarılı yanıt korunmalı: %d %s", rec.Code, rec.Body.String())
	}
}

// Örtük 200 (WriteHeader çağrılmadan Write) ve SSE flush'ı bozulmamalı.
func TestHataGizleAkisVeFlush(t *testing.T) {
	rec := hataGizleCalistir(t, RolMusteri, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: 1\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush başarısız: %v", err)
		}
	})
	if rec.Code != 200 || !rec.Flushed || rec.Body.String() != "data: 1\n\n" {
		t.Fatalf("akış bozuldu: kod=%d flushed=%v gövde=%q", rec.Code, rec.Flushed, rec.Body.String())
	}
}
