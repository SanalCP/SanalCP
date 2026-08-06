package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// scopeDB: MusteriScope'un domain askıya-alma durumunu kontrol edebilmesi için DB
// handle'ı. main() içinde middleware.Init(db) ile set edilir. nil ise askı kontrolü
// atlanır (mc.DomainID eşleşmesi zaten şarttır; askı yalnızca EK bir kısıttır).
var scopeDB *sql.DB

// Init: middleware paketine DB handle'ı verir (müşteri-scope askı kontrolü için).
func Init(db *sql.DB) { scopeDB = db }

type ctxKey int

const claimsKey ctxKey = 1

// RequireAuth: token'ı doğrular ve claim'leri context'e koyar.
//
// Tek token tipi vardır (auth.Claims); rol ayrımı claim içindeki Role
// alanındadır. Müşteriye özel ikinci bir token tipi (auth.MusteriClaims)
// 2026-07-27'de kaldırıldı — kapsamı token'a gömüyordu, oysa yetki artık
// her istekte sahiplik zincirinden çözülüyor.
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

			c, err := auth.Parse(secret, tokenRaw)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "geçersiz oturum")
				return
			}
			// İmza tek başına yeterli değildir: hesap silinmiş/askıya alınmış,
			// rolü değiştirilmiş veya parola/2FA sonrası oturum sürümü artırılmış
			// olabilir. DB okunamazsa güvenli tarafta kalıp erişimi reddet.
			if scopeDB == nil {
				httpx.WriteError(w, http.StatusServiceUnavailable, "oturum doğrulanamadı")
				return
			}
			var durum, rol string
			var surum uint64
			err = scopeDB.QueryRowContext(r.Context(),
				`SELECT status, role, auth_version FROM users WHERE id=?`, c.UserID).
				Scan(&durum, &rol, &surum)
			if err != nil || durum != "active" || rol != c.Role || surum != c.Version {
				httpx.WriteError(w, http.StatusUnauthorized, "oturum geçersiz veya sona erdirilmiş")
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, c)
			next.ServeHTTP(w, r.WithContext(ctx))
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
					// Müşterinin birden çok domaini olabilir; kapsam token'dan
					// değil sahiplik zincirinden çözülür.
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
			httpx.WriteError(w, http.StatusUnauthorized, "yetkilendirme gerekli")
		})
	}
}

// DomainSahibiMi: verilen domain ID cagirana ait mi? Merkezi sahiplik denetimi.
//   - Admin token   => her zaman true (tum domainlere erisir).
//   - Bayi token    => domain, bayinin yonettigi bir musteriye aitse true.
//   - Musteri token => domain, musterinin hesabina bagli bir musteri kaydina aitse true.
//   - Kimlik yoksa   => false.
//
// MusteriScope middleware'inin handler-ici eslenigi: URL'de {id} domain param'i
// bulunmayan (or. {dbId} gibi turev kaynak) uclarda, kaynagin domain_id'si DB'den
// cozuldukten sonra bu fonksiyonla sahiplik dogrulanir.
func DomainSahibiMi(r *http.Request, domainID int64) bool {
	c := ClaimsFrom(r)
	if c == nil {
		return false
	}
	switch c.Role {
	case RolAdmin:
		return true // admin: tum domainlere erisir
	case RolBayi:
		return BayiDomainiMi(r, c.UserID, domainID)
	case RolMusteri:
		return MusteriKullanicisininDomainiMi(r, c.UserID, domainID)
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
// Zincir: users.id -> customers.user_id -> domains.customer_id. Bir musterinin
// BİRDEN ÇOK domaini olabilir ve sahiplik degisebilir — bu yuzden kapsam
// token'a gomulmez, her istekte zincirden cozulur.
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
// Admin icin bos string doner (daraltma yok). Bayi ve musteri icin customers
// uzerinden EXISTS kosulu doner — ikisi de ayni zinciri kullanir, yalnizca
// eslestikleri sutun farklidir (owner_user_id / user_id). Kimliksiz istek icin
// hicbir satirin eslesmedigi bir kosul doner (fail-closed).
func KapsamSQL(r *http.Request, domainAlias string) (string, []any) {
	c := ClaimsFrom(r)
	if c == nil {
		return " WHERE 1 = 0", nil
	}
	switch c.Role {
	case RolAdmin:
		return "", nil
	case RolBayi:
		return " WHERE EXISTS (SELECT 1 FROM customers kc WHERE kc.id = " +
			domainAlias + ".customer_id AND kc.owner_user_id = ?)", []any{c.UserID}
	case RolMusteri:
		return " WHERE EXISTS (SELECT 1 FROM customers kc WHERE kc.id = " +
			domainAlias + ".customer_id AND kc.user_id = ?)", []any{c.UserID}
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

// ClaimsIle: isteğe kimlik bilgisi iliştirilmiş bir kopyasını döner.
//
// Yalnızca TESTLER için — üretimde claims'i RequireAuth, doğrulanmış JWT'den
// yazar. Bu fonksiyon bir yetki KAPISI DEĞİLDİR ve hiçbir denetimi atlatmaz:
// aynı süreçte çalışan kod zaten kendi context'ini kurabilir. Var olma sebebi,
// handler'ların (ör. domains.sahipBayiCoz) rol davranışının kendi paketinden
// test edilebilmesi — claimsKey dışa kapalı olduğu için aksi mümkün değil.
func ClaimsIle(r *http.Request, c *auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, c))
}

func ClaimsFrom(r *http.Request) *auth.Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*auth.Claims)
	return c
}
