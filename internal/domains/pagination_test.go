package domains

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

func TestListLegacyArrayVeCountYok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`(?s)^SELECT d\.id,.*ORDER BY d\.alan_adi, d\.id$`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	req := httptest.NewRequest(http.MethodGet, "/domains?bilinmeyen=1", nil)
	req = middleware.ClaimsIle(req, &auth.Claims{UserID: 1, Role: middleware.RolAdmin})
	rec := httptest.NewRecorder()
	(&Handlers{DB: db}).List(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("kod=%d govde=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListSayfaliBayiScopeCountVeOffset(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	countRe := `(?s)^SELECT COUNT\(\*\) FROM domains d.*WHERE EXISTS \(SELECT 1 FROM customers kc WHERE kc\.id = d\.customer_id AND kc\.owner_user_id = \?\) AND \(d\.alan_adi LIKE \? ESCAPE '!' OR.*COALESCE\(cu\.ad,''\) LIKE \? ESCAPE '!'\)$`
	listeRe := `(?s)^SELECT d\.id,.*WHERE EXISTS \(SELECT 1 FROM customers kc WHERE kc\.id = d\.customer_id AND kc\.owner_user_id = \?\) AND \(d\.alan_adi LIKE \? ESCAPE '!' OR.*ORDER BY d\.alan_adi, d\.id LIMIT \? OFFSET \?$`
	desen := "%ornek!%%"
	mock.ExpectQuery(countRe).
		WithArgs(int64(7), desen, desen, desen, desen).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(61))
	mock.ExpectQuery(listeRe).
		WithArgs(int64(7), desen, desen, desen, desen, 25, 25).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(http.MethodGet, "/domains?sayfa=2&limit=25&arama=%20ornek%25%20", nil)
	req = middleware.ClaimsIle(req, &auth.Claims{UserID: 7, Role: middleware.RolBayi})
	rec := httptest.NewRecorder()
	(&Handlers{DB: db}).List(rec, req)

	var got struct {
		Icerik      []Domain `json:"icerik"`
		Sayfa       int      `json:"sayfa"`
		Limit       int      `json:"limit"`
		Toplam      int64    `json:"toplam"`
		ToplamSayfa int64    `json:"toplam_sayfa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got.Icerik == nil || got.Sayfa != 2 || got.Limit != 25 || got.Toplam != 61 || got.ToplamSayfa != 3 {
		t.Fatalf("kod=%d yanit=%+v govde=%s", rec.Code, got, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
