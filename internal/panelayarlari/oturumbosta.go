package panelayarlari

// Oturum boşta zaman aşımı: bir hesap N dakika hiç istek atmazsa, token süresi
// dolmamış olsa bile bir sonraki istekte oturumu geçersiz kılar (bkz.
// internal/middleware/auth.go RequireAuth). 0 = kapalı (varsayılan).

import (
	"encoding/json"
	"net/http"

	"sanalcp/internal/httpx"
)

// OturumBosta — GET /api/v1/system/oturum-bosta (AdminOnly).
func (h *Handlers) OturumBosta(w http.ResponseWriter, r *http.Request) {
	var dakika int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT oturum_bosta_dakika FROM panel_ayarlari WHERE id=1`).Scan(&dakika)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dakika": dakika})
}

type oturumBostaKaydetReq struct {
	Dakika int `json:"dakika"`
}

// OturumBostaKaydet — PUT /api/v1/system/oturum-bosta (AdminOnly).
// 0 = kapalı. Üst sınır 1440 (24 saat) — panel_ayarlari.oturum_bosta_dakika
// SMALLINT UNSIGNED olduğu için negatif zaten JSON decode'da elenir.
func (h *Handlers) OturumBostaKaydet(w http.ResponseWriter, r *http.Request) {
	var req oturumBostaKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Dakika < 0 || req.Dakika > 1440 {
		httpx.WriteError(w, http.StatusBadRequest, "dakika 0-1440 arası olmalı (0=kapalı)")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET oturum_bosta_dakika=? WHERE id=1`, req.Dakika); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "dakika": req.Dakika})
}
