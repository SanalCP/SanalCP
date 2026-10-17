package laravel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalcp/internal/middleware"
)

func TestEnvGet_DemoModundaEngellenir(t *testing.T) {
	h := &Handlers{}

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/1/laravel/env", nil), true)
	w := httptest.NewRecorder()
	h.EnvGet(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda .env içeriği engellenmedi: code=%d", w.Code)
	}
}
