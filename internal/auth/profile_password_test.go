package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestParolaDegistir_PanelHesabiniGuncellerVeOturumuSiler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	eskiHash, err := bcrypt.GenerateFromPassword([]byte("eski-parola-12"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT password_hash FROM users WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(eskiHash)))
	mock.ExpectExec(`UPDATE users SET password_hash=\?, auth_version=auth_version\+1, updated_at=NOW\(\) WHERE id=\?`).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db, Secret: []byte("test"), LifetimeSec: 3600}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/parola",
		strings.NewReader(`{"mevcut":"eski-parola-12","yeni":"yeni-parola-34"}`))
	req = req.WithContext(ClaimsContext(req.Context(),
		&Claims{UserID: 7, Username: "admin", Role: "admin"}))
	w := httptest.NewRecorder()

	h.ParolaDegistir(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	var silindi bool
	for _, c := range w.Result().Cookies() {
		if c.Name == OturumCerezAdi && c.MaxAge < 0 && c.Value == "" {
			silindi = true
		}
	}
	if !silindi {
		t.Fatalf("başarılı parola değişiminde %q çerezi silinmedi", OturumCerezAdi)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
