package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestListDatabases_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT id, domain_id, db_name, db_user, db_host, db_pass_plain`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "domain_id", "db_name", "db_user", "db_host", "db_pass_plain", "created_at"}).
			AddRow(1, 5, "ornek_db", "ornek_user", "localhost", "sifrelenmis-deger", "2026-08-24 10:00"))

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/5/databases", nil), true)
	w := httptest.NewRecorder()
	h.ListDatabases(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !contains(got, `"db_parola":"••••••••"`) {
		t.Fatalf("parola maskelenmedi: %s", got)
	}
}

func TestDatabaseGrupDetay_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT id, db_user, db_host, db_pass_plain`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "db_user", "db_host", "db_pass_plain", "created_at"}).
			AddRow(int64(1), "sk_blog", "localhost", "gercek-parola", "2026-08-25 10:00"))

	rtr := chi.NewRouter()
	rtr.Get("/domains/{id}/databases/{dbAdi}", h.DatabaseGrupDetay)
	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/7/databases/sk_blog", nil), true)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	got := w.Body.String()
	if contains(got, "gercek-parola") || !contains(got, `"db_parola":"••••••••"`) {
		t.Fatalf("parola maskelenmedi: %s", got)
	}
}
