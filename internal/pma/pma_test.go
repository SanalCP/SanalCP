package pma

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"
	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

// Gerçek TIMESTAMP dönüşümünü sınar. DSN, CREATE DATABASE yetkili bir test
// sunucusunu göstermeli; mevcut tablolar yerine ayrı, geçici bir şema kullanılır.
// /etc/sanalcp/pma-internal.token test ortamında önceden hazırlanmalıdır.
func TestTokenSaatDilimi(t *testing.T) {
	dsn := os.Getenv("SANALCP_PMA_TEST_DSN")
	if dsn == "" {
		t.Skip("SANALCP_PMA_TEST_DSN gerekli (MariaDB entegrasyonu)")
	}
	internalToken := internalAuthToken()
	if internalToken == "" {
		t.Fatal("test ortamının internal token dosyası eksik")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("test DSN geçersiz")
	}
	cfg.DBName = ""
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("test DB açılamadı")
	}
	defer root.Close()
	schema := fmt.Sprintf("zz_pma_test_%d", time.Now().UnixNano())
	if _, err := root.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	defer root.Exec("DROP DATABASE `" + schema + "`")
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0010_pma.sql"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = schema
	cfg.ParseTime = true
	cfg.Loc = time.UTC // Üretimdeki varsayılan Go sürücü konumu.
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	for _, zone := range []string{"+00:00", "+03:00"} {
		t.Run(zone, func(t *testing.T) {
			cfg.Params["time_zone"] = "'" + zone + "'"
			db, err := sql.Open("mysql", cfg.FormatDSN())
			if err != nil {
				t.Fatal("test DB açılamadı")
			}
			defer db.Close()
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := db.Exec(query, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec("CREATE TABLE IF NOT EXISTS domains (id BIGINT UNSIGNED PRIMARY KEY, is_demo INT NOT NULL)")
			exec("CREATE TABLE IF NOT EXISTS db_accounts (id BIGINT PRIMARY KEY, domain_id BIGINT UNSIGNED, db_user VARCHAR(80), db_pass_plain VARCHAR(255), db_name VARCHAR(80))")
			exec(string(migration))
			exec("DELETE FROM pma_tokens")
			exec("REPLACE INTO domains VALUES(1,0)")
			exec("REPLACE INTO db_accounts VALUES(1,1,'pma_test_user','pma_test_password','pma_test_db')")
			h := &Handlers{DB: db}
			router := chi.NewRouter()
			router.Post("/databases/{dbId}/pma-token", h.TokenIste)
			issue := func(role string) *httptest.ResponseRecorder {
				t.Helper()
				r := httptest.NewRequest(http.MethodPost, "/databases/1/pma-token", nil)
				if role != "" {
					r = r.WithContext(auth.ClaimsContext(r.Context(), &auth.Claims{Role: role}))
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w
			}
			create := func() string {
				t.Helper()
				w := issue(middleware.RolAdmin)
				if w.Code != http.StatusCreated {
					t.Fatalf("oluşturma HTTP %d", w.Code)
				}
				var result struct {
					Token   string    `json:"token"`
					TTL     int       `json:"gecerlilik_sn"`
					Expires time.Time `json:"son_kullanma"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Token == "" || result.TTL != 120 || time.Until(result.Expires) < 115*time.Second {
					t.Fatal("oluşturma yanıtı geçersiz")
				}
				var seconds int
				if err := db.QueryRow("SELECT TIMESTAMPDIFF(SECOND,NOW(),son_kullanma) FROM pma_tokens WHERE token=?", result.Token).Scan(&seconds); err != nil {
					t.Fatal("geçerli token temizlikte silindi")
				}
				if seconds < 115 || seconds > 120 {
					t.Fatalf("geçerlilik %d saniye", seconds)
				}
				return result.Token
			}
			redeem := func(token, authHeader string) int {
				r := httptest.NewRequest(http.MethodPost, "/api/v1/internal/pma-redeem", strings.NewReader(`{"token":"`+token+`"}`))
				r.Header.Set("X-Internal-Auth", authHeader)
				w := httptest.NewRecorder()
				h.Bozdur(w, r)
				if w.Code == http.StatusOK {
					var result map[string]string
					if json.Unmarshal(w.Body.Bytes(), &result) != nil || result["kullanici"] != "pma_test_user" || result["parola"] != "pma_test_password" || result["db"] != "pma_test_db" || result["host"] != "localhost" {
						return 0
					}
				}
				return w.Code
			}
			assert := func(ok bool, message string) {
				t.Helper()
				if !ok {
					t.Fatal(message)
				}
			}
			assert(issue("").Code == http.StatusNotFound, "yetkisiz oluşturma reddedilmedi")
			exec("UPDATE domains SET is_demo=1")
			assert(issue(middleware.RolAdmin).Code == http.StatusForbidden, "demo oluşturma reddedilmedi")
			exec("UPDATE domains SET is_demo=0")
			token := create()
			assert(redeem(token, "") == http.StatusUnauthorized, "yetkisiz tüketim reddedilmedi")
			assert(redeem(token, internalToken) == http.StatusOK, "geçerli token tüketilemedi")
			replay := redeem(token, internalToken)
			assert(replay == http.StatusNotFound || replay == http.StatusGone, "tekrar kullanım reddedilmedi")
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM pma_tokens WHERE token=? AND db_parola<>''", token).Scan(&count)
			assert(err == nil && count == 0, "tüketilen parola temizlenmedi")
			token = create()
			exec("UPDATE pma_tokens SET son_kullanma=NOW()-INTERVAL 1 SECOND WHERE token=?", token)
			assert(redeem(token, internalToken) == http.StatusGone, "süresi dolan token reddedilmedi")
			used := create() // Eski token oluşturma temizliğinde silinmeli.
			exec("UPDATE pma_tokens SET kullanildi=1,db_parola='' WHERE token=?", used)
			assert(redeem(used, internalToken) == http.StatusGone, "kullanılmış token reddedilmedi")
			live := create()
			err = db.QueryRow("SELECT COUNT(*) FROM pma_tokens WHERE token IN (?,?)", token, used).Scan(&count)
			assert(err == nil && count == 0, "eski/kullanılmış token temizlenmedi")
			// Eşzamanlı tüketimlerde yalnız bir istek kimlik bilgilerini alabilir.
			codes := make(chan int, 8)
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); codes <- redeem(live, internalToken) }()
			}
			wg.Wait()
			close(codes)
			successes := 0
			for code := range codes {
				if code == http.StatusOK {
					successes++
				} else {
					assert(code == http.StatusGone || code == http.StatusNotFound, "eşzamanlı tüketimde beklenmeyen yanıt")
				}
			}
			assert(successes == 1, "token tam bir kez tüketilmedi")
		})
	}
}
