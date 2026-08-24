package genelbakis

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/middleware"
)

func TestVeritabanlari_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT a.id, a.domain_id, d.alan_adi, a.db_name, a.db_user, a.db_host, a.db_pass_plain`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "domain_id", "alan_adi", "db_name", "db_user", "db_host", "db_pass_plain", "created_at"}).
			AddRow(1, 5, "ornek.com", "ornek_db", "ornek_user", "localhost", "sifrelenmis-deger", "2026-08-24"))

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/genelbakis/veritabanlari", nil), true)
	w := httptest.NewRecorder()
	h.Veritabanlari(w, r)

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
