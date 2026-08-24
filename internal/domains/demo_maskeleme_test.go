package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

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

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
