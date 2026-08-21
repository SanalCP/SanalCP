// hotlink.go — resim hotlink koruması (valid_referers). DB satırını yazıp
// provisioner.ApplyVhostForDomain'i tetiklemekten ibaret; asıl render mantığı
// (buildHotlink) provisioner paketinde, her render'da domains.hotlink_* okuyarak
// kendini korur (bkz. redirect.go'daki aynı desen).
package domains

import (
	"encoding/json"
	"net/http"
	"strings"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
)

type HotlinkAyar struct {
	Aktif  bool     `json:"aktif"`
	Izinli []string `json:"izinli"`
}

// GET /domains/{id}/hotlink
func (h *Handlers) HotlinkDurum(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	var aktif int
	var izinli string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(hotlink_aktif,0), COALESCE(hotlink_izinli,'') FROM domains WHERE id=?`, id).
		Scan(&aktif, &izinli)
	out := HotlinkAyar{Aktif: aktif == 1}
	for _, d := range strings.Split(izinli, ",") {
		if d = strings.TrimSpace(d); d != "" {
			out.Izinli = append(out.Izinli, d)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// PUT /domains/{id}/hotlink  {aktif, izinli: string[]}
func (h *Handlers) HotlinkAyarla(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req HotlinkAyar
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	clean := make([]string, 0, len(req.Izinli))
	for _, d := range req.Izinli {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		// Kural adlar paketinde tektir; provisioner tarafı DB'den okurken aynı
		// doğrulamayı ikinci savunma hattı olarak tekrar uygular.
		desen, err := adlar.RefererDeseni(d)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz izinli domain: "+d+" — "+err.Error())
			return
		}
		clean = append(clean, desen)
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET hotlink_aktif=?, hotlink_izinli=? WHERE id=?`,
		req.Aktif, strings.Join(clean, ","), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi: "+err.Error())
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
