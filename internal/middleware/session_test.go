package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
)

func TestRequireAuthOturumSurumu(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	cases := []struct {
		name     string
		status   string
		dbRole   string
		dbVer    uint64
		dbError  bool
		expected int
	}{
		{"geçerli", "active", RolBayi, 7, false, http.StatusOK},
		{"sürüm değişmiş", "active", RolBayi, 8, false, http.StatusUnauthorized},
		{"askıya alınmış", "suspended", RolBayi, 7, false, http.StatusUnauthorized},
		{"rol değişmiş", "active", RolMusteri, 7, false, http.StatusUnauthorized},
		{"DB hatası fail-closed", "", "", 0, true, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := mockDB(t)
			q := mock.ExpectQuery(regexp.QuoteMeta(
				"SELECT status, role, auth_version FROM users WHERE id=?")).
				WithArgs(int64(42))
			if tc.dbError {
				q.WillReturnError(sqlmock.ErrCancelled)
			} else {
				q.WillReturnRows(sqlmock.NewRows([]string{"status", "role", "auth_version"}).
					AddRow(tc.status, tc.dbRole, tc.dbVer))
			}

			token, err := auth.Issue(secret, 3600, 42, "bayi", RolBayi, 7)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			RequireAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(rec, req)
			if rec.Code != tc.expected {
				t.Fatalf("kod=%d beklenen=%d", rec.Code, tc.expected)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
