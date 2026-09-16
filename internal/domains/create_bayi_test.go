package domains

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

// Geçersiz proxy hedefi, sahiplik doğrulamasından sonra fakat Linux/nginx
// sağlamasından önce durdurur; test gerçek sunucu servislerine dokunmaz.
func TestCreateBayiMusteriSecimi(t *testing.T) {
	for _, tc := range []struct {
		ad           string
		customer     string
		musteriLimit int
		sahip        bool
		status       int
		mesaj        string
	}{
		{"otomatik müşteri", "", 0, false, 400, "proxy protokolü"},
		{"müşteri kotası dolu", "", 1, false, 403, "bayi limiti aşıldı"},
		{"kendi müşterisi", `,"customer_id":9`, 0, true, 400, "proxy protokolü"},
		{"başka bayinin müşterisi", `,"customer_id":9`, 0, false, 403, "bu müşteriye erişim yok"},
	} {
		t.Run(tc.ad, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			middleware.Init(db)
			defer middleware.Init(nil)
			mock.ExpectQuery(`SELECT id FROM service_plans`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(`SELECT id FROM domains`).WithArgs("ornek.com").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			for _, kolon := range []string{"max_domain", "disk_kota_mb", "trafik_kota_mb"} {
				mock.ExpectQuery(`SELECT ` + kolon + ` FROM reseller_limits`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{kolon}).AddRow(0))
			}
			if tc.customer == "" {
				mock.ExpectQuery(`SELECT max_customer FROM reseller_limits`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"max_customer"}).AddRow(tc.musteriLimit))
				if tc.musteriLimit > 0 {
					mock.ExpectQuery(`SELECT COUNT\(\*\) FROM customers WHERE owner_user_id=\?`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				}
			} else {
				n := 0
				if tc.sahip {
					n = 1
				}
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM customers WHERE id = \? AND owner_user_id = \?`).WithArgs(int64(9), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(n))
			}
			body := `{"alan_adi":"ornek.com","php_surum":"8.3","site_tipi":"reverse_proxy","proxy_scheme":"ftp"` + tc.customer + `}`
			r := httptest.NewRequest(http.MethodPost, "/domains", strings.NewReader(body))
			r = middleware.ClaimsIle(r, &auth.Claims{UserID: 7, Role: middleware.RolBayi})
			w := httptest.NewRecorder()
			(&Handlers{DB: db}).Create(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.mesaj) {
				t.Fatalf("yanıt = %d %s; beklenen %d / %s", w.Code, w.Body.String(), tc.status, tc.mesaj)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
