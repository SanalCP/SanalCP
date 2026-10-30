package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalcp/internal/auth"
	"sanalcp/internal/planfeatures"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func featureRequest(role string) *http.Request {
	return ClaimsIle(httptest.NewRequest(http.MethodGet, "/domains/7/module", nil), &auth.Claims{UserID: 5, Role: role})
}

func expectFeatureOwnership(mock sqlmock.Sqlmock, role string, owned bool) {
	if role == RolAdmin {
		return
	}
	n := 0
	if owned {
		n = 1
	}
	mock.ExpectQuery("SELECT COUNT").WithArgs(int64(7), int64(5)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
	if role == RolMusteri && owned {
		mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"askida"}).AddRow(0))
	}
}

func expectFeaturePlan(mock sqlmock.Sqlmock, stored any) {
	mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"ana_domain_id", "plan_id", "id", "features"}).AddRow(nil, 2, 2, stored))
}

func TestFeatureScopeCustomerChangesOnNextRequest(t *testing.T) {
	for _, module := range []planfeatures.Module{planfeatures.DNS, planfeatures.Mail, planfeatures.Databases, planfeatures.Files, planfeatures.FTP, planfeatures.Cron, planfeatures.Backups, planfeatures.Git} {
		t.Run(string(module), func(t *testing.T) {
			mock := mockDB(t)
			router := chi.NewRouter()
			router.With(FeatureScope(module)).Get("/domains/{id}/module", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			for _, allowed := range []bool{false, true} {
				expectFeatureOwnership(mock, RolMusteri, true)
				features := planfeatures.Features{}
				if allowed {
					features = planfeatures.All()
				}
				stored, _ := json.Marshal(features)
				expectFeaturePlan(mock, string(stored))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, featureRequest(RolMusteri))
				if allowed {
					if rec.Code != http.StatusNoContent {
						t.Fatalf("reenabled module blocked: %d %s", rec.Code, rec.Body.String())
					}
				} else {
					var body struct {
						Code    string `json:"code"`
						Feature string `json:"feature"`
						Hata    string `json:"hata"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if rec.Code != http.StatusForbidden || body.Code != "feature_disabled" || body.Feature != string(module) || body.Hata == "" {
						t.Fatalf("disabled module response: %d %s", rec.Code, rec.Body.String())
					}
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFeatureScopeOwnershipPrecedesGrantsAndManagersRemainScoped(t *testing.T) {
	for _, tc := range []struct {
		role   string
		owned  bool
		status int
	}{
		{RolAdmin, true, 204}, {RolBayi, true, 204}, {RolBayi, false, 403}, {RolMusteri, false, 403},
	} {
		t.Run(tc.role+http.StatusText(tc.status), func(t *testing.T) {
			mock := mockDB(t)
			expectFeatureOwnership(mock, tc.role, tc.owned)
			router := chi.NewRouter()
			router.With(FeatureScope(planfeatures.Files)).Get("/domains/{id}/module", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, featureRequest(tc.role))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDomainFeaturesReportsActualFlagsAndEnforcement(t *testing.T) {
	for _, role := range []string{RolAdmin, RolBayi, RolMusteri} {
		t.Run(role, func(t *testing.T) {
			mock := mockDB(t)
			expectFeatureOwnership(mock, role, true)
			stored, _ := json.Marshal(planfeatures.Features{})
			expectFeaturePlan(mock, string(stored))
			router := chi.NewRouter()
			router.With(MusteriScope).Get("/domains/{id}/module", DomainFeatures)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, featureRequest(role))
			var body struct {
				Features planfeatures.Features `json:"features"`
				Enforced bool                  `json:"enforced"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || body.Features != (planfeatures.Features{}) || body.Enforced != (role == RolMusteri) {
				t.Fatalf("incorrect feature view: %d %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFeatureScopeCorruptStoredSettingsFailClosed(t *testing.T) {
	mock := mockDB(t)
	expectFeatureOwnership(mock, RolMusteri, true)
	expectFeaturePlan(mock, `{"files":true}`)
	router := chi.NewRouter()
	router.With(FeatureScope(planfeatures.Files)).Get("/domains/{id}/module", func(w http.ResponseWriter, _ *http.Request) { t.Fatal("corrupt settings reached handler") })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, featureRequest(RolMusteri))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
