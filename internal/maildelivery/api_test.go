package maildelivery

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

var apiDeliveryColumns = []string{"id", "queue_id", "message_id", "sender", "recipient", "original_recipient", "status", "first_seen_at", "last_seen_at", "relay", "dsn", "response"}

func apiMock(t *testing.T) (*Handlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	return &Handlers{DB: db}, mock
}

func apiRequest(path, role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		r = middleware.ClaimsIle(r, &auth.Claims{UserID: 5, Role: role})
	}
	return r
}

func apiRouter(h *Handlers) http.Handler {
	r := chi.NewRouter()
	r.With(middleware.BayiVeUstu).Get("/mail/deliveries", h.List)
	r.With(middleware.BayiVeUstu).Get("/mail/deliveries/{deliveryID}", h.Detail)
	r.With(middleware.FeatureScope("mail")).Get("/domains/{id}/mail/deliveries", h.DomainList)
	r.With(middleware.FeatureScope("mail")).Get("/domains/{id}/mail/deliveries/{deliveryID}", h.DomainDetail)
	return r
}

func apiServe(h *Handlers, path, role string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	apiRouter(h).ServeHTTP(rec, apiRequest(path, role))
	return rec
}

func apiCollectorRows(state, warning string, collected any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"state", "warning", "collected_at"}).AddRow(state, warning, collected)
}

func TestDeliveryGlobalCustomerAndUnknownRoleDenied(t *testing.T) {
	for _, role := range []string{"", middleware.RolMusteri, "other"} {
		for _, path := range []string{"/mail/deliveries", "/mail/deliveries/1"} {
			rec := apiServe(&Handlers{}, path, role)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("role=%q path=%s status=%d", role, path, rec.Code)
			}
		}
	}
}

func TestDeliveryInvalidFiltersRejectedBeforeDatabase(t *testing.T) {
	for _, query := range []string{
		"status=accepted", "status=sent", "page=0", "page=-1", "page=1000001", "page=", "page=1.5", "page=1&page=2",
		"page_size=0", "page_size=101", "page_size=", "from=2026-02-30", "from=0999-12-31", "to=2026-10-29T00:00:00%2B03:00",
		"from=2026-10-30&to=2026-10-29", "from=2026-10-29T23:59:59Z&to=2026-10-29T00:00:00Z",
		"sender=" + strings.Repeat("x", 321), "recipient=" + strings.Repeat("x", 321), "queue_id=" + strings.Repeat("x", 65),
		"sender=%00", "recipient=%FF",
	} {
		t.Run(query, func(t *testing.T) {
			rec := apiServe(&Handlers{}, "/mail/deliveries?"+query, middleware.RolAdmin)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeliveryFiltersLiteralWildcardsAndInclusiveDates(t *testing.T) {
	q := url.Values{
		"sender": {`A%_!\B@example.test`}, "recipient": {`user_100%@example.test`}, "queue_id": {"MixedCaseID"}, "status": {"deferred"},
		"from": {"2026-10-01"}, "to": {"2026-10-29"}, "page": {"2"}, "page_size": {"100"},
	}
	f, err := parseDeliveryFilters(q)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{`%A!%!_!!\B@example.test%`, `%user!_100!%@example.test%`, "MixedCaseID", "deferred", "2026-10-01 00:00:00.000000", "2026-10-29 23:59:59.999999"}
	if !reflect.DeepEqual(f.args, want) || f.page != 2 || f.pageSize != 100 {
		t.Fatalf("filters=%+v want args=%v", f, want)
	}
	f, err = parseDeliveryFilters(url.Values{"from": {"2026-10-29T00:00:00+00:00"}, "to": {"2026-10-29T12:34:56.123456Z"}})
	if err != nil || !reflect.DeepEqual(f.args, []any{"2026-10-29 00:00:00.000000", "2026-10-29 12:34:56.123456"}) {
		t.Fatalf("UTC timestamp bounds=%+v err=%v", f, err)
	}
	f, err = parseDeliveryFilters(url.Values{"from": {"2026-10-29T00:00:00.123456789Z"}, "to": {"2026-10-29T00:00:00.123456999Z"}})
	if err != nil || !reflect.DeepEqual(f.args, []any{"2026-10-29 00:00:00.123457", "2026-10-29 00:00:00.123456"}) {
		t.Fatalf("sub-microsecond bounds=%+v err=%v", f, err)
	}
}

func TestDeliveryListFiltersApplyToTotalAndPage(t *testing.T) {
	h, mock := apiMock(t)
	args := []driver.Value{int64(5), "%person!_!%@example.test%", "%target!_%", "QueueID", "delivered", "2026-10-01 00:00:00.000000", "2026-10-29 23:59:59.999999"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(37))
	pageArgs := append(append([]driver.Value(nil), args...), 25, 25)
	mock.ExpectQuery("SELECT").WithArgs(pageArgs...).WillReturnRows(sqlmock.NewRows(apiDeliveryColumns).
		AddRow(8, "QueueID", "<message@example.test>", "person_%@example.test", "target_x@example.test", "", "delivered", "2026-10-01T00:00:00.000000Z", "2026-10-29T01:00:00.000000Z", "mx.example.test", "2.0.0", "250 accepted"))
	mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows("ok", "", "2026-10-29T01:01:00.000000Z"))
	mock.ExpectCommit()
	q := url.Values{"sender": {"person_%@example.test"}, "recipient": {"target_"}, "queue_id": {"QueueID"}, "status": {"delivered"}, "from": {"2026-10-01"}, "to": {"2026-10-29"}, "page": {"2"}}
	rec := apiServe(h, "/mail/deliveries?"+q.Encode(), middleware.RolBayi)
	var result deliveryListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || result.Total != 37 || result.Page != 2 || result.PageSize != 25 || len(result.Items) != 1 || result.Items[0].ID != 8 || result.Items[0].Status != "delivered" || result.Collector.State != "ok" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeliveryEmptyHistoryDoesNotHideCollectorState(t *testing.T) {
	for _, state := range []string{"missing", "starting", "error", "ok"} {
		t.Run(state, func(t *testing.T) {
			h, mock := apiMock(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(apiDeliveryColumns))
			warning := ""
			if state == "missing" {
				mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)
			} else {
				if state == "error" {
					warning = "source_unavailable"
				}
				mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows(state, warning, nil))
			}
			mock.ExpectCommit()
			rec := apiServe(h, "/mail/deliveries", middleware.RolAdmin)
			var result deliveryListResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantState := state
			if state == "missing" {
				wantState = "starting"
			}
			if rec.Code != 200 || result.Items == nil || len(result.Items) != 0 || result.Total != 0 || result.Collector.State != wantState || result.Collector.Warning != warning || result.Collector.LastCollectedAt != nil {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeliveryListDatabaseFailuresNeverReturnEmptySuccess(t *testing.T) {
	for _, stage := range []string{"begin", "count", "list", "row", "collector", "commit"} {
		t.Run(stage, func(t *testing.T) {
			h, mock := apiMock(t)
			failure := errors.New("private /var/log/path database error")
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
			} else {
				mock.ExpectBegin()
				if stage == "count" {
					mock.ExpectQuery("SELECT").WillReturnError(failure)
				} else {
					mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
					if stage == "list" {
						mock.ExpectQuery("SELECT").WillReturnError(failure)
					} else {
						rows := sqlmock.NewRows(apiDeliveryColumns)
						if stage == "row" {
							rows.AddRow(1, "Q", "", "", "target@example.test", "", "pending", "date", "date", "", "", "").RowError(0, failure)
						}
						mock.ExpectQuery("SELECT").WillReturnRows(rows)
						if stage != "row" {
							if stage == "collector" {
								mock.ExpectQuery("SELECT").WillReturnError(failure)
							} else {
								mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows("ok", "", nil))
								mock.ExpectCommit().WillReturnError(failure)
							}
						}
					}
				}
				if stage != "commit" {
					mock.ExpectRollback()
				}
			}
			rec := apiServe(h, "/mail/deliveries", middleware.RolAdmin)
			if rec.Code < 500 || strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "items") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func apiDetailRow() *sqlmock.Rows {
	columns := append(append([]string(nil), apiDeliveryColumns...), "message_fk")
	return sqlmock.NewRows(columns).AddRow(8, "QueueID", "<message@example.test>", "sender@example.test", "recipient@example.test", "", "delivered", "2026-10-01T00:00:00.000000Z", "2026-10-29T01:00:00.000000Z", "mx.example.test", "2.0.0", "250 accepted", 3)
}

func TestDeliveryDetailTimelineChronologicalAndExplicitlyTruncated(t *testing.T) {
	h, mock := apiMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT").WithArgs(uint64(8)).WillReturnRows(apiDetailRow())
	events := sqlmock.NewRows([]string{"id", "at", "status", "relay", "dsn", "response", "delay_seconds"})
	for id := deliveryEventLimit + 1; id >= 1; id-- {
		events.AddRow(id, "2026-10-29T01:00:00.000000Z", "deferred", "mx.example.test", "4.0.0", "temporary failure", 0.5)
	}
	mock.ExpectQuery("SELECT").WithArgs(uint64(3), uint64(8), deliveryEventLimit+1).WillReturnRows(events)
	mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows("ok", "history_gap", nil))
	mock.ExpectCommit()
	rec := apiServe(h, "/mail/deliveries/8", middleware.RolAdmin)
	var result deliveryDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || result.Delivery.ID != 8 || !result.EventsTruncated || len(result.Events) != deliveryEventLimit || result.Events[0].ID != 2 || result.Events[len(result.Events)-1].ID != deliveryEventLimit+1 || result.Events[0].DelaySeconds == nil || *result.Events[0].DelaySeconds != 0.5 || result.Collector.Warning != "history_gap" {
		t.Fatalf("status=%d events=%d truncated=%v", rec.Code, len(result.Events), result.EventsTruncated)
	}
}

func TestDeliveryForeignAndUnknownItemsBothNotFound(t *testing.T) {
	for _, domain := range []bool{false, true} {
		t.Run(map[bool]string{false: "global", true: "domain"}[domain], func(t *testing.T) {
			h, mock := apiMock(t)
			mock.ExpectBegin()
			expected := mock.ExpectQuery("SELECT")
			path := "/mail/deliveries/8"
			if domain {
				path = "/domains/7/mail/deliveries/8"
				expected.WithArgs(uint64(7), uint64(8))
			} else {
				expected.WithArgs(uint64(8))
			}
			expected.WillReturnError(sql.ErrNoRows)
			mock.ExpectRollback()
			rec := apiServe(h, path, middleware.RolAdmin)
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "events") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeliveryDomainCustomerMailPlanOffBlocksListAndDetail(t *testing.T) {
	for _, path := range []string{"/domains/7/mail/deliveries", "/domains/7/mail/deliveries/8"} {
		h, mock := apiMock(t)
		middleware.Init(h.DB)
		t.Cleanup(func() { middleware.Init(nil) })
		mock.ExpectQuery("SELECT").WithArgs(int64(7), int64(5)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		mock.ExpectQuery("SELECT").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"askida"}).AddRow(0))
		mock.ExpectQuery("SELECT").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"ana_domain_id", "plan_id", "id", "features"}).AddRow(nil, 2, 2, `{"dns":false,"mail":false,"databases":false,"files":false,"ftp":false,"cron":false,"backups":false,"git":false}`))
		rec := apiServe(h, path, middleware.RolMusteri)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "feature_disabled") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestDeliveryDetailFailuresDoNotReturnPartialTimeline(t *testing.T) {
	for _, stage := range []string{"item", "events", "row", "collector", "commit"} {
		t.Run(stage, func(t *testing.T) {
			h, mock := apiMock(t)
			failure := errors.New("private collector error /var/log/journal")
			mock.ExpectBegin()
			if stage == "item" {
				mock.ExpectQuery("SELECT").WillReturnError(failure)
			} else {
				mock.ExpectQuery("SELECT").WillReturnRows(apiDetailRow())
				if stage == "events" {
					mock.ExpectQuery("SELECT").WillReturnError(failure)
				} else {
					rows := sqlmock.NewRows([]string{"id", "at", "status", "relay", "dsn", "response", "delay_seconds"})
					if stage == "row" {
						rows.AddRow(1, "2026-10-29T00:00:00Z", "queued", "", "", "", nil).RowError(0, failure)
					}
					mock.ExpectQuery("SELECT").WillReturnRows(rows)
					if stage != "row" {
						if stage == "collector" {
							mock.ExpectQuery("SELECT").WillReturnError(failure)
						} else {
							mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows("ok", "", nil))
							mock.ExpectCommit().WillReturnError(failure)
						}
					}
				}
			}
			if stage != "commit" {
				mock.ExpectRollback()
			}
			rec := apiServe(h, "/mail/deliveries/8", middleware.RolAdmin)
			if rec.Code < 500 || strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "events") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeliveryCollectorDoesNotExposeUnexpectedWarningText(t *testing.T) {
	h, mock := apiMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(apiDeliveryColumns))
	mock.ExpectQuery("SELECT").WillReturnRows(apiCollectorRows("internal-state", "private /var/log/journal failure", nil))
	mock.ExpectCommit()
	rec := apiServe(h, "/mail/deliveries", middleware.RolAdmin)
	var result deliveryListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || result.Collector.State != "error" || result.Collector.Warning != "source_unavailable" || strings.Contains(rec.Body.String(), "private") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
