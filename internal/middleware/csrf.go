package middleware

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
)

// 🔴 GÜVENLİK (CSRF): Panel oturumu artık HttpOnly çerezle taşınıyor, yani
// tarayıcı kimliği her state-changing isteğe KENDİLİĞİNDEN ekliyor. Bu, klasik
// cookie-CSRF'i gerçek bir yüzey hâline getirir ve bu middleware'i dolaylı bir
// sertleştirmeden ZORUNLU bir savunmaya çevirir.
//
// Tarayıcı katmanında ilk savunma SameSite=Strict'tir (bkz. auth/cookie.go):
// çerez cross-site bir istekte hiç gönderilmez. Buradaki origin kontrolü onun
// ikinci katmanıdır — SameSite'a güvenmemek gerekir, çünkü eski bir tarayıcı,
// bir uzantı ya da ileride SameSite'ı gevşetecek bir değişiklik tek hamlede
// korumayı kaldırabilir.
//
// Kural:
//   - GET/HEAD/OPTIONS/TRACE muaf — tanım gereği state değiştirmezler.
//   - Origin varsa host'u panelin host'uyla eşleşmeli, yoksa 403.
//   - Origin yok ama Referer varsa Referer'ın host'u kontrol edilir.
//   - İkisi de yoksa: OTURUM ÇEREZİ VARSA 403, yoksa istek geçer.
//
// Son maddedeki ayrım bu tasarımın kilit noktasıdır. Çerez taşıyan bir istek
// tanım gereği tarayıcı kaynaklıdır; böyle bir istekte Origin'in de Referer'ın
// da bulunmaması normal değildir, dolayısıyla fail-closed davranılır. Buna
// karşılık çerezsiz istekler curl, API token'lı otomasyon (scp_…) ve git
// webhook'udur: tarayıcı kimliği taşımadıkları için CSRF yüzeyleri yoktur,
// onları kırmak paneli kullanılamaz yapar ve karşılığında hiçbir savunma
// eklemez.
//
// (Çerez öncesi bu son madde koşulsuz "geçer" idi. O sırada güvenliydi, çünkü
// oturum Authorization başlığındaydı ve tarayıcı o başlığı cross-origin bir
// isteğe kendiliğinden eklemez. Çereze geçişle birlikte aynı kural açığa
// dönüşürdü — ikisi ayrılmaz bir çifttir.)
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
			if auth.OturumCerezDegeri(r) != "" {
				// Oturum çerezi var => istek tarayıcıdan geliyor. Origin'i de
				// Referer'ı da olmayan bir tarayıcı isteği meşru değildir.
				httpx.WriteError(w, http.StatusForbidden, "istek kaynağı doğrulanamadı (CSRF koruması)")
				return
			}
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
