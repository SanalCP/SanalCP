package middleware

import (
	"net"
	"net/http"
	"strings"

	"sanalcp/internal/httpx"
)

// PanelErisimKisiti, panel_ayarlari'ndaki erişim listesini kısa TTL'li önbellekten
// okur (bkz. panelayar_cache.go) — ayar değişikliği yeniden başlatma gerektirmez,
// en geç TTL kadar sonra yansır. DB'ye ilk dolumda erişilemezse güvenli tarafta
// kalıp erişimi keser.
func PanelErisimKisiti(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, err := panelAyarlariOku(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "panel erişim ayarı doğrulanamadı")
			return
		}
		ham := o.ErisimHam
		if strings.TrimSpace(ham) == "" {
			next.ServeHTTP(w, r)
			return
		}
		ip := net.ParseIP(strings.Trim(httpx.ClientIP(r), "[]"))
		if o.GeciciAktif {
			_, ag, err := net.ParseCIDR(o.GeciciCIDR)
			if err == nil && ip != nil && ag.Contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}
		for _, s := range strings.FieldsFunc(ham, func(c rune) bool { return c == '\n' || c == '\r' || c == ',' }) {
			_, ag, err := net.ParseCIDR(strings.TrimSpace(s))
			if err == nil && ip != nil && ag.Contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}
		httpx.WriteError(w, http.StatusForbidden, "bu IP adresinden panel erişimine izin verilmiyor")
	})
}
