package musteri

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

// Başarılı müşteri girişi last_activity_at'i sıfırlamalı. Aksi hâlde hesabın
// son isteği oturum_bosta_dakika limitinden eskiyse parola doğru kabul edilip
// çerez yazılır, ardından SPA'nın ilk yetkili isteği "oturum boşta kaldığı
// için sona erdi" 401'i alır ve kullanıcı login ekranına geri düşer.
func TestLogin_BasariliGirisBostaSayacini_Sifirlar(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("dogru-parola"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, password_hash, role, status, auth_version FROM users WHERE username=\?`).
		WithArgs("c_ornek_com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash", "role", "status", "auth_version"}).
			AddRow(42, string(hash), "user", "active", 3))
	mock.ExpectQuery(`SELECT d.id, d.alan_adi`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "alan_adi"}).AddRow(7, "ornek.com"))
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET last_login_at=NOW\(\), last_login_ip=\?, last_activity_at=NOW\(\) WHERE id=\?`).
		WithArgs("192.0.2.1", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := &Handlers{DB: db, Secret: []byte("test")}
	govde := strings.NewReader(`{"kullanici":"c_ornek_com","parola":"dogru-parola"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/musteri/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d (gövde: %s)", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
