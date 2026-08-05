package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorBody{Hata: msg})
}

type ErrorBody struct {
	Hata string `json:"hata"`
}

var trustedProxies atomic.Value // []*net.IPNet

func init() {
	trustedProxies.Store([]*net.IPNet{})
}

// SetTrustedProxies, X-Forwarded-For / X-Real-IP başlıklarına güvenilecek
// ters-vekil (reverse proxy) CIDR'larını ayarlar. Sunucu başlamadan önce bir
// kez çağrılmalı. Boş/çağrılmamışsa ClientIP bu başlıkları hiç okumaz —
// varsayılan fail-closed davranış budur.
func SetTrustedProxies(cidrs []*net.IPNet) {
	trustedProxies.Store(cidrs)
}

// ClientIP gerçek istemci adresini döner. r.RemoteAddr (TCP bağlantısının
// gerçek kaynağı, spoof edilemez) tek güvenilir kaynaktır — X-Forwarded-For
// ve X-Real-IP yalnızca doğrudan bağlanan taraf SetTrustedProxies ile
// tanımlanmış güvenilir bir vekil listesindeyse dikkate alınır. Aksi halde
// bu başlıklar saldırgan tarafından serbestçe ayarlanabilir ve IP-bazlı hız
// sınırlarını (bkz. middleware.GirisLimiti) atlatmak için kullanılabilir.
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := lastColon(r.RemoteAddr); i > 0 {
		host = r.RemoteAddr[:i]
	}

	if isTrustedProxy(r.RemoteAddr) {
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			for i := 0; i < len(v); i++ {
				if v[i] == ',' {
					return strings.TrimSpace(v[:i])
				}
			}
			return strings.TrimSpace(v)
		}
		if v := r.Header.Get("X-Real-IP"); v != "" {
			return strings.TrimSpace(v)
		}
	}

	return host
}

func isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	cidrs, _ := trustedProxies.Load().([]*net.IPNet)
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// ExtendDeadline, mevcut isteğin soket okuma/yazma zaman aşımını d kadar
// uzatır. SADECE büyük yükleme/indirme uçlarında (dosya, arşiv, DB dump)
// çağrılmalı — sunucu genelindeki kısa varsayılan ReadTimeout/WriteTimeout
// (bkz. cmd/server/main.go) geri kalan tüm uçları slow-DoS'a karşı korur;
// bu fonksiyon yalnız gerçekten uzun sürmesi beklenen istekler için istisna
// açar. ResponseWriter zinciri http.ResponseController'ı desteklemiyorsa
// (ör. test ortamı) sessizce yok sayılır.
func ExtendDeadline(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(d)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}
