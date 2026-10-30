package auth_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

const enableTOTPUpdate = `UPDATE users SET totp_secret=\?, totp_enabled=1, totp_last_step=\?, auth_version=auth_version\+1 WHERE id=\? AND auth_version=\? AND totp_enabled=0`
const disableTOTPQuery = `SELECT totp_enabled, totp_secret FROM users WHERE id=\?`
const disableTOTPUpdate = `UPDATE users SET totp_secret='', totp_enabled=0, totp_last_step=0, auth_version=auth_version\+1 WHERE id=\? AND auth_version=\? AND totp_enabled=1 AND totp_secret=\?`
const passwordUpdate = `UPDATE users SET password_hash=\?, auth_version=auth_version\+1, updated_at=NOW\(\) WHERE id=\?`

// Kabul edilen argümanın gerçekten yeni parolanın bcrypt hash'i olduğunu kanıtlar.
type changedPassword string

func (p changedPassword) Match(value driver.Value) bool {
	hash, ok := value.(string)
	return ok && bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

func securityHandler(h *auth.Handlers, action string) http.HandlerFunc {
	switch action {
	case "parola":
		return h.ParolaDegistir
	case "enable":
		return h.TwoFAEnable
	case "disable":
		return h.TwoFADisable
	default:
		return h.TwoFASetup
	}
}

func securityRequest(action, body, role string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/"+action, strings.NewReader(body))
	return req.WithContext(auth.ClaimsContext(req.Context(),
		&auth.Claims{UserID: 42, Username: "hesap", Role: role, Version: 3}))
}

func assertSessionCleared(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.OturumCerezAdi && cookie.Value == "" && cookie.MaxAge < 0 {
			return
		}
	}
	t.Fatal("güvenlik değişikliği tarayıcıdaki oturumu kaldırmadı")
}

func TestSelfSecurityChangesUseSessionIdentityAndRevokeOldSessions(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("eski-parola-12"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "admin", "reseller"} {
		for _, action := range []string{"parola", "enable", "disable"} {
			t.Run(role+"/"+action, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				step := time.Now().Unix() / 30
				code := loginTOTPCode(step)
				var body string
				switch action {
				case "parola":
					body = `{"mevcut":"eski-parola-12","yeni":"yeni-parola-34","id":1,"username":"root","role":"admin"}`
					mock.ExpectQuery(`SELECT password_hash FROM users WHERE id=\?`).WithArgs(int64(42)).
						WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))
					mock.ExpectExec(passwordUpdate).WithArgs(changedPassword("yeni-parola-34"), int64(42)).
						WillReturnResult(sqlmock.NewResult(0, 1))
				case "enable":
					body = fmt.Sprintf(`{"secret":%q,"kod":%q,"id":1,"username":"root","role":"admin"}`, loginTOTPSecret, code)
					mock.ExpectExec(enableTOTPUpdate).WithArgs(loginTOTPSecret, step, int64(42), uint64(3)).
						WillReturnResult(sqlmock.NewResult(0, 1))
				case "disable":
					body = fmt.Sprintf(`{"kod":%q,"id":1,"username":"root","role":"admin"}`, code)
					mock.ExpectQuery(disableTOTPQuery).WithArgs(int64(42)).
						WillReturnRows(sqlmock.NewRows([]string{"totp_enabled", "totp_secret"}).AddRow(1, loginTOTPSecret))
					mock.ExpectExec(disableTOTPUpdate).WithArgs(int64(42), uint64(3), loginTOTPSecret).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
				mock.ExpectExec(`INSERT INTO audit_log`).
					WithArgs(int64(42), "hesap", "192.0.2.1", "auth."+map[string]string{
						"parola": "parola", "enable": "2fa.enable", "disable": "2fa.disable",
					}[action], "hesap", 1).
					WillReturnResult(sqlmock.NewResult(1, 1))

				secret := []byte("security-test-secret")
				h := &auth.Handlers{DB: db, Secret: secret, LifetimeSec: 3600}
				rec := httptest.NewRecorder()
				securityHandler(h, action)(rec, securityRequest(action, body, role))
				if rec.Code != http.StatusOK {
					t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
				}
				assertSessionCleared(t, rec)

				// Aynı kullanıcının önceki JWT'si, güncellenen auth_version ile
				// gerçek RequireAuth tüketicisinde artık kabul edilmemeli.
				oldToken, err := auth.Issue(secret, 3600, 42, "hesap", role, 3)
				if err != nil {
					t.Fatal(err)
				}
				mock.ExpectQuery(`(?s)SELECT\s+u.status,\s*u.role,\s*u.auth_version,.*FROM users u\s+WHERE u.id = \?`).
					WithArgs(int64(42)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "role", "auth_version", "idle"}).AddRow("active", role, 4, nil))
				middleware.Init(db)
				t.Cleanup(func() { middleware.Init(nil) })
				stale := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
				stale.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: oldToken})
				denied := httptest.NewRecorder()
				middleware.RequireAuth(secret)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("iptal edilmiş oturum korumalı handler'a ulaştı")
				})).ServeHTTP(denied, stale)
				if denied.Code != http.StatusUnauthorized {
					t.Fatalf("eski oturum status=%d, body=%s", denied.Code, denied.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSelfTwoFASetupUsesSessionAccount(t *testing.T) {
	for _, role := range []string{"user", "admin", "reseller"} {
		t.Run(role, func(t *testing.T) {
			rec := httptest.NewRecorder()
			(&auth.Handlers{}).TwoFASetup(rec, securityRequest("setup", `{"username":"root"}`, role))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
			}
			var result struct {
				Secret string `json:"secret"`
				URI    string `json:"otpauth_uri"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			uri, err := url.Parse(result.URI)
			if err != nil {
				t.Fatal(err)
			}
			if result.Secret == "" || uri.Query().Get("secret") != result.Secret || uri.Path != "/SanalCP:hesap" {
				t.Fatalf("setup yanlış hesaba ait: %+v", result)
			}
			if len(rec.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("setup oturumu değiştirmemeli")
			}
		})
	}
}

func TestSelfSecurityRequiresAuthenticatedClaims(t *testing.T) {
	for _, action := range []string{"parola", "setup", "enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			rec := httptest.NewRecorder()
			securityHandler(&auth.Handlers{}, action)(rec,
				httptest.NewRequest(http.MethodPost, "/me/"+action, strings.NewReader(`{"id":1,"role":"admin"}`)))
			if rec.Code != http.StatusUnauthorized || len(rec.Header().Values("Set-Cookie")) != 0 {
				t.Fatalf("kimliksiz güvenlik isteği kabul edildi: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSelfTwoFAWriteAndReadFailuresDoNotSucceed(t *testing.T) {
	for _, action := range []string{"enable", "disable"} {
		for _, scenario := range []string{"json", "kod", "yazma", "sonuc", "satir-yok", "okuma", "bozuk-secret", "kapali"} {
			if action == "enable" && (scenario == "okuma" || scenario == "bozuk-secret" || scenario == "kapali") {
				continue
			}
			t.Run(action+"/"+scenario, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				step := time.Now().Unix() / 30
				code := loginTOTPCode(step)
				status := http.StatusInternalServerError
				if scenario == "kod" {
					code = "invalid"
				}
				body := fmt.Sprintf(`{"secret":%q,"kod":%q}`, loginTOTPSecret, code)
				if scenario == "json" {
					body = "{"
					status = http.StatusBadRequest
				} else if action == "disable" {
					query := mock.ExpectQuery(disableTOTPQuery).WithArgs(int64(42))
					if scenario == "okuma" {
						query.WillReturnError(errors.New("database unavailable"))
					} else {
						enabled, storedSecret := 1, loginTOTPSecret
						if scenario == "bozuk-secret" {
							storedSecret = "invalid!"
						}
						if scenario == "kapali" {
							enabled = 0
						}
						query.WillReturnRows(sqlmock.NewRows([]string{"totp_enabled", "totp_secret"}).AddRow(enabled, storedSecret))
					}
				}
				if scenario == "kod" || scenario == "bozuk-secret" || scenario == "kapali" {
					status = http.StatusBadRequest
				}
				if scenario == "yazma" || scenario == "sonuc" || scenario == "satir-yok" {
					var update *sqlmock.ExpectedExec
					if action == "enable" {
						update = mock.ExpectExec(enableTOTPUpdate).WithArgs(loginTOTPSecret, step, int64(42), uint64(3))
					} else {
						update = mock.ExpectExec(disableTOTPUpdate).WithArgs(int64(42), uint64(3), loginTOTPSecret)
					}
					switch scenario {
					case "yazma":
						update.WillReturnError(errors.New("database read only"))
					case "sonuc":
						update.WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected unavailable")))
					case "satir-yok":
						update.WillReturnResult(sqlmock.NewResult(0, 0))
						status = http.StatusConflict
					}
				}
				rec := httptest.NewRecorder()
				securityHandler(&auth.Handlers{DB: db}, action)(rec, securityRequest(action, body, "user"))
				if rec.Code != status || len(rec.Header().Values("Set-Cookie")) != 0 {
					t.Fatalf("hatalı güvenlik değişikliği: status=%d beklenen=%d cookie=%v body=%s", rec.Code, status, rec.Header().Values("Set-Cookie"), rec.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestCustomerPasswordFailuresDoNotRevokeOrIssueSessions(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("eski-parola-12"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"yanlis", "okuma", "yazma", "sonuc", "satir-yok"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			current := "eski-parola-12"
			status := http.StatusInternalServerError
			query := mock.ExpectQuery(`SELECT password_hash FROM users WHERE id=\?`).WithArgs(int64(42))
			if scenario == "okuma" {
				query.WillReturnError(errors.New("database unavailable"))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))
			}
			if scenario == "yanlis" {
				current, status = "yanlis-parola", http.StatusUnauthorized
				mock.ExpectExec(`INSERT INTO audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
			}
			if scenario == "yazma" || scenario == "sonuc" || scenario == "satir-yok" {
				update := mock.ExpectExec(passwordUpdate).WithArgs(changedPassword("yeni-parola-34"), int64(42))
				switch scenario {
				case "yazma":
					update.WillReturnError(errors.New("database read only"))
				case "sonuc":
					update.WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected unavailable")))
				case "satir-yok":
					update.WillReturnResult(sqlmock.NewResult(0, 0))
				}
			}
			body := fmt.Sprintf(`{"mevcut":%q,"yeni":"yeni-parola-34","id":1,"role":"admin"}`, current)
			rec := httptest.NewRecorder()
			(&auth.Handlers{DB: db}).ParolaDegistir(rec, securityRequest("parola", body, "user"))
			if rec.Code != status || len(rec.Header().Values("Set-Cookie")) != 0 {
				t.Fatalf("başarısız parola değişimi: status=%d beklenen=%d cookie=%v body=%s", rec.Code, status, rec.Header().Values("Set-Cookie"), rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
