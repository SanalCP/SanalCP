package domains

import (
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPlanIslemDurumu(t *testing.T) {
	planIslemYaz("42-7", planIslemDurumu{Durum: "calisiyor", Ilerleme: 75, Adim: "waf"})
	t.Cleanup(func() {
		planIslemleri.Lock()
		delete(planIslemleri.m, "42-7")
		planIslemleri.Unlock()
	})

	r := chi.NewRouter()
	r.Get("/domains/{id}/plan-islemleri/{jid}", (&Handlers{}).PlanIslemDurumu)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/domains/42/plan-islemleri/42-7", nil))
	if w.Code != 200 || w.Body.String() != "{\"durum\":\"calisiyor\",\"ilerleme\":75,\"adim\":\"waf\"}\n" {
		t.Fatalf("durum=%d gövde=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/domains/99/plan-islemleri/42-7", nil))
	if w.Code != 404 {
		t.Fatalf("başka domain yolundan durum=%d, 404 bekleniyordu", w.Code)
	}
}
