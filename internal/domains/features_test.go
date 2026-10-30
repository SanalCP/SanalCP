package domains

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
	"sanalcp/internal/planfeatures"
	"sanalcp/internal/pma"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func TestDatabaseIDRoutesCannotBypassDisabledModule(t *testing.T) {
	for _, route := range []string{"delete", "password", "pma-token"} {
		for _, owned := range []bool{true, false} {
			t.Run(route+map[bool]string{true: " owned", false: " unowned"}[owned], func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				middleware.Init(db)
				defer middleware.Init(nil)
				h := &Handlers{DB: db}
				router := chi.NewRouter()
				method, path := http.MethodDelete, "/databases/11"
				switch route {
				case "delete":
					mock.ExpectQuery("SELECT db.db_name, db.domain_id").WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"db_name", "domain_id", "is_demo"}).AddRow("site_db", 7, 0))
					router.Delete("/databases/{dbid}", h.DeleteDatabase)
				case "password":
					method, path = http.MethodPut, "/databases/11/password"
					mock.ExpectQuery("SELECT db.db_name, db.db_user").WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"db_name", "db_user", "domain_id", "is_demo"}).AddRow("site_db", "site_user", 7, 0))
					router.Put("/databases/{dbid}/password", h.SetDatabasePassword)
				case "pma-token":
					method, path = http.MethodPost, "/databases/11/pma-token"
					mock.ExpectQuery("SELECT db.db_user, db.db_pass_plain").WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"db_user", "db_pass_plain", "db_name", "domain_id", "is_demo"}).AddRow("site_user", "encrypted", "site_db", 7, 0))
					router.Post("/databases/{dbId}/pma-token", (&pma.Handlers{DB: db}).TokenIste)
				}
				n := 0
				if owned {
					n = 1
				}
				mock.ExpectQuery("SELECT COUNT").WithArgs(int64(7), int64(5)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
				if owned {
					features := planfeatures.All()
					features.Databases = false
					stored, _ := json.Marshal(features)
					mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"ana_domain_id", "plan_id", "id", "features"}).AddRow(nil, 2, 2, string(stored)))
				}
				request := middleware.ClaimsIle(httptest.NewRequest(method, path, strings.NewReader("not-json")), &auth.Claims{UserID: 5, Role: middleware.RolMusteri})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, request)
				if owned {
					var body struct {
						Code string `json:"code"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if rec.Code != http.StatusForbidden || body.Code != "feature_disabled" {
						t.Fatalf("disabled module bypass: %d %s", rec.Code, rec.Body.String())
					}
				} else if rec.Code != http.StatusNotFound {
					t.Fatalf("ownership checked after feature: %d %s", rec.Code, rec.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
