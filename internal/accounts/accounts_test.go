package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

func hesapKur(t *testing.T) (*Handlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Handlers{DB: db}, mock
}

func musteriIstegi(url, rol string, uid int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, url, nil)
	return middleware.ClaimsIle(r, &auth.Claims{UserID: uid, Role: rol})
}

func TestListCustomersLegacyArrayVeCountYok(t *testing.T) {
	h, mock := hesapKur(t)
	mock.ExpectQuery(`(?s)^SELECT id, ad,.*FROM customers ORDER BY ad, id$`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rec := httptest.NewRecorder()
	h.ListCustomers(rec, musteriIstegi("/customers?x=1", middleware.RolAdmin, 1))

	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("kod=%d govde=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListCustomersSayfaliBayiScopeCountVeOffset(t *testing.T) {
	h, mock := hesapKur(t)
	desen := "%firma%"
	mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM customers WHERE owner_user_id = \? AND \(ad LIKE \? ESCAPE '!' OR eposta LIKE \? ESCAPE '!' OR notlar LIKE \? ESCAPE '!'\)$`).
		WithArgs(int64(12), desen, desen, desen).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(11))
	mock.ExpectQuery(`(?s)^SELECT id, ad,.*FROM customers WHERE owner_user_id = \? AND \(ad LIKE \? ESCAPE '!' OR eposta LIKE \? ESCAPE '!' OR notlar LIKE \? ESCAPE '!'\) ORDER BY ad, id LIMIT \? OFFSET \?$`).
		WithArgs(int64(12), desen, desen, desen, 5, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rec := httptest.NewRecorder()
	h.ListCustomers(rec, musteriIstegi("/customers?sayfa=3&limit=5&arama=%20firma%20", middleware.RolBayi, 12))

	var got struct {
		Icerik      []Customer `json:"icerik"`
		Sayfa       int        `json:"sayfa"`
		Limit       int        `json:"limit"`
		Toplam      int64      `json:"toplam"`
		ToplamSayfa int64      `json:"toplam_sayfa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got.Icerik == nil || got.Sayfa != 3 || got.Limit != 5 || got.Toplam != 11 || got.ToplamSayfa != 3 {
		t.Fatalf("kod=%d yanit=%+v govde=%s", rec.Code, got, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
