package middleware

import (
	"context"
	"net/http"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

// demoModuSorgu: panelbayrak.DemoModuAcik'in çalıştırdığı sorgu — test
// dosyasında sqlmock.ExpectQuery eşleşmesi için burada da tutulur.
const demoModuSorgu = `SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`

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
// Yalnızca TESTLER için (bkz. ClaimsIle'nin üstündeki aynı uyarı) — üretimde
// bunu DemoSaltOkunur yazar.
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
// uçlar — oturum açma/kapama. Başka HİÇBİR yazma demo modunda geçmez.
var demoYazmaBeyazListesi = map[string]bool{
	"POST /api/v1/auth/login": true,
	"POST /api/v1/auth/cikis": true,
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
		acik := panelbayrak.DemoModuAcik(r.Context(), scopeDB)
		if acik {
			r = r.WithContext(context.WithValue(r.Context(), demoCtxKey{}, true))
		}
		if !acik {
			next.ServeHTTP(w, r)
			return
		}
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
