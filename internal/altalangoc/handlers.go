package altalangoc

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"sanalcp/internal/httpx"
)

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

// GET /api/v1/altalan-goc — göç envanteri. Yalnız okur.
func (h *Handlers) Envanter(w http.ResponseWriter, r *http.Request) {
	liste, err := Envanter(r.Context(), h.DB)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "envanter: "+err.Error())
		return
	}
	gocEdilebilir := 0
	for _, k := range liste {
		if k.Sorun == "" {
			gocEdilebilir++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"kayitlar":       liste,
		"toplam":         len(liste),
		"goc_edilebilir": gocEdilebilir,
	})
}

type gocReq struct {
	IDs []int64 `json:"ids"`
	// DryRun: gövdede AÇIKÇA false gönderilmedikçe kuru çalıştırma yapılır.
	// Varsayılanın "gerçekten taşı" olması, eksik bir alan yüzünden canlı
	// siteleri hazırlıksız taşırdı; bu yüzden pointer + nil = true.
	DryRun *bool `json:"dry_run"`
}

// POST /api/v1/altalan-goc — seçili alt alan adlarını bağımsız domaine taşır.
// ids boş bırakılırsa göç edilebilir TÜM kayıtlar işlenir.
func (h *Handlers) Calistir(w http.ResponseWriter, r *http.Request) {
	var req gocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	dryRun := req.DryRun == nil || *req.DryRun

	sonuclar, err := Goc(r.Context(), h.DB, h.IPv4, req.IDs, dryRun)
	basarili, basarisiz := 0, 0
	for _, s := range sonuclar {
		if s.Basarili {
			basarili++
		} else {
			basarisiz++
		}
	}
	govde := map[string]any{
		"dry_run":   dryRun,
		"sonuclar":  sonuclar,
		"basarili":  basarili,
		"basarisiz": basarisiz,
	}
	if err != nil {
		// nginx doğrulaması düştüyse kayıtların bir kısmı taşınmış olabilir;
		// sonuç listesi yine döndürülür, aksi halde operatör nerede kalındığını
		// bilemez.
		govde["hata"] = err.Error()
		httpx.WriteJSON(w, http.StatusInternalServerError, govde)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, govde)
}
