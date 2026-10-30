package plans

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sanalcp/internal/planfeatures"
)

func TestPlanFeatureInputDefaultsAndPreservation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		create   bool
		body     string
		preserve bool
		invalid  bool
	}{
		{"create omitted defaults enabled", true, `{"ad":"legacy-compatible"}`, false, false},
		{"update omitted preserves restrictions", false, `{"ad":"unchanged"}`, true, false},
		{"update null preserves restrictions", false, `{"ad":"unchanged","features":null}`, true, false},
		{"create null rejected", true, `{"ad":"invalid","features":null}`, false, true},
		{"partial rejected on create", true, `{"ad":"invalid","features":{"dns":true}}`, false, true},
		{"partial rejected on update", false, `{"ad":"invalid","features":{"dns":true}}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(tc.body))
			p, stored, err := decodePlan(request, tc.create)
			if (err != nil) != tc.invalid {
				t.Fatalf("decode error=%v", err)
			}
			if tc.invalid {
				return
			}
			if (stored == nil) != tc.preserve {
				t.Fatalf("stored=%v preserve=%v", stored, tc.preserve)
			}
			if !tc.preserve {
				if p.Features != planfeatures.All() {
					t.Fatalf("create defaults=%+v", p.Features)
				}
				var features planfeatures.Features
				if err := json.Unmarshal([]byte(stored.(string)), &features); err != nil || features != planfeatures.All() {
					t.Fatalf("stored defaults=%v error=%v", stored, err)
				}
			}
		})
	}
}

func TestPlanFeatureObjectKeepsExplicitFalse(t *testing.T) {
	body := `{"ad":"restricted","features":{"dns":false,"mail":true,"databases":true,"files":true,"ftp":true,"cron":true,"backups":true,"git":true}}`
	for _, create := range []bool{false, true} {
		p, stored, err := decodePlan(httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body)), create)
		if err != nil {
			t.Fatal(err)
		}
		if p.Features.DNS || !p.Features.Mail || stored == nil {
			t.Fatalf("false grant lost: %+v %v", p.Features, stored)
		}
		var roundtrip planfeatures.Features
		if err := json.Unmarshal([]byte(stored.(string)), &roundtrip); err != nil || roundtrip != p.Features {
			t.Fatalf("stored settings changed: %+v %v", roundtrip, err)
		}
	}
}

func TestPlanCRUDRejectsMalformedFeaturesBeforeDatabaseWork(t *testing.T) {
	h := &Handlers{}
	for _, handler := range []http.HandlerFunc{h.Create, h.Update} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"ad":"restricted","features":{"dns":false}}`)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings accepted: %d %s", rec.Code, rec.Body.String())
		}
	}
}
