package panelayarlari

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
)

const (
	auditPanelErisimAc    = "panel.erisim_kisiti.ac"
	auditPanelErisimKapat = "panel.erisim_kisiti.kapat"
	auditPanelGeciciAc    = "panel.erisim_gecici.ac"
	auditPanelGeciciKapat = "panel.erisim_gecici.kapat"
)

// ErisimKisiti — GET /api/v1/system/panel-erisim (AdminOnly).
func (h *Handlers) ErisimKisiti(w http.ResponseWriter, r *http.Request) {
	var ham, geciciCIDR, geciciBitis string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(erisim_cidrleri,''), COALESCE(gecici_erisim_cidr,''),
		        COALESCE(DATE_FORMAT(gecici_erisim_bitis,'%Y-%m-%d %H:%i:%s'),'')
		   FROM panel_ayarlari WHERE id=1`).Scan(&ham, &geciciCIDR, &geciciBitis); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "panel erişim ayarı okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"cidrler": satirlar(ham), "istemci_ip": httpx.ClientIP(r),
		"gecici_cidr": geciciCIDR, "gecici_bitis": geciciBitis,
	})
}

type geciciErisimReq struct {
	CIDR   string `json:"cidr"`
	Dakika int    `json:"dakika"`
}

// GeciciErisimKaydet tek bir IP/ağı sınırlı süreyle kalıcı listenin yanına ekler.
// Dakika=0 geçici erişimi erken iptal eder. Süre DB saatine göre değerlendirilir.
func (h *Handlers) GeciciErisimKaydet(w http.ResponseWriter, r *http.Request) {
	var req geciciErisimReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Dakika == 0 {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE panel_ayarlari SET gecici_erisim_cidr=NULL, gecici_erisim_bitis=NULL WHERE id=1`); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
			return
		}
		h.erisimAudit(r, auditPanelGeciciKapat)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "gecici_cidr": "", "gecici_bitis": ""})
		return
	}
	if req.Dakika < 5 || req.Dakika > 1440 {
		httpx.WriteError(w, http.StatusBadRequest, "geçici erişim süresi 5-1440 dakika olmalı")
		return
	}
	norm, _, err := normalizeCIDRler([]string{req.CIDR})
	if err != nil || len(norm) != 1 {
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		} else {
			httpx.WriteError(w, http.StatusBadRequest, "tek bir IP/CIDR gerekli")
		}
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET gecici_erisim_cidr=?, gecici_erisim_bitis=DATE_ADD(NOW(), INTERVAL ? MINUTE) WHERE id=1`, norm[0], req.Dakika); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	var bitis string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(DATE_FORMAT(gecici_erisim_bitis,'%Y-%m-%d %H:%i:%s'),'') FROM panel_ayarlari WHERE id=1`).Scan(&bitis)
	h.erisimAudit(r, auditPanelGeciciAc)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "gecici_cidr": norm[0], "gecici_bitis": bitis})
}

func (h *Handlers) erisimAudit(r *http.Request, eylem string) {
	var uid int64
	kadi := ""
	if c := middleware.ClaimsFrom(r); c != nil {
		uid, kadi = c.UserID, c.Username
	}
	auth.WriteAudit(h.DB, uid, kadi, httpx.ClientIP(r), eylem, "panel", true)
}

type erisimKaydetReq struct {
	CIDRler []string `json:"cidrler"`
}

// ErisimKisitiKaydet ayarı ancak mevcut istemci yeni listede kalıyorsa uygular.
// Böylece yönetici tek istekle kendi panel erişimini kesemez.
func (h *Handlers) ErisimKisitiKaydet(w http.ResponseWriter, r *http.Request) {
	var req erisimKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	norm, aglar, err := normalizeCIDRler(req.CIDRler)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	istemci := net.ParseIP(httpx.ClientIP(r))
	if len(aglar) > 0 && !ipIzinli(istemci, aglar) {
		httpx.WriteError(w, http.StatusBadRequest,
			"mevcut istemci IP adresiniz ("+httpx.ClientIP(r)+") izin listesinde değil")
		return
	}
	ham := strings.Join(norm, "\n")
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET erisim_cidrleri=? WHERE id=1`, ham); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	eylem := auditPanelErisimAc
	if len(norm) == 0 {
		eylem = auditPanelErisimKapat
	}
	var uid int64
	kadi := ""
	if c := middleware.ClaimsFrom(r); c != nil {
		uid, kadi = c.UserID, c.Username
	}
	auth.WriteAudit(h.DB, uid, kadi, httpx.ClientIP(r), eylem, "panel", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "cidrler": norm, "istemci_ip": httpx.ClientIP(r),
	})
}

func satirlar(ham string) []string {
	var sonuc []string
	for _, s := range strings.FieldsFunc(ham, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		if s = strings.TrimSpace(s); s != "" {
			sonuc = append(sonuc, s)
		}
	}
	return sonuc
}

func normalizeCIDRler(girdiler []string) ([]string, []*net.IPNet, error) {
	var norm []string
	var aglar []*net.IPNet
	goruldu := map[string]bool{}
	for _, girdi := range girdiler {
		for _, s := range satirlar(girdi) {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s = ip.String() + "/32"
				} else {
					s = ip.String() + "/128"
				}
			}
			_, ag, err := net.ParseCIDR(s)
			if err != nil {
				return nil, nil, &cidrHatasi{deger: s}
			}
			s = ag.String()
			if !goruldu[s] {
				goruldu[s] = true
				norm = append(norm, s)
				aglar = append(aglar, ag)
			}
		}
	}
	return norm, aglar, nil
}

type cidrHatasi struct{ deger string }

func (e *cidrHatasi) Error() string { return "geçersiz IP/CIDR: " + e.deger }

func ipIzinli(ip net.IP, aglar []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, ag := range aglar {
		if ag.Contains(ip) {
			return true
		}
	}
	return false
}
