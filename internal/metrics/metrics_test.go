package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMiddlewareVeHandler(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/domains/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/domains/123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("beklenmeyen durum kodu: %d", rr.Code)
	}

	mreq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	mrr := httptest.NewRecorder()
	Handler().ServeHTTP(mrr, mreq)
	if mrr.Code != http.StatusOK {
		t.Fatalf("/metrics beklenmeyen durum kodu: %d", mrr.Code)
	}
	gövde := mrr.Body.String()
	if !strings.Contains(gövde, `sanalcp_http_istekleri_toplam{durum="200",yol="/domains/{id}",yontem="GET"} 1`) {
		t.Fatalf("beklenen sayaç bulunamadı; çıktı:\n%s", gövde)
	}
	if !strings.Contains(gövde, "sanalcp_http_istek_suresi_saniye") {
		t.Fatalf("beklenen histogram bulunamadı; çıktı:\n%s", gövde)
	}
}

// Canlı günlük (SSE) http.ResponseController ile flush eder; sarmalayıcı
// Unwrap sağlamazsa flush sessizce başarısız olur ve akış kesilir.
func TestMiddlewareFlushGecirir(t *testing.T) {
	var hata error
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: x\n\n"))
		hata = http.NewResponseController(w).Flush()
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if hata != nil || !rec.Flushed {
		t.Fatalf("flush metrics sarmalayıcısından geçmedi: err=%v flushed=%v", hata, rec.Flushed)
	}
}
