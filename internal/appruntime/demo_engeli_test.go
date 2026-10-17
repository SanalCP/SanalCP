package appruntime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalcp/internal/middleware"
)

func TestLogs_DemoModundaEngellenir(t *testing.T) {
	h := &Handlers{}

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/1/app-runtime/logs", nil), true)
	w := httptest.NewRecorder()
	h.Logs(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda uygulama logu engellenmedi: code=%d", w.Code)
	}
}
