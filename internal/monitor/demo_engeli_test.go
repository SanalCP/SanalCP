package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalcp/internal/middleware"
)

func TestProcesses_DemoModundaEngellenir(t *testing.T) {
	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/system/processes", nil), true)
	w := httptest.NewRecorder()
	Processes(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda süreç listesi engellenmedi: code=%d", w.Code)
	}
}

func TestSunucuLog_DemoModundaEngellenir(t *testing.T) {
	h := &Handlers{}

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/admin/system/loglar", nil), true)
	w := httptest.NewRecorder()
	h.SunucuLog(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda sunucu logu engellenmedi: code=%d", w.Code)
	}
}
