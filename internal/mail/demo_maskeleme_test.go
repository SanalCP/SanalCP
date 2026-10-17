package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestParolaGoster_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT sistem_kullanici, COALESCE\(is_demo,0\) FROM domains`).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("c_ornek", 0))
	mock.ExpectQuery(`SELECT 1 FROM mailboxes WHERE id=\? AND domain_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	token := revealSakla(9, "gercek-parola")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("mid", "9")
	rctx.URLParams.Add("token", token)

	r := httptest.NewRequest(http.MethodGet, "/domains/5/mail/9/parola-reveal/"+token, nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = middleware.DemoIle(r, true)

	w := httptest.NewRecorder()
	h.ParolaGoster(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"parola":"••••••••"}`+"\n" {
		t.Fatalf("beklenmeyen gövde: %q", got)
	}
}
