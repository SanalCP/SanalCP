package middleware

import (
	"net"
	"net/http"
	"strings"

	"sanalcp/internal/httpx"
)

// PanelErisimKisiti, panel_ayarlari listesini her istekte okur; ayar değişikliği
// yeniden başlatma gerektirmez. DB okunamazsa güvenli tarafta kalıp erişimi keser.
func PanelErisimKisiti(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scopeDB == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "panel erişim ayarı doğrulanamadı")
			return
		}
		var ham, gecici string
		var geciciAktif int
		if err := scopeDB.QueryRowContext(r.Context(),
			`SELECT COALESCE(erisim_cidrleri,''), COALESCE(gecici_erisim_cidr,''),
			        COALESCE(gecici_erisim_bitis > NOW(),0)
			   FROM panel_ayarlari WHERE id=1`).Scan(&ham, &gecici, &geciciAktif); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "panel erişim ayarı doğrulanamadı")
			return
		}
		if strings.TrimSpace(ham) == "" {
			next.ServeHTTP(w, r)
			return
		}
		ip := net.ParseIP(strings.Trim(httpx.ClientIP(r), "[]"))
		if geciciAktif == 1 {
			_, ag, err := net.ParseCIDR(gecici)
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
