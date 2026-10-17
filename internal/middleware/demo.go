package middleware

import (
	"context"
	"net/http"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

type demoCtxKey struct{}

// DemoPaneliMi: bu istek demo-modu AÇIK bir panelden mi geliyor?
//
// Yalnız DemoSaltOkunur zincirdeyken güvenilirdir — o middleware bayrağı
// bir kez okuyup context'e işler, sır döndüren handler'lar (bkz.
// internal/domains, internal/mail, internal/files) her istekte ikinci bir
// DB sorgusu atmadan bunu okur.
func DemoPaneliMi(r *http.Request) bool {
	v, _ := r.Context().Value(demoCtxKey{}).(bool)
	return v
}

// DemoIle: isteğe demo-modu bayrağı iliştirilmiş bir kopyasını döner.
//
// Üretimde yalnız DemoSaltOkunur çağırır; handler'lar ve testler başka
// yerde çağırmamalı (bkz. ClaimsIle'nin üstündeki aynı uyarı).
func DemoIle(r *http.Request, acik bool) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), demoCtxKey{}, acik))
}

// Maskele: demoPanel true ise sabit bir maske döner, değilse deger'i
// olduğu gibi bırakır. Sır döndüren tüm demo-farkında handler'lar bunu
// kullanır — tek yerde tanımlı, tek görünüm.
func Maskele(demoPanel bool, deger string) string {
	if demoPanel {
		return "••••••••"
	}
	return deger
}

// demoYazmaBeyazListesi: demo modunda dahi izinli kalan TEK state-changing
// uçlar — oturum açma/kapama (yönetici ve müşteri girişi; çıkış ikisinde de
// /auth/cikis). Başka HİÇBİR yazma demo modunda geçmez.
var demoYazmaBeyazListesi = map[string]bool{
	"POST /api/v1/auth/login":    true,
	"POST /api/v1/musteri/login": true,
	"POST /api/v1/auth/cikis":    true,
}

// DemoSaltOkunur: panel_ayarlari.demo_modu_acik açıkken tüm yazan istekleri
// (beyaz liste hariç) 403 ile reddeder. GET/HEAD/OPTIONS her zaman serbest —
// tanım gereği state değiştirmezler (CSRFKoruma'daki muafiyetle tutarlı,
// bkz. internal/middleware/csrf.go).
//
// Global zincire CSRFKoruma'dan hemen sonra, TÜM route'lardan önce eklenir
// (bkz. cmd/server/main.go) — git-webhook ve pma-redeem gibi RequireAuth
// dışındaki uçlar da dahil olmak üzere hiçbir route'a tek tek dokunulmaz.
func DemoSaltOkunur(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !panelbayrak.DemoModuAcik(r.Context(), scopeDB) {
			next.ServeHTTP(w, r)
			return
		}
		r = DemoIle(r, true)
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if demoYazmaBeyazListesi[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		httpx.WriteError(w, http.StatusForbidden, "demo modunda değişiklik yapılamaz")
	})
}

// DemoKapali: demo modunda isteği 403 ile kapatır. Ham metin döndüren
// (log, dump) ve tek alan gibi maskelenemeyen GET uçları için — route
// tanımında takılır (bkz. cmd/server/main.go). Handler içi DemoPaneliMi
// kontrolüyle aynı sonuç; internal/system gibi bu paketi import edemeyen
// (döngü) paketlerin uçları da böylece kapsanır.
func DemoKapali(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if DemoPaneliMi(r) {
			httpx.WriteError(w, http.StatusForbidden, "demo modunda kullanılamaz")
			return
		}
		next.ServeHTTP(w, r)
	})
}
