package panelayarlari

import (
	"encoding/json"
	"net"
	"net/http"
	"sanalcp/internal/httpx"
	"strings"
)

type panelHizAyar struct {
	Profil        string   `json:"profil"`
	IstekDakika   int      `json:"istek_dakika"`
	Burst         int      `json:"burst"`
	IPIstisnalari []string `json:"ip_istisnalari"`
}

func (h *Handlers) HizLimiti(w http.ResponseWriter, r *http.Request) {
	var a panelHizAyar
	var ips string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT hiz_profili,hiz_istek_dakika,hiz_burst,COALESCE(hiz_ip_istisnalari,'') FROM panel_ayarlari WHERE id=1`).Scan(&a.Profil, &a.IstekDakika, &a.Burst, &ips); err != nil {
		httpx.WriteError(w, 500, "ayar okunamadı")
		return
	}
	a.IPIstisnalari = satirlar(ips)
	rows, _ := h.DB.QueryContext(r.Context(), `SELECT DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),ip,yol FROM panel_hiz_olaylari ORDER BY id DESC LIMIT 50`)
	defer func() {
		if rows != nil {
			rows.Close()
		}
	}()
	olaylar := []map[string]any{}
	if rows != nil {
		for rows.Next() {
			var z, ip, y string
			if rows.Scan(&z, &ip, &y) == nil {
				olaylar = append(olaylar, map[string]any{"zaman": z, "ip": ip, "yol": y, "durum": 429})
			}
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"ayar": a, "olaylar": olaylar, "istemci_ip": httpx.ClientIP(r)})
}
func (h *Handlers) HizLimitiKaydet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ayar panelHizAyar `json:"ayar"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	a := req.Ayar
	if a.Profil != "kapali" && a.Profil != "dengeli" && a.Profil != "siki" && a.Profil != "ozel" {
		httpx.WriteError(w, 400, "geçersiz profil")
		return
	}
	if a.Profil == "dengeli" {
		a.IstekDakika, a.Burst = 600, 100
	}
	if a.Profil == "siki" {
		a.IstekDakika, a.Burst = 120, 20
	}
	if a.IstekDakika < 1 || a.IstekDakika > 60000 || a.Burst < 0 || a.Burst > 10000 {
		httpx.WriteError(w, 400, "geçersiz limit")
		return
	}
	var norm []string
	for _, x := range a.IPIstisnalari {
		for _, s := range satirlar(x) {
			if ip := net.ParseIP(s); ip != nil {
				norm = append(norm, ip.String())
				continue
			}
			if _, n, e := net.ParseCIDR(s); e == nil {
				norm = append(norm, n.String())
				continue
			}
			httpx.WriteError(w, 400, "geçersiz IP/CIDR: "+s)
			return
		}
	}
	if _, e := h.DB.ExecContext(r.Context(), `UPDATE panel_ayarlari SET hiz_profili=?,hiz_istek_dakika=?,hiz_burst=?,hiz_ip_istisnalari=? WHERE id=1`, a.Profil, a.IstekDakika, a.Burst, strings.Join(norm, "\n")); e != nil {
		httpx.WriteError(w, 500, "DB: "+e.Error())
		return
	}
	a.IPIstisnalari = norm
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "ayar": a})
}
