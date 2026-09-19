package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIslemZamanAsimiRotaBazli(t *testing.T) {
	tests := []struct {
		method, path string
		uzun         bool
	}{
		{http.MethodGet, "/api/v1/domains/7/files/indir", true},
		{http.MethodPost, "/api/v1/domains/7/files/upload", true},
		{http.MethodPost, "/api/v1/domains/7/backups/9/geriyukle", true},
		{http.MethodPost, "/api/v1/domains/7/backups/9/dogrula", true},
		{http.MethodPost, "/api/v1/admin/transfers/import", true},
		{http.MethodGet, "/api/v1/domains/7/files", false},
		{http.MethodDelete, "/api/v1/domains/7/backups/9/indir", false},
		{http.MethodPost, "/unrelated/backups/9/geriyukle", false},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var kalan time.Duration
			h := IslemZamanAsimi(time.Minute, 30*time.Minute)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				deadline, ok := r.Context().Deadline()
				if !ok {
					t.Fatal("deadline yok")
				}
				kalan = time.Until(deadline)
			}))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.path, nil))
			if got := kalan > 20*time.Minute; got != tt.uzun {
				t.Fatalf("uzun=%v, kalan=%v", got, kalan)
			}
		})
	}
}

func TestIslemZamanAsimiContextiIptalEder(t *testing.T) {
	var done <-chan struct{}
	h := IslemZamanAsimi(time.Minute, 30*time.Minute)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		done = r.Context().Done()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	select {
	case <-done:
	default:
		t.Fatal("handler tamamlanınca context iptal edilmedi")
	}
}
