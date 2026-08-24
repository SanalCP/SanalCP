package backups

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestDownload_DemoModundaEngellenir(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("bid", "9")
	r := httptest.NewRequest(http.MethodGet, "/domains/5/backups/9/indir", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = middleware.DemoIle(r, true)

	w := httptest.NewRecorder()
	h.Download(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda indirme engellenmedi: code=%d", w.Code)
	}
}
