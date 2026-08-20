package middleware

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"sanalcp/internal/httpx"
)

// 🔴 GÜVENLİK (CSRF): Panel oturumu localStorage'daki JWT ile taşınır ve her
// istekte Authorization başlığına konur. Tarayıcı bu başlığı cross-origin bir
// isteğe KENDİLİĞİNDEN eklemez, dolayısıyla klasik cookie-CSRF'i bugün yoktur.
// Ancak state-changing uçların tek savunması bu dolaylı özellikti: ileride
// cookie'ye geçilirse (bkz. SECURITY_AUDIT 2.1) veya bir eklenti CORS'u
// gevşetirse, koruma tek hamlede yok olurdu. Bu middleware o zemini kapatır ve
// aynı zamanda "yanlış origin'den gelen istek" için erken, ucuz bir reddir.
//
// Kural:
//   - GET/HEAD/OPTIONS/TRACE muaf — tanım gereği state değiştirmezler.
//   - Origin varsa host'u panelin host'uyla eşleşmeli, yoksa 403.
//   - Origin yok ama Referer varsa Referer'ın host'u kontrol edilir.
//   - İkisi de yoksa istek GEÇER. Bu bilinçlidir: modern tarayıcılar
//     cross-site fetch/XHR/form POST'larında Origin'i HER ZAMAN gönderir, yani
//     Origin'siz bir istek tarayıcı kaynaklı değildir — curl, API token'lı
//     otomasyon (scp_…), git webhook'u. Bunları kırmak paneli kullanılamaz
//     yapar, karşılığında CSRF savunması eklemez.
//
// Port karşılaştırması KASTEN yapılmaz: nginx vhost'u `proxy_set_header Host
// $host` kullanır ve $host port taşımaz, oysa tarayıcının gönderdiği Origin
// portludur (https://sunucu:8443). Ham port kıyaslaması her isteği reddederdi.
// Hostname eşleşmesi saldırgan-origin'i (farklı alan adı) yine yakalar; aynı
// hostname'in başka bir portunda düşman içerik barınması panel sunucusunda
// mümkün değildir — müşteri siteleri kendi alan adlarında sunulur.
//
// PANEL_CSRF_ORIGIN ile bir veya birden çok origin/host açıkça beyan
// edilebilir (virgülle ayrılır); verildiğinde r.Host yerine bu liste kullanılır.
func CSRFKoruma(next http.Handler) http.Handler {
	izinli := csrfIzinliHostlar()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}

		kaynak := r.Header.Get("Origin")
		if kaynak == "" {
			kaynak = r.Header.Get("Referer")
		}
		if kaynak == "" {
			// Tarayıcı kaynaklı değil (curl / API token / webhook) — CSRF yüzeyi yok.
			next.ServeHTTP(w, r)
			return
		}

		gelen := hostAyikla(kaynak)
		if gelen == "" {
			httpx.WriteError(w, http.StatusForbidden, "geçersiz istek kaynağı")
			return
		}

		hedefler := izinli
		if len(hedefler) == 0 {
			hedefler = []string{hostname(r.Host)}
		}
		for _, h := range hedefler {
			if h != "" && strings.EqualFold(h, gelen) {
				next.ServeHTTP(w, r)
				return
			}
		}
		httpx.WriteError(w, http.StatusForbidden, "istek farklı bir kaynaktan geldi (CSRF koruması)")
	})
}

// csrfIzinliHostlar: PANEL_CSRF_ORIGIN içeriğini hostname listesine çevirir.
// Hem tam origin (https://panel.example.com:8443) hem düz hostname kabul edilir.
func csrfIzinliHostlar() []string {
	ham := strings.TrimSpace(os.Getenv("PANEL_CSRF_ORIGIN"))
	if ham == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(ham, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if h := hostAyikla(p); h != "" {
			out = append(out, h)
			continue
		}
		out = append(out, hostname(p))
	}
	return out
}

// hostAyikla: "https://sunucu:8443/yol" → "sunucu". Şema yoksa "" döner.
func hostAyikla(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return hostname(u.Host)
}

// hostname: "sunucu:8443" → "sunucu"; portsuzsa olduğu gibi. IPv6 köşeli
// parantezleri de temizlenir.
func hostname(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if k, _, err := net.SplitHostPort(h); err == nil {
		return strings.Trim(k, "[]")
	}
	return strings.Trim(h, "[]")
}
