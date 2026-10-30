package auth_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"

	"sanalcp/internal/auth"
	"sanalcp/internal/musteri"
)

const loginTOTPSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
const loginTOTPQuery = `SELECT totp_enabled, totp_secret, totp_last_step, auth_version FROM users WHERE id=\?`
const loginTOTPConsume = `UPDATE users SET totp_last_step=\? WHERE id=\? AND totp_enabled=1 AND totp_secret=\? AND totp_last_step<\? AND auth_version=\?`

func loginTOTPCode(step int64) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(loginTOTPSecret)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	n := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}

func expectLoginIdentity(mock sqlmock.Sqlmock, role, hash string) {
	if role == "user" {
		mock.ExpectQuery(`SELECT id, password_hash, role, status, auth_version FROM users WHERE username=\?`).
			WithArgs("hesap").
			WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash", "role", "status", "auth_version"}).
				AddRow(42, hash, role, "active", 3))
		return
	}
	mock.ExpectQuery(`SELECT id, username, password_hash, role, status, full_name FROM users WHERE username=\?`).
		WithArgs("hesap").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "role", "status", "full_name"}).
			AddRow(42, "hesap", hash, role, "active", "Örnek Hesap"))
}

func expectLoginSuccess(mock sqlmock.Sqlmock, role string) {
	if role == "user" {
		mock.ExpectQuery(`SELECT d.id, d.alan_adi`).WithArgs(int64(42)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "alan_adi"}).AddRow(7, "ornek.com"))
	}
	mock.ExpectExec(`INSERT INTO audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET last_login_at=NOW\(\), last_login_ip=\?, last_activity_at=NOW\(\) WHERE id=\?`).
		WithArgs("192.0.2.1", int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
}

func loginConsumer(db *sql.DB, secret []byte, role string) http.HandlerFunc {
	if role == "user" {
		return (&musteri.Handlers{DB: db, Secret: secret}).Login
	}
	return (&auth.Handlers{DB: db, Secret: secret, LifetimeSec: 3600}).Login
}

func assertLoginSession(t *testing.T, rec *httptest.ResponseRecorder, secret []byte, role string, success bool) {
	t.Helper()
	if !success {
		if cookies := rec.Header().Values("Set-Cookie"); len(cookies) != 0 {
			t.Fatalf("reddedilen/challenge giriş çerez üretti: %v", cookies)
		}
		return
	}
	var raw string
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.OturumCerezAdi && cookie.HttpOnly {
			raw = cookie.Value
		}
	}
	claims, err := auth.Parse(secret, raw)
	if err != nil {
		t.Fatalf("başarılı giriş oturumu doğrulanamadı: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "hesap" || claims.Role != role || claims.Version != 3 {
		t.Fatalf("oturum kimliği/sürümü yanlış: %+v", claims)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, leaked := body["token"]; leaked {
		t.Fatal("JWT yanıt gövdesinde döndü")
	}
	if body["bitis"] == nil || body["iki_fa_gerekli"] != nil {
		t.Fatalf("başarılı giriş yanıtı yanlış: %v", body)
	}
	if role == "user" && (body["domain_id"] != float64(7) || body["alan_adi"] != "ornek.com") {
		t.Fatalf("müşteri hizmet yönlendirmesi kayboldu: %v", body)
	}
}

func TestLoginConsumersTOTP(t *testing.T) {
	t.Setenv("PANEL_SURUM_KONTROL", "0")
	hash, err := bcrypt.GenerateFromPassword([]byte("dogru-parola"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("login-test-secret")
	for _, role := range []string{"user", "admin", "reseller"} {
		for _, scenario := range []string{
			"kapali", "challenge", "yanlis", "replay", "bozuk-secret", "bos-secret",
			"bozuk-etkinlik", "bozuk-adim", "okuma-hatasi", "gecerli", "yazma-hatasi",
			"sonuc-hatasi", "yarisi-kaybetti",
		} {
			t.Run(role+"/"+scenario, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				expectLoginIdentity(mock, role, string(hash))
				step := time.Now().Unix() / 30
				code := loginTOTPCode(step)
				enabled, storedSecret, lastStep := 1, loginTOTPSecret, int64(0)
				status, success := http.StatusOK, false
				switch scenario {
				case "kapali":
					enabled, code, success = 0, "", true
				case "challenge":
					code = ""
				case "yanlis":
					code, status = "invalid", http.StatusUnauthorized
				case "replay":
					lastStep, status = step+1, http.StatusUnauthorized
				case "bozuk-secret":
					storedSecret, code, status = "not-a-base32-secret!", "", http.StatusInternalServerError
				case "bos-secret":
					storedSecret, code, status = "", "", http.StatusInternalServerError
				case "bozuk-etkinlik":
					enabled, status = 2, http.StatusInternalServerError
				case "bozuk-adim":
					lastStep, status = -1, http.StatusInternalServerError
				case "okuma-hatasi", "yazma-hatasi", "sonuc-hatasi":
					status = http.StatusInternalServerError
				case "gecerli":
					success = true
				case "yarisi-kaybetti":
					status = http.StatusUnauthorized
				}
				query := mock.ExpectQuery(loginTOTPQuery).WithArgs(int64(42))
				if scenario == "okuma-hatasi" {
					query.WillReturnError(errors.New("database unavailable"))
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"totp_enabled", "totp_secret", "totp_last_step", "auth_version"}).
						AddRow(enabled, storedSecret, lastStep, 3))
				}
				switch scenario {
				case "gecerli", "yazma-hatasi", "sonuc-hatasi", "yarisi-kaybetti":
					consume := mock.ExpectExec(loginTOTPConsume).
						WithArgs(step, int64(42), loginTOTPSecret, step, uint64(3))
					switch scenario {
					case "yazma-hatasi":
						consume.WillReturnError(errors.New("database read only"))
					case "sonuc-hatasi":
						consume.WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected unavailable")))
					case "yarisi-kaybetti":
						consume.WillReturnResult(sqlmock.NewResult(0, 0))
					default:
						consume.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				if status == http.StatusUnauthorized {
					mock.ExpectExec(`INSERT INTO audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
				}
				if success {
					expectLoginSuccess(mock, role)
				}
				body, err := json.Marshal(map[string]string{"kullanici": "hesap", "parola": "dogru-parola", "kod": code})
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				loginConsumer(db, secret, role)(rec, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(string(body))))
				if rec.Code != status {
					t.Fatalf("status=%d, beklenen=%d, gövde=%s", rec.Code, status, rec.Body.String())
				}
				assertLoginSession(t, rec, secret, role, success)
				if scenario == "challenge" && strings.TrimSpace(rec.Body.String()) != `{"iki_fa_gerekli":true}` {
					t.Fatalf("challenge oturum/kişisel veri içermemeli: %s", rec.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestLoginConsumersConcurrentReplay(t *testing.T) {
	t.Setenv("PANEL_SURUM_KONTROL", "0")
	hash, err := bcrypt.GenerateFromPassword([]byte("dogru-parola"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "admin", "reseller"} {
		t.Run(role, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.MatchExpectationsInOrder(false)
			step := time.Now().Unix() / 30
			for range 2 {
				expectLoginIdentity(mock, role, string(hash))
				// İki istek de aynı eski adımı görür: okuma tarafındaki replay
				// kontrolü yarışı çözemez; yalnız koşullu yazımın kazananı girmeli.
				mock.ExpectQuery(loginTOTPQuery).WithArgs(int64(42)).
					WillDelayFor(10 * time.Millisecond).
					WillReturnRows(sqlmock.NewRows([]string{"totp_enabled", "totp_secret", "totp_last_step", "auth_version"}).
						AddRow(1, loginTOTPSecret, 0, 3))
			}
			mock.ExpectExec(loginTOTPConsume).WithArgs(step, int64(42), loginTOTPSecret, step, uint64(3)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(loginTOTPConsume).WithArgs(step, int64(42), loginTOTPSecret, step, uint64(3)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			expectLoginSuccess(mock, role)
			mock.ExpectExec(`INSERT INTO audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

			secret := []byte("login-test-secret")
			body := fmt.Sprintf(`{"kullanici":"hesap","parola":"dogru-parola","kod":%q}`, loginTOTPCode(step))
			handler := loginConsumer(db, secret, role)
			var wg sync.WaitGroup
			recorders := [2]*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
			start := make(chan struct{})
			for _, rec := range recorders {
				wg.Go(func() {
					<-start
					handler(rec, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body)))
				})
			}
			close(start)
			wg.Wait()
			successes := 0
			for _, rec := range recorders {
				if rec.Code == http.StatusOK {
					successes++
				} else if rec.Code != http.StatusUnauthorized {
					t.Fatalf("beklenmeyen sonuç: %d %s", rec.Code, rec.Body.String())
				}
				assertLoginSession(t, rec, secret, role, rec.Code == http.StatusOK)
			}
			if successes != 1 {
				t.Fatalf("aynı kodla %d oturum açıldı, beklenen 1", successes)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
