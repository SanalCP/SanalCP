package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

func listeIstegi(url, rol string, uid int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, url, nil)
	return middleware.ClaimsIle(r, &auth.Claims{UserID: uid, Role: rol})
}

func TestListeLegacyArrayVeCountYok(t *testing.T) {
	h, mock := kur(t)
	mock.ExpectQuery(`(?s)^SELECT id, username,.*FROM users ORDER BY username, id$`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rec := httptest.NewRecorder()
	h.Liste(rec, listeIstegi("/users?x=1", middleware.RolAdmin, 1))

	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("kod=%d govde=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListeSayfaliBayiScopeCountVeOffset(t *testing.T) {
	h, mock := kur(t)
	desen := "%ali!_%"
	mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM users WHERE reseller_id = \? AND \(username LIKE \? ESCAPE '!' OR email LIKE \? ESCAPE '!' OR full_name LIKE \? ESCAPE '!'\)$`).
		WithArgs(int64(9), desen, desen, desen).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(4))
	mock.ExpectQuery(`(?s)^SELECT id, username,.*FROM users WHERE reseller_id = \? AND \(username LIKE \? ESCAPE '!' OR email LIKE \? ESCAPE '!' OR full_name LIKE \? ESCAPE '!'\) ORDER BY username, id LIMIT \? OFFSET \?$`).
		WithArgs(int64(9), desen, desen, desen, 2, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rec := httptest.NewRecorder()
	h.Liste(rec, listeIstegi("/users?sayfa=2&limit=2&arama=ali_", middleware.RolBayi, 9))

	var got struct {
		Icerik      []KullaniciSatir `json:"icerik"`
		Sayfa       int              `json:"sayfa"`
		Limit       int              `json:"limit"`
		Toplam      int64            `json:"toplam"`
		ToplamSayfa int64            `json:"toplam_sayfa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got.Icerik == nil || got.Sayfa != 2 || got.Limit != 2 || got.Toplam != 4 || got.ToplamSayfa != 2 {
		t.Fatalf("kod=%d yanit=%+v govde=%s", rec.Code, got, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
