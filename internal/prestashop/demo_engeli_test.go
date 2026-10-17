package prestashop

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalcp/internal/middleware"
)

func TestLoglar_DemoModundaEngellenir(t *testing.T) {
	h := &Handlers{}

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/1/prestashop/loglar", nil), true)
	w := httptest.NewRecorder()
	h.Loglar(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda PrestaShop logu engellenmedi: code=%d", w.Code)
	}
}
