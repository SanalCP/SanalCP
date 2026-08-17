package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
)

// requireAuthSorgu: RequireAuth'un tek doğrulama sorgusu. Testler bunu regexp
// olarak eşleştirir; boşluk/satır sonu farkları için (?s) + \s+ kullanılır.
var requireAuthSorgu = regexp.MustCompile(`(?s)SELECT\s+u\.status,\s*u\.role,\s*u\.auth_version,\s*TIMESTAMPDIFF\(SECOND,\s*u\.last_activity_at,\s*NOW\(\)\),\s*p\.oturum_bosta_dakika\s*FROM\s+users u JOIN panel_ayarlari p ON p\.id = 1\s*WHERE u\.id = \?`)

func TestRequireAuthOturumSurumu(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	cases := []struct {
		name       string
		status     string
		dbRole     string
		dbVer      uint64
		bostaSn    any // sql.NullInt64 karşılığı: nil ya da int64
		bostaLimit int
		dbError    bool
		expected   int
	}{
		{"geçerli", "active", RolBayi, 7, nil, 0, false, http.StatusOK},
		{"sürüm değişmiş", "active", RolBayi, 8, nil, 0, false, http.StatusUnauthorized},
		{"askıya alınmış", "suspended", RolBayi, 7, nil, 0, false, http.StatusUnauthorized},
		{"rol değişmiş", "active", RolMusteri, 7, nil, 0, false, http.StatusUnauthorized},
		{"DB hatası fail-closed", "", "", 0, nil, 0, true, http.StatusUnauthorized},
		{"limit kapalı, boşta çok olsa da geçer", "active", RolBayi, 7, int64(999999), 0, false, http.StatusOK},
		{"limit açık, boşta süre aşıldı", "active", RolBayi, 7, int64(1801), 30, false, http.StatusUnauthorized},
		{"limit açık, boşta süre sınırın altında", "active", RolBayi, 7, int64(1799), 30, false, http.StatusOK},
		{"limit açık, hiç aktivite yok (NULL) → geçer", "active", RolBayi, 7, nil, 30, false, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := mockDB(t)
			q := mock.ExpectQuery(requireAuthSorgu.String()).WithArgs(int64(42))
			if tc.dbError {
				q.WillReturnError(sqlmock.ErrCancelled)
			} else {
				q.WillReturnRows(sqlmock.NewRows(
					[]string{"status", "role", "auth_version", "bosta_saniye", "oturum_bosta_dakika"}).
					AddRow(tc.status, tc.dbRole, tc.dbVer, tc.bostaSn, tc.bostaLimit))
				if tc.expected == http.StatusOK {
					mock.ExpectExec(regexp.QuoteMeta(
						"UPDATE users SET last_activity_at = NOW()")).
						WithArgs(int64(42)).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
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
