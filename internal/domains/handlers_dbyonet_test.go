package domains

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDatabaseIsimDegistirGecersizSonekiReddeder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Put("/domains/{id}/databases/{dbAdi}/isim", h.DatabaseIsimDegistir)

	body := strings.NewReader(`{"yeni_sonek":"Buyuk Harf Ve Bosluk"}`)
	req := httptest.NewRequest(http.MethodPut, "/domains/7/databases/sk_blog/isim", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("400 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
}

func TestDatabaseIsimDegistirCakismaVarsa409Doner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE db_name=\?`).
		WithArgs("sk_yeni").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Put("/domains/{id}/databases/{dbAdi}/isim", h.DatabaseIsimDegistir)

	body := strings.NewReader(`{"yeni_sonek":"yeni"}`)
	req := httptest.NewRequest(http.MethodPut, "/domains/7/databases/sk_blog/isim", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDatabaseIsimDegistirCakismaSorguHatasi500Doner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE db_name=\?`).
		WithArgs("sk_yeni").
		WillReturnError(sql.ErrConnDone)

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Put("/domains/{id}/databases/{dbAdi}/isim", h.DatabaseIsimDegistir)

	body := strings.NewReader(`{"yeni_sonek":"yeni"}`)
	req := httptest.NewRequest(http.MethodPut, "/domains/7/databases/sk_blog/isim", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("500 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDatabaseKullaniciEkleMevcutKullaniciDomaineAitDegilse400(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Post("/domains/{id}/databases/{dbAdi}/kullanicilar", h.DatabaseKullaniciEkle)

	body := strings.NewReader(`{"kullanici_tipi":"mevcut","mevcut_kullanici":"sk_baska"}`)
	req := httptest.NewRequest(http.MethodPost, "/domains/7/databases/sk_blog/kullanicilar", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("400 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
}

func TestDatabaseKullaniciEkleZatenErisimiVarsa409(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_blog", "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Post("/domains/{id}/databases/{dbAdi}/kullanicilar", h.DatabaseKullaniciEkle)

	body := strings.NewReader(`{"kullanici_tipi":"mevcut","mevcut_kullanici":"sk_baska"}`)
	req := httptest.NewRequest(http.MethodPost, "/domains/7/databases/sk_blog/kullanicilar", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSonKullaniciMi(t *testing.T) {
	if !sonKullaniciMi(1) {
		t.Error("1 kullanıcı = son kullanıcı olmalı")
	}
	if !sonKullaniciMi(0) {
		t.Error("0 kullanıcı = son kullanıcı (veri tutarsızlığına karşı güvenli taraf) olmalı")
	}
	if sonKullaniciMi(2) {
		t.Error("2 kullanıcı = son kullanıcı DEĞİL")
	}
}

func TestDatabaseKullaniciSilSonKullaniciysa409(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT db_user FROM db_accounts WHERE id=\? AND domain_id=\? AND db_name=\?`).
		WithArgs(int64(101), int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Delete("/domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}", h.DatabaseKullaniciSil)

	req := httptest.NewRequest(http.MethodDelete, "/domains/7/databases/sk_blog/kullanicilar/101", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
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
