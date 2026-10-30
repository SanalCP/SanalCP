package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"sanalcp/internal/planfeatures"
	"sanalcp/internal/plans"

	"github.com/go-chi/chi/v5"
)

// Uses the existing isolated MariaDB migration fixture, never a live panel DB.
func TestPlanFeatureCRUDPersistsAndOmittedUpdatesPreserveRestrictions(t *testing.T) {
	db := migrationTestDB(t)
	depoMigrationDizini(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	h := &plans.Handlers{DB: db}
	router := chi.NewRouter()
	router.Post("/plans", h.Create)
	router.Put("/plans/{id}", h.Update)
	router.Get("/plans/{id}", h.Get)

	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	create := func(body string) plans.Plan {
		t.Helper()
		rec := call(http.MethodPost, "/plans", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
		}
		var p plans.Plan
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	legacy := create(`{"ad":"features-default"}`)
	if legacy.Features != planfeatures.All() {
		t.Fatalf("omitted create features=%+v", legacy.Features)
	}
	restricted := planfeatures.All()
	restricted.Files = false
	stored, _ := json.Marshal(restricted)
	created := create(`{"ad":"features-restricted","features":` + string(stored) + `}`)
	path := "/plans/" + strconv.FormatInt(created.ID, 10)
	if created.Features != restricted {
		t.Fatalf("create lost restrictions: %+v", created.Features)
	}
	for _, body := range []string{`{"ad":"features-renamed"}`, `{"ad":"features-renamed","features":null}`} {
		rec := call(http.MethodPut, path, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
		}
		var updated plans.Plan
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatal(err)
		}
		if updated.Features != restricted {
			t.Fatalf("omitted/null update restored access: %+v", updated.Features)
		}
	}
	rec := call(http.MethodPut, path, `{"ad":"features-renamed","features":{"files":true}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("partial object accepted: %d %s", rec.Code, rec.Body.String())
	}
	read := call(http.MethodGet, path, "")
	var result struct {
		Plan plans.Plan `json:"plan"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || result.Plan.Features != restricted {
		t.Fatalf("stored restrictions changed: %d %s", read.Code, read.Body.String())
	}
	stored, _ = json.Marshal(planfeatures.All())
	rec = call(http.MethodPut, path, `{"ad":"features-enabled","features":`+string(stored)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", rec.Code, rec.Body.String())
	}
	read = call(http.MethodGet, path, "")
	if err := json.Unmarshal(read.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || result.Plan.Features != planfeatures.All() {
		t.Fatalf("enable did not persist: %d %s", read.Code, read.Body.String())
	}
}
