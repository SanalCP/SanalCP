// Package metrics — Prometheus gözlemlenebilirliği. Toplama Middleware ile ana
// API router'ında (public), sunum ise Handler ile YALNIZ loopback-only CLI
// sunucusunda yapılmalı (bkz. cmd/server/main.go) — /metrics dışa asla açılmamalı,
// route sayıları ve gecikme dağılımı gibi iç bilgiyi sızdırır.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	istekSayisi = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sanalcp_http_istekleri_toplam",
		Help: "Panel API'sine gelen HTTP isteklerinin toplam sayısı.",
	}, []string{"yontem", "yol", "durum"})

	istekSuresi = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sanalcp_http_istek_suresi_saniye",
		Help:    "HTTP isteklerinin işlenme süresi (saniye).",
		Buckets: prometheus.DefBuckets,
	}, []string{"yontem", "yol"})
)

type durumYazici struct {
	http.ResponseWriter
	durum int
}

func (d *durumYazici) WriteHeader(kod int) {
	d.durum = kod
	d.ResponseWriter.WriteHeader(kod)
}

// Unwrap alttaki gerçek ResponseWriter'ı döner — http.ResponseController'ın
// (ör. büyük yükleme/indirme uçlarında per-istek soket deadline uzatması,
// bkz. httpx.ExtendDeadline) bu sarmalayıcının arkasına geçebilmesi için gerekli.
func (d *durumYazici) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}

// Middleware — chi router'ına global r.Use(...) ile takılır; her isteğin
// sayısını ve süresini "yol" etiketiyle (ham path DEĞİL, chi route şablonu —
// örn. /domains/{id}, /domains/123 değil) kaydeder; aksi hâlde her domain/id
// ayrı bir seri açar ve kardinalite patlar.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		basla := time.Now()
		ww := &durumYazici{ResponseWriter: w, durum: http.StatusOK}
		next.ServeHTTP(ww, r)

		yol := chi.RouteContext(r.Context()).RoutePattern()
		if yol == "" {
			yol = "diger" // eşleşmeyen (404) istekler — tek bir sepette toplanır
		}
		istekSayisi.WithLabelValues(r.Method, yol, strconv.Itoa(ww.durum)).Inc()
		istekSuresi.WithLabelValues(r.Method, yol).Observe(time.Since(basla).Seconds())
	})
}

// Handler — /metrics scrape ucu.
func Handler() http.Handler {
	return promhttp.Handler()
}
