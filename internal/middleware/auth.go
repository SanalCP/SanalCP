package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"sanalpanel/internal/auth"
	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// scopeDB: MusteriScope'un domain askıya-alma durumunu kontrol edebilmesi için DB
// handle'ı. main() içinde middleware.Init(db) ile set edilir. nil ise askı kontrolü
// atlanır (mc.DomainID eşleşmesi zaten şarttır; askı yalnızca EK bir kısıttır).
var scopeDB *sql.DB

// Init: middleware paketine DB handle'ı verir (müşteri-scope askı kontrolü için).
func Init(db *sql.DB) { scopeDB = db }

type ctxKey int

const (
	claimsKey        ctxKey = 1
	musteriClaimsKey ctxKey = 2
)

// RequireAuth: hem admin (auth.Claims) hem müşteri (auth.MusteriClaims) token'larını kabul eder.
// Müşteri ise context'e MusteriClaims, admin ise Claims yerleştirir.
func RequireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get("Authorization"))
			const p = "Bearer "
			if !strings.HasPrefix(raw, p) {
				httpx.WriteError(w, http.StatusUnauthorized, "yetkilendirme gerekli")
				return
			}
			tokenRaw := raw[len(p):]
			// CVE-2025-30204 savunma: dogrulama ONCESI asiri-uzun token reddedilir (pre-auth DoS yuzeyi kucultulur)
			if len(tokenRaw) > 8192 {
				httpx.WriteError(w, http.StatusUnauthorized, "geçersiz oturum")
				return
			}

			// Önce admin claims dene
			if c, err := auth.Parse(secret, tokenRaw); err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, c)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Sonra müşteri claims dene
			if mc, err := auth.ParseMusteri(secret, tokenRaw); err == nil {
				ctx := context.WithValue(r.Context(), musteriClaimsKey, mc)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			httpx.WriteError(w, http.StatusUnauthorized, "geçersiz oturum")
		})
	}
}

// RequireRole: sadece admin rol kontrolü
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := ClaimsFrom(r)
			if c == nil || !allowed[c.Role] {
				httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Rol sabitleri — users.role ENUM('admin','reseller','user') ile birebir.
const (
	RolAdmin   = "admin"
	RolBayi    = "reseller"
	RolMusteri = "user"
)

// AdminOnly: yalnız rol=admin geçer.
//
// 🔴 GÜVENLİK: Bu fonksiyon eskiden yalnız "admin tipi token var mı" diye
// bakıyordu (ClaimsFrom(r) == nil), rolü hiç okumuyordu. Tek token tipi
// üretildiği sürece (root → rol=admin) zararsızdı; ama bayi hesaplarına
// auth.Claims verildiği anda 87 admin ucunun tamamı — güvenlik duvarı, servis
// yeniden başlatma, paket kurulumu dahil — bayiye açılırdı. Rol kontrolü
// çok kullanıcılı desteğin ÖNKOŞULUDUR, sonradan eklenecek bir iyileştirme
// değil.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsFrom(r)
		if c == nil || c.Role != RolAdmin {
			httpx.WriteError(w, http.StatusForbidden, "sadece yöneticiler için")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BayiVeUstu: rol=admin veya rol=reseller geçer.
//
// İki tür uçta kullanılır:
//   - Hesap işlemleri (domain, müşteri, DNS, SSL...) — bayi KENDİ kapsamında;
//     kapsam daraltması ayrıca DomainKapsam/MusteriKapsam ile yapılır, bu
//     middleware yalnız "rol yeterli mi" sorusunu yanıtlar.
//   - Salt-okunur sunucu bilgisi (servis durumu, yük, sürüm) — bayinin destek
//     verebilmesi için görünür, ama değiştiren uçlar AdminOnly'de kalır.
func BayiVeUstu(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsFrom(r)
		if c == nil || (c.Role != RolAdmin && c.Role != RolBayi) {
			httpx.WriteError(w, http.StatusForbidden, "bu işlem için yetkiniz yok")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MusteriScope: müşteri token'ı varsa URL'deki {id} domain ID müşterinin domain'iyle eşleşmeli.
// Admin ise serbest. Param adı varsayılan "id" — farklı param için MusteriScopeParam.
func MusteriScope(next http.Handler) http.Handler {
	return MusteriScopeParam("id")(next)
}

func MusteriScopeParam(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 🔴 GÜVENLİK: burada eskiden yalnız "ClaimsFrom(r) != nil" vardı,
			// yani auth.Claims taşıyan HER token admin muamelesi görüp kapsam
			// denetimini atlıyordu. Bayi token'ları da auth.Claims olduğu için
			// bu, 141 müşteri-kapsamlı ucun tamamına kapsamsız erişim demekti —
			// AdminOnly'deki aynı hatanın daha geniş yüzeyli ikizi.
			if c := ClaimsFrom(r); c != nil {
				switch c.Role {
				case RolAdmin:
					next.ServeHTTP(w, r) // admin: tüm domainler
					return
				case RolBayi:
					urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
					if !BayiDomainiMi(r, c.UserID, urlID) {
						httpx.WriteError(w, http.StatusForbidden, "bu domain'e erişim yok")
						return
					}
					next.ServeHTTP(w, r)
					return
				case RolMusteri:
					// users hesabıyla giren müşteri (Faz 5C). Eski FTP kimlikli
					// oturumdan farkı: birden çok domaini olabilir, bu yüzden
					// kapsam zincirden çözülür.
					urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
					if !MusteriKullanicisininDomainiMi(r, c.UserID, urlID) {
						httpx.WriteError(w, http.StatusForbidden, "bu domain'e erişim yok")
						return
					}
					if domainAskidaMi(r, urlID) {
						httpx.WriteError(w, http.StatusForbidden, "hesap askıya alınmış")
						return
					}
					next.ServeHTTP(w, r)
					return
				default:
					httpx.WriteError(w, http.StatusForbidden, "bu domain'e erişim yok")
					return
				}
			}
			mc := MusteriClaimsFrom(r)
			if mc == nil {
				httpx.WriteError(w, http.StatusUnauthorized, "yetkilendirme gerekli")
				return
			}
			urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
			if urlID != mc.DomainID {
				httpx.WriteError(w, http.StatusForbidden, "bu domain'e erişim yok")
				return
			}
			// Askıya-alma zorlaması: askıdaki domain için müşteri token'ı (önceden
			// verilmiş/hâlâ geçerli olsa bile) TÜM işlemlerde 403 alır. Admin bu
			// bloktan önce (ClaimsFrom != nil) zaten geçmiştir; yönetici askıyı kaldırabilir.
			if domainAskidaMi(r, mc.DomainID) {
				httpx.WriteError(w, http.StatusForbidden, "hesap askıya alınmış")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DomainSahibiMi: verilen domain ID cagirana ait mi? Merkezi sahiplik denetimi.
//   - Admin token   => her zaman true (tum domainlere erisir).
//   - Bayi token    => domain, bayinin yonettigi bir musteriye aitse true.
//   - Musteri token => yalniz kendi DomainID'siyle eslesiyorsa true.
//   - Kimlik yoksa   => false.
//
// MusteriScope middleware'inin handler-ici eslenigi: URL'de {id} domain param'i
// bulunmayan (or. {dbId} gibi turev kaynak) uclarda, kaynagin domain_id'si DB'den
// cozuldukten sonra bu fonksiyonla sahiplik dogrulanir.
func DomainSahibiMi(r *http.Request, domainID int64) bool {
	if c := ClaimsFrom(r); c != nil {
		if c.Role == RolAdmin {
			return true // admin: tum domainlere erisir
		}
		if c.Role == RolBayi {
			return BayiDomainiMi(r, c.UserID, domainID)
		}
		return false
	}
	if mc := MusteriClaimsFrom(r); mc != nil {
		return mc.DomainID == domainID
	}
	return false
}

// BayiDomainiMi: domain, verilen bayinin yonettigi bir musteriye mi ait?
//
// Sahiplik zinciri tek yerde cozulur: domains.customer_id -> customers.owner_user_id.
// Yetki karari her zaman DB'den okunur, token'daki bir listeden degil — bayi
// bir musteriyi devrettiginde/kaybettiginde eski token aninda gecersizlesmelidir.
//
// FAIL-CLOSED: DB okunamazsa false doner (erisim reddedilir).
func BayiDomainiMi(r *http.Request, bayiUserID, domainID int64) bool {
	if scopeDB == nil || bayiUserID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(), `
		SELECT COUNT(*)
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE d.id = ? AND c.owner_user_id = ?`, domainID, bayiUserID).Scan(&n)
	return err == nil && n > 0
}

// domainAskidaMi: domain askıya alınmış mı? Askı, müşteri tarafındaki TÜM
// işlemleri kapatır (token önceden verilmiş ve hâlâ geçerli olsa bile).
// Admin/bayi bu kontrolden önce geçer — askıyı kaldırabilmeleri gerekir.
//
// DB okunamazsa false döner: askı EK bir kısıttır, sahiplik doğrulaması zaten
// yapılmıştır; burada fail-closed davranmak DB hıçkırığında tüm müşterileri
// kilitlerdi.
func domainAskidaMi(r *http.Request, domainID int64) bool {
	if scopeDB == nil {
		return false
	}
	var askida int
	err := scopeDB.QueryRowContext(r.Context(),
		`SELECT COALESCE(askida,0) FROM domains WHERE id=?`, domainID).Scan(&askida)
	return err == nil && askida == 1
}

// MusteriKullanicisininDomainiMi: domain, verilen MÜŞTERİ HESABINA mi ait?
//
// Zincir: users.id -> customers.user_id -> domains.customer_id. Faz 5C'de
// müşteriler users hesabına taşındı; eski FTP kimlikli oturumlar hâlâ tek bir
// DomainID tasiyan MusteriClaims kullanir (bkz. MusteriScopeParam), ama users
// hesabiyla giren bir musterinin BİRDEN ÇOK domaini olabilir — bu yuzden
// kapsam token'dan degil zincirden cozulur.
//
// FAIL-CLOSED: DB okunamazsa false doner.
func MusteriKullanicisininDomainiMi(r *http.Request, userID, domainID int64) bool {
	if scopeDB == nil || userID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(), `
		SELECT COUNT(*)
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE d.id = ? AND c.user_id = ?`, domainID, userID).Scan(&n)
	return err == nil && n > 0
}

// BayiMusterisiMi: musteri kaydi verilen bayiye mi ait? (customers uzerinde
// islem yapan uclar icin — domain zincirine girmeden.)
func BayiMusterisiMi(r *http.Request, bayiUserID, customerID int64) bool {
	if scopeDB == nil || bayiUserID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM customers WHERE id = ? AND owner_user_id = ?`,
		customerID, bayiUserID).Scan(&n)
	return err == nil && n > 0
}

// KapsamSQL: liste uclari icin WHERE parcasi ve argumani uretir.
//
// Liste uclarinda tek tek sahiplik dogrulamak ise yaramaz — sorgunun kendisi
// daraltilmali, yoksa bayi TUM kayitlari goren bir liste alir. Kullanim:
//
//	kosul, arg := middleware.KapsamSQL(r, "d")
//	sorgu := "SELECT ... FROM domains d " + kosul
//
// Admin icin bos string doner (daraltma yok). Bayi icin customers JOIN'i
// gerektiren bir EXISTS kosulu doner. Musteri/kimliksiz icin hicbir satirin
// eslesmedigi bir kosul doner (fail-closed).
func KapsamSQL(r *http.Request, domainAlias string) (string, []any) {
	c := ClaimsFrom(r)
	if c != nil && c.Role == RolAdmin {
		return "", nil
	}
	if c != nil && c.Role == RolBayi {
		return " WHERE EXISTS (SELECT 1 FROM customers kc WHERE kc.id = " +
			domainAlias + ".customer_id AND kc.owner_user_id = ?)", []any{c.UserID}
	}
	if mc := MusteriClaimsFrom(r); mc != nil {
		return " WHERE " + domainAlias + ".id = ?", []any{mc.DomainID}
	}
	return " WHERE 1 = 0", nil
}

// ClaimsContext: verilen context'e admin/bayi claim'lerini yerleştirir.
//
// RequireAuth bunu token doğruladıktan sonra kendisi yapar; bu dışa açık
// biçim, kimlik bağlamını elle kurması gereken yerler içindir (başka
// paketlerin yetki testleri gibi — claimsKey paket-özeldir ve dışarıdan
// erişilemez). Üretim yolunda çağrılmaz.
func ClaimsContext(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func ClaimsFrom(r *http.Request) *auth.Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*auth.Claims)
	return c
}

func MusteriClaimsFrom(r *http.Request) *auth.MusteriClaims {
	v := r.Context().Value(musteriClaimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*auth.MusteriClaims)
	return c
}
