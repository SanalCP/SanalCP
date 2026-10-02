package middleware

import (
	"bytes"
	"log"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"

	"sanalcp/internal/httpx"
)

// HataAyrintisiGizle: admin dışındaki rollere 5xx yanıtlarda iç hata
// ayrıntısı göstermez.
//
// NEDEN: ~300 uç 500 yanıtına err.Error() ya da exec çıktısı koyuyor (SQL
// hata metni, dosya yolları, komut/sürüm bilgisi). Bunlar sunucunun sahibi
// olan admin için değerli tanı bilgisidir; bayi ve müşteri için ise iç yapıyı
// sızdıran bilgidir. Uçları tek tek değiştirmek yerine kimlik doğrulanmış
// grubun tamamında yanıt katmanında süzülür: admin aynı yanıtı alır, diğer
// roller genel bir mesaj ve istek kimliği alır; ayrıntı sunucu günlüğüne
// istek kimliğiyle yazılır, böylece destek talebinde eşleştirilebilir.
//
// RequireAuth'tan SONRA kullanılmalıdır (rolü claim'lerden okur).
func HataAyrintisiGizle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := ClaimsFrom(r); c != nil && c.Role == RolAdmin {
			next.ServeHTTP(w, r)
			return
		}
		g := &hataGizleyici{ResponseWriter: w, r: r}
		next.ServeHTTP(g, r)
		g.bitir()
	})
}

// hataGizleyiciMaks: gizlenen gövdenin günlüğe yazılacak en fazla kısmı.
const hataGizleyiciMaks = 4 << 10

type hataGizleyici struct {
	http.ResponseWriter
	r       *http.Request
	kod     int
	gizle   bool
	yazildi bool
	tampon  bytes.Buffer
}

func (g *hataGizleyici) WriteHeader(kod int) {
	if g.yazildi {
		return
	}
	g.yazildi = true
	g.kod = kod
	// Yalnız JSON hata gövdeleri süzülür: dosya indirme/SSE gibi akışlar ya da
	// 503 + Retry-After gibi bilinçli yanıtlar handler'ın yazdığı gibi gider.
	ct := g.Header().Get("Content-Type")
	if kod >= 500 && kod != http.StatusServiceUnavailable && (ct == "" || strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "text/plain")) {
		g.gizle = true
		return
	}
	g.ResponseWriter.WriteHeader(kod)
}

func (g *hataGizleyici) Write(b []byte) (int, error) {
	if !g.yazildi {
		g.WriteHeader(http.StatusOK)
	}
	if g.gizle {
		if kalan := hataGizleyiciMaks - g.tampon.Len(); kalan > 0 {
			if len(b) > kalan {
				g.tampon.Write(b[:kalan])
			} else {
				g.tampon.Write(b)
			}
		}
		return len(b), nil
	}
	return g.ResponseWriter.Write(b)
}

// Flush: SSE uçları (ör. canlı günlük) http.ResponseController ile flush eder.
func (g *hataGizleyici) Flush() {
	if g.gizle {
		return
	}
	_ = http.NewResponseController(g.ResponseWriter).Flush()
}

func (g *hataGizleyici) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *hataGizleyici) bitir() {
	if !g.gizle {
		return
	}
	id := chimw.GetReqID(g.r.Context())
	log.Printf("5xx ayrıntısı gizlendi (istek=%s %s %s, durum=%d): %s",
		id, g.r.Method, g.r.URL.Path, g.kod, strings.TrimSpace(g.tampon.String()))
	g.Header().Del("Content-Length")
	mesaj := "işlem sunucu tarafında başarısız oldu"
	if id != "" {
		mesaj += " (hata kimliği: " + id + ")"
	}
	httpx.WriteError(g.ResponseWriter, g.kod, mesaj)
}
