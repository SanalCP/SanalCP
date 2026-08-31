// Plan atama + kaynak limit yeniden uygulama.
package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/kaynaklimit"
	"sanalcp/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// PUT /domains/{id}/plan  body: {"plan_id": 3}  (null → planı kaldır)
type setPlanReq struct {
	PlanID *int64 `json:"plan_id"`
}

type planIslemDurumu struct {
	Durum    string `json:"durum"`
	Ilerleme int    `json:"ilerleme"`
	Adim     string `json:"adim"`
	Hata     string `json:"hata,omitempty"`
}

var planIslemleri = struct {
	sync.RWMutex
	m map[string]planIslemDurumu
}{m: make(map[string]planIslemDurumu)}

var planIslemSirasi atomic.Uint64

func planIslemYaz(id string, durum planIslemDurumu) {
	planIslemleri.Lock()
	planIslemleri.m[id] = durum
	planIslemleri.Unlock()
}

func planIslemOku(id string) (planIslemDurumu, bool) {
	planIslemleri.RLock()
	durum, ok := planIslemleri.m[id]
	planIslemleri.RUnlock()
	return durum, ok
}

func (h *Handlers) SetPlan(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setPlanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	// Plan var mı doğrula
	if req.PlanID != nil {
		var n int
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM service_plans WHERE id=?`, *req.PlanID).Scan(&n); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "plan bulunamadı")
			return
		}
	}
	// Domain var mı
	var sk string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici FROM domains WHERE id=?`, id).Scan(&sk); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		} else {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// Güncelle
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET plan_id=? WHERE id=?`, req.PlanID, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB: "+err.Error())
		return
	}
	islemID := strconv.FormatInt(id, 10) + "-" + strconv.FormatUint(planIslemSirasi.Add(1), 10)
	planIslemYaz(islemID, planIslemDurumu{Durum: "calisiyor", Ilerleme: 15, Adim: "plan_kaydedildi"})

	// Kaynak limitlerini yeniden uygula — arkaplanda + kendi context'i
	// (r.Context() HTTP request bitince iptal olur, cgroup yazımı yarıda kalır)
	go func(did int64, jid string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		planIslemYaz(jid, planIslemDurumu{Durum: "calisiyor", Ilerleme: 25, Adim: "kaynak_limitleri"})
		hatalar := make([]string, 0, 2)
		if err := kaynaklimit.UygulaHepsi(ctx, h.DB, did); err != nil {
			log.Printf("kaynaklimit apply domain=%d: %v", did, err)
			hatalar = append(hatalar, "kaynak limitleri: "+err.Error())
		}
		planIslemYaz(jid, planIslemDurumu{Durum: "calisiyor", Ilerleme: 75, Adim: "waf"})
		// Plan degisti → WAF plan varsayilani da degismis olabilir; vhost'u WAF ile yeniden
		// render et (domain override yoksa yeni planin WAF varsayilanini devralir).
		if err := provisioner.WAFUygula(h.DB, did); err != nil {
			log.Printf("waf apply (plan degisimi) domain=%d: %v", did, err)
			hatalar = append(hatalar, "WAF: "+err.Error())
		}
		if len(hatalar) > 0 {
			planIslemYaz(jid, planIslemDurumu{Durum: "basarisiz", Ilerleme: 100, Adim: "tamamlandi", Hata: strings.Join(hatalar, "; ")})
		} else {
			planIslemYaz(jid, planIslemDurumu{Durum: "basarili", Ilerleme: 100, Adim: "tamamlandi"})
		}
		time.AfterFunc(10*time.Minute, func() {
			planIslemleri.Lock()
			delete(planIslemleri.m, jid)
			planIslemleri.Unlock()
		})
	}(id, islemID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "plan_id": req.PlanID, "islem_id": islemID})
}

// PlanIslemDurumu, SetPlan'in başlattığı kaynak limiti + WAF uygulamasını izler.
func (h *Handlers) PlanIslemDurumu(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "id")
	islemID := chi.URLParam(r, "jid")
	// İş kimliği domain ID'siyle başlar; başka bir domain yolundan sorgulanamaz.
	if !strings.HasPrefix(islemID, domainID+"-") {
		httpx.WriteError(w, http.StatusNotFound, "işlem bulunamadı")
		return
	}
	durum, ok := planIslemOku(islemID)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "işlem bulunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, durum)
}
