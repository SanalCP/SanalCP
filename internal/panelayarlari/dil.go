package panelayarlari

// Panelin sunucu-varsayılan dili. Kullanıcı henüz giriş yapmamışken (login ekranı)
// frontend'in hangi dili göstereceğini belirler — bu yüzden PUBLIC (auth'suz) bir
// uçtur. Giriş yapmış kullanıcının kendi tercih_dil'i (users tablosu) bunu her
// zaman geçersiz kılar; bu sadece ilk izlenim / ortak varsayılan içindir.

import (
	"encoding/json"
	"net/http"
	"strings"

	"sanalcp/internal/httpx"
)

// Dil — GET /api/v1/public/dil (auth gerekmez).
func (h *Handlers) Dil(w http.ResponseWriter, r *http.Request) {
	var dil string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT varsayilan_dil FROM panel_ayarlari WHERE id=1`).Scan(&dil)
	if dil != "en" {
		dil = "tr"
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dil": dil})
}

type dilKaydetReq struct {
	Dil string `json:"dil"`
}

// DilKaydet — PUT /api/v1/system/panel-dil (AdminOnly). Kurulumdan sonra da
// panelin varsayılan dilini değiştirebilmek için.
func (h *Handlers) DilKaydet(w http.ResponseWriter, r *http.Request) {
	var req dilKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	dil := strings.ToLower(strings.TrimSpace(req.Dil))
	if dil != "tr" && dil != "en" {
		httpx.WriteError(w, http.StatusBadRequest, "dil 'tr' veya 'en' olmalı")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET varsayilan_dil=? WHERE id=1`, dil); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "dil": dil})
}
