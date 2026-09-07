package middleware

import (
	"net"
	"net/http"
	"sanalcp/internal/httpx"
	"strings"
	"sync"
	"time"
)

type panelHizKayit struct {
	pencere time.Time
	adet    int
}

var panelHizMu sync.Mutex
var panelHizSayac = map[string]panelHizKayit{}

// PanelHizLimiti tüm panel API çağrılarına dakika pencereli bir üst sınır koyar.
// Girişteki başarısız-deneme kilidi ayrıca ve daha sıkı biçimde çalışmaya devam eder.
// Ayar satırı her istekte DB'den değil kısa TTL'li önbellekten okunur (panelayar_cache.go).
func PanelHizLimiti(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, err := panelAyarlariOku(r.Context())
		if err != nil {
			httpx.WriteError(w, 503, "panel hız ayarı doğrulanamadı")
			return
		}
		profil, istisna := o.HizProfil, o.HizIstisna
		limit, burst := o.HizIstek, o.HizBurst
		if profil == "kapali" || panelIPIstisna(httpx.ClientIP(r), istisna) {
			next.ServeHTTP(w, r)
			return
		}
		if profil == "dengeli" {
			limit, burst = 600, 100
		}
		if profil == "siki" {
			limit, burst = 120, 20
		}
		esik := limit + burst
		anahtar := httpx.ClientIP(r)
		simdi := time.Now()
		dakika := simdi.Truncate(time.Minute)
		panelHizMu.Lock()
		k := panelHizSayac[anahtar]
		if k.pencere != dakika {
			k = panelHizKayit{pencere: dakika}
		}
		k.adet++
		panelHizSayac[anahtar] = k
		if len(panelHizSayac) > 10000 {
			for ip, x := range panelHizSayac {
				if x.pencere.Before(dakika) {
					delete(panelHizSayac, ip)
				}
			}
		}
		panelHizMu.Unlock()
		if k.adet > esik {
			w.Header().Set("Retry-After", "60")
			// Aynı IP/pencere için yalnız ilk reddi yaz; saldırganın 429 yağmuruyla
			// DB'yi olay satırlarıyla doldurmasına izin verme.
			if k.adet == esik+1 {
				_, _ = scopeDB.ExecContext(r.Context(), `INSERT INTO panel_hiz_olaylari(ip,yol) VALUES(?,?)`, anahtar, kisaltYol(r.URL.Path))
			}
			httpx.WriteError(w, 429, "panel istek hızı sınırı aşıldı")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func panelIPIstisna(ipS, ham string) bool {
	ip := net.ParseIP(ipS)
	if ip == nil {
		return false
	}
	for _, s := range strings.FieldsFunc(ham, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		s = strings.TrimSpace(s)
		if x := net.ParseIP(s); x != nil && x.Equal(ip) {
			return true
		}
		if _, n, e := net.ParseCIDR(s); e == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}
func kisaltYol(s string) string {
	if len(s) > 255 {
		return s[:255]
	}
	return s
}
