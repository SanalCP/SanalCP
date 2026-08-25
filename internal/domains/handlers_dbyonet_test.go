package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func TestDatabaseGrupDetayBirdenFazlaKullaniciyiGruplar(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "db_user", "db_host", "db_pass_plain", "olusturulma"}).
		AddRow(int64(1), "sk_blog", "localhost", "", "2026-08-25 10:00").
		AddRow(int64(2), "sk_ikinci", "localhost", "", "2026-08-25 11:00")
	mock.ExpectQuery(`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT\(created_at,'%Y-%m-%d %H:%i'\)\s+FROM db_accounts WHERE domain_id=\? AND db_name=\? ORDER BY id`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT SUM\(data_length\+index_length\)/1024/1024 FROM information_schema.tables WHERE table_schema=\?`).
		WithArgs("sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"boyut"}).AddRow(2.5))
	mock.ExpectQuery(`SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name=\?`).
		WithArgs("sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"cs", "co"}).AddRow("utf8mb4", "utf8mb4_unicode_ci"))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Get("/domains/{id}/databases/{dbAdi}", h.DatabaseGrupDetay)

	req := httptest.NewRequest(http.MethodGet, "/domains/7/databases/sk_blog", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("200 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, `"db_adi":"sk_blog"`) || !contains(body, "sk_ikinci") {
		t.Errorf("iki kullanıcı da yanıtta olmalıydı: %s", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDatabaseGrupDetayBulunamayanDB404Doner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT\(created_at,'%Y-%m-%d %H:%i'\)\s+FROM db_accounts WHERE domain_id=\? AND db_name=\? ORDER BY id`).
		WithArgs(int64(7), "yok_boyle_db").
		WillReturnRows(sqlmock.NewRows([]string{"id", "db_user", "db_host", "db_pass_plain", "olusturulma"}))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Get("/domains/{id}/databases/{dbAdi}", h.DatabaseGrupDetay)

	req := httptest.NewRequest(http.MethodGet, "/domains/7/databases/yok_boyle_db", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("404 bekleniyordu, %d geldi", w.Code)
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
