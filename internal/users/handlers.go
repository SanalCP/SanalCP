package users

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sanalpanel/internal/auth"
	"sanalpanel/internal/httpx"
	"sanalpanel/internal/kota"
	"sanalpanel/internal/middleware"
)

type Handlers struct {
	DB *sql.DB
}

type meResp struct {
	ID      int64  `json:"id"`
	Adi     string `json:"adi"`
	Rol     string `json:"rol"`
	Eposta  string `json:"eposta"`
	AdSoyad string `json:"ad_soyad"`
	Durum   string `json:"durum"`
	TwoFA   bool   `json:"iki_fa"`
	Tema    string `json:"tercih_tema"`
	Dil     string `json:"tercih_dil"`
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	// Müşteri (FTP) oturumu — DB lookup'a gerek yok, claim'den synthetic döner.
	if mc := middleware.MusteriClaimsFrom(r); mc != nil {
		httpx.WriteJSON(w, http.StatusOK, meResp{
			ID:      0,
			Adi:     mc.Kullanici,
			Rol:     "musteri",
			AdSoyad: mc.AlanAdi,
			Durum:   "active",
		})
		return
	}
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var resp meResp
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, username, role, email, full_name, status, totp_enabled, tercih_tema, tercih_dil FROM users WHERE id=?`,
		c.UserID).Scan(&resp.ID, &resp.Adi, &resp.Rol, &resp.Eposta, &resp.AdSoyad, &resp.Durum, &resp.TwoFA, &resp.Tema, &resp.Dil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---------- Panel hesapları (admin + bayi) ----------
//
// Kapsam kuralları tek yerde toplandı; her handler bunları yeniden yorumlamaz:
//
//   - admin  : tüm hesapları görür/yönetir, bayi ve admin oluşturabilir.
//   - bayi   : YALNIZ kendi altındaki hesapları (users.reseller_id = kendi id)
//              görür/yönetir ve yalnız 'user' rolünde hesap açabilir.
//   - root   : id=1 dokunulmazdır — silinemez, rolü/durumu değiştirilemez.
//              Parolası /etc/shadow'dadır, bu uçlardan sıfırlanamaz.
//
// Kendi hesabını silme ve son admini devre dışı bırakma da engellenir; ikisi
// de panelden kalıcı kilitlenmeye yol açar.

type KullaniciSatir struct {
	ID           int64  `json:"id"`
	KullaniciAdi string `json:"kullanici_adi"`
	Eposta       string `json:"eposta"`
	AdSoyad      string `json:"ad_soyad"`
	Rol          string `json:"rol"`
	Durum        string `json:"durum"`
	BayiID       *int64 `json:"bayi_id"`
	IkiFA        bool   `json:"iki_fa"`
	SonGiris     string `json:"son_giris"`
	SonGirisIP   string `json:"son_giris_ip"`
	Olusturma    string `json:"olusturma"`
}

const rootID = int64(1)

// yetkiliMi: çağıran, hedef kullanıcı üzerinde işlem yapabilir mi?
// Bulunamayan hedef için de false döner (bayi, var olmayan id ile deneme yapıp
// hesap varlığını çıkaramasın).
func (h *Handlers) yetkiliMi(r *http.Request, hedefID int64) bool {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return false
	}
	if c.Role == middleware.RolAdmin {
		return true
	}
	if c.Role != middleware.RolBayi {
		return false
	}
	var bayiID *int64
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT reseller_id FROM users WHERE id=?`, hedefID).Scan(&bayiID); err != nil {
		return false
	}
	return bayiID != nil && *bayiID == c.UserID
}

// Liste: GET /users
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}

	q := `SELECT id, username, email, full_name, role, status, reseller_id, totp_enabled,
	             COALESCE(DATE_FORMAT(last_login_at,'%Y-%m-%d %H:%i'),''), last_login_ip,
	             COALESCE(DATE_FORMAT(created_at,'%Y-%m-%d'),'')
	      FROM users`
	var arg []any
	if c.Role == middleware.RolBayi {
		q += ` WHERE reseller_id = ?`
		arg = append(arg, c.UserID)
	}
	q += ` ORDER BY id`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]KullaniciSatir, 0)
	for rows.Next() {
		var s KullaniciSatir
		var iki int
		if err := rows.Scan(&s.ID, &s.KullaniciAdi, &s.Eposta, &s.AdSoyad, &s.Rol, &s.Durum,
			&s.BayiID, &iki, &s.SonGiris, &s.SonGirisIP, &s.Olusturma); err != nil {
			continue
		}
		s.IkiFA = iki == 1
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type olusturIstek struct {
	KullaniciAdi string `json:"kullanici_adi"`
	Parola       string `json:"parola"`
	Eposta       string `json:"eposta"`
	AdSoyad      string `json:"ad_soyad"`
	Rol          string `json:"rol"`
}

var kullaniciAdiDesen = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

// Olustur: POST /users
func (h *Handlers) Olustur(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b olusturIstek
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	b.KullaniciAdi = strings.ToLower(strings.TrimSpace(b.KullaniciAdi))
	b.Rol = strings.TrimSpace(b.Rol)

	if !kullaniciAdiDesen.MatchString(b.KullaniciAdi) {
		httpx.WriteError(w, http.StatusBadRequest,
			"kullanıcı adı: 3-32 karakter, küçük harfle başlamalı, harf/rakam/_/- içerebilir")
		return
	}
	// "root" panel DB'sinde değil sistemde tanımlıdır; aynı adla ikinci bir
	// hesap açmak giriş akışını belirsizleştirir.
	if auth.KullaniciRootMu(b.KullaniciAdi) {
		httpx.WriteError(w, http.StatusBadRequest, "bu kullanıcı adı ayrılmıştır")
		return
	}

	// Rol yükseltme koruması: bayi yalnız müşteri hesabı açabilir.
	switch c.Role {
	case middleware.RolAdmin:
		if b.Rol != middleware.RolAdmin && b.Rol != middleware.RolBayi && b.Rol != middleware.RolMusteri {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz rol")
			return
		}
	case middleware.RolBayi:
		if b.Rol != middleware.RolMusteri {
			httpx.WriteError(w, http.StatusForbidden, "bayi yalnız müşteri hesabı açabilir")
			return
		}
		// Kota TEK sayaçtan geçer: "müşteri" = customers kaydıdır. Bayinin
		// açtığı her müşteri hesabı aşağıda kendi customers kaydını da ürettiği
		// için burada da aynı limit sorulur; iki ayrı sayaç kullanmak bayinin
		// limiti iki kapıdan (bir kayıt + bir hesap) aşmasına izin verirdi.
		if err := kota.CheckBayiMusteriEklenebilir(r.Context(), h.DB, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
	default:
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return
	}

	hash, err := auth.ParolaHashle(b.Parola)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Bayinin açtığı hesap otomatik olarak kendisine bağlanır; admin'in açtığı
	// hesap sahipsizdir (doğrudan admin'e ait).
	var bayiID any
	if c.Role == middleware.RolBayi {
		bayiID = c.UserID
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO users(username, email, password_hash, role, reseller_id, full_name, status)
		 VALUES(?,?,?,?,?,?, 'active')`,
		b.KullaniciAdi, strings.TrimSpace(b.Eposta), hash, b.Rol, bayiID, strings.TrimSpace(b.AdSoyad))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			httpx.WriteError(w, http.StatusConflict, "bu kullanıcı adı zaten kullanılıyor")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "hesap oluşturulamadı")
		return
	}
	id, _ := res.LastInsertId()

	// Bayinin açtığı müşteri hesabı, sahiplik zincirinin tamamlanması için
	// kendi customers kaydını da alır: domain -> customer -> bayi. Zincir
	// kurulmazsa hesap açılır ama bayi ona domain bağlayamaz (Create,
	// BayiMusterisiMi ile kendi müşterisini arar) ve kota sayacı da şaşar.
	// Faz 5C göçünün ürettiği yapıyla aynı biçim.
	if c.Role == middleware.RolBayi && b.Rol == middleware.RolMusteri {
		ad := strings.TrimSpace(b.AdSoyad)
		if ad == "" {
			ad = b.KullaniciAdi
		}
		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT INTO customers(ad, eposta, durum, notlar, user_id, owner_user_id)
			 VALUES(?,?, 'aktif', 'bayi tarafından oluşturuldu', ?, ?)`,
			ad, strings.TrimSpace(b.Eposta), id, c.UserID); err != nil {
			// Hesap açıldı ama zincir kurulamadı — sessiz bırakmak sonradan
			// "domain bağlayamıyorum" olarak ortaya çıkardı.
			httpx.WriteError(w, http.StatusInternalServerError,
				"hesap açıldı ancak müşteri kaydı oluşturulamadı: "+err.Error())
			return
		}
	}

	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.olustur", b.KullaniciAdi, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

type guncelleIstek struct {
	Eposta  *string `json:"eposta"`
	AdSoyad *string `json:"ad_soyad"`
	Rol     *string `json:"rol"`
}

// Guncelle: PUT /users/{id}
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.yetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return
	}
	var b guncelleIstek
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}

	if b.Rol != nil {
		if id == rootID {
			httpx.WriteError(w, http.StatusForbidden, "root hesabının rolü değiştirilemez")
			return
		}
		if c.Role != middleware.RolAdmin {
			httpx.WriteError(w, http.StatusForbidden, "rol yalnız yönetici tarafından değiştirilebilir")
			return
		}
		if *b.Rol != middleware.RolAdmin && *b.Rol != middleware.RolBayi && *b.Rol != middleware.RolMusteri {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz rol")
			return
		}
		// Son admin rolünden düşürülemez.
		if *b.Rol != middleware.RolAdmin {
			if yalniz, err := h.sonAdminMi(r, id); err != nil || yalniz {
				httpx.WriteError(w, http.StatusForbidden, "sistemdeki son yönetici hesabı değiştirilemez")
				return
			}
		}
	}

	if b.Eposta != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET email=?, updated_at=NOW() WHERE id=?`, strings.TrimSpace(*b.Eposta), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
			return
		}
	}
	if b.AdSoyad != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET full_name=?, updated_at=NOW() WHERE id=?`, strings.TrimSpace(*b.AdSoyad), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
			return
		}
	}
	if b.Rol != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET role=?, updated_at=NOW() WHERE id=?`, *b.Rol, id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
			return
		}
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.guncelle", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sonAdminMi: verilen kullanıcı, sistemdeki tek aktif admin mi?
func (h *Handlers) sonAdminMi(r *http.Request, id int64) (bool, error) {
	var n int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>?`, id).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// ParolaSifirla: POST /users/{id}/parola — yönetici/bayi tarafından sıfırlama
// (mevcut parola sorulmaz; kendi parolanı değiştirmek için /me/parola).
func (h *Handlers) ParolaSifirla(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.yetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden,
			"root parolası sistem parolasıdır; Profil ekranından değiştirilir")
		return
	}
	var b struct {
		Yeni string `json:"yeni"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	hash, err := auth.ParolaHashle(b.Yeni)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET password_hash=?, updated_at=NOW() WHERE id=?`, hash, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "parola sıfırlanamadı")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.parola", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DurumDegistir: POST /users/{id}/durum
func (h *Handlers) DurumDegistir(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.yetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden, "root hesabı askıya alınamaz")
		return
	}
	if id == c.UserID {
		httpx.WriteError(w, http.StatusForbidden, "kendi hesabınızı askıya alamazsınız")
		return
	}
	var b struct {
		Durum string `json:"durum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || (b.Durum != "active" && b.Durum != "suspended") {
		httpx.WriteError(w, http.StatusBadRequest, "durum 'active' veya 'suspended' olmalı")
		return
	}
	if b.Durum == "suspended" {
		if yalniz, err := h.sonAdminMi(r, id); err != nil || yalniz {
			httpx.WriteError(w, http.StatusForbidden, "sistemdeki son yönetici askıya alınamaz")
			return
		}
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET status=?, updated_at=NOW() WHERE id=?`, b.Durum, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "durum değiştirilemedi")
		return
	}

	// Zincirleme askı: bir bayi askıya alınınca altındaki müşteri hesapları da
	// giriş yapamamalı — aksi hâlde bayi kapatılır ama müşterileri panele
	// girmeye devam eder. Geri alma da zincirlemedir.
	//
	// NOT: Yalnız PANEL GİRİŞİ etkilenir. Sitelerin yayını domains.askida ile
	// yönetilir ve buradan değiştirilmez; hizmeti kesmek ayrı ve bilinçli bir
	// karardır.
	var hedefRol string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, id).Scan(&hedefRol)
	var zincir int64
	if hedefRol == middleware.RolBayi {
		res, err := h.DB.ExecContext(r.Context(),
			`UPDATE users SET status=?, updated_at=NOW() WHERE reseller_id=?`, b.Durum, id)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "bağlı hesapların durumu değiştirilemedi")
			return
		}
		zincir, _ = res.RowsAffected()
	}

	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.durum", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "zincirleme": zincir})
}

// ---------- Bayi limitleri (reseller_limits) ----------
//
// 🔴 Bu iki uç YALNIZ ADMIN'e açıktır (bkz. cmd/server/main.go). Bayinin
// kendi kotasını okuması zararsız görünse de yazması yetki yükseltmesidir;
// ikisi de AdminOnly tutulur ki "okuma açık, yazma kapalı" gibi ince bir
// ayrımın yanlış tarafına düşme riski olmasın.

type BayiLimit struct {
	UserID        int64 `json:"user_id"`
	MaxMusteri    int   `json:"max_customer"`
	MaxDomain     int   `json:"max_domain"`
	TanimliMi     bool  `json:"tanimli"`         // reseller_limits satırı var mı
	MevcutMusteri int   `json:"mevcut_customer"` // şu anki kullanım
	MevcutDomain  int   `json:"mevcut_domain"`
}

// LimitGetir: GET /users/{id}/limitler
func (h *Handlers) LimitGetir(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var rol string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, id).Scan(&rol); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "hesap bulunamadı")
		return
	}
	if rol != middleware.RolBayi {
		httpx.WriteError(w, http.StatusBadRequest, "limitler yalnız bayi hesapları için tanımlanır")
		return
	}

	out := BayiLimit{UserID: id}
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT max_customer, max_domain FROM reseller_limits WHERE user_id=?`, id).
		Scan(&out.MaxMusteri, &out.MaxDomain)
	out.TanimliMi = err == nil // satır yoksa sınırsız

	// Kullanım: limitin anlamlı olması için yanında gösterilir.
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM customers WHERE owner_user_id=?`, id).Scan(&out.MevcutMusteri)
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, id).Scan(&out.MevcutDomain)

	httpx.WriteJSON(w, http.StatusOK, out)
}

// LimitKaydet: PUT /users/{id}/limitler
//
// 0 = sınırsız (service_plans'taki kota sözleşmesiyle aynı). Her iki limit de
// 0 verilirse satır silinir — "sınırsız" durumu tek biçimde (satır yok)
// temsil edilsin, iki farklı gösterimi olmasın.
func (h *Handlers) LimitKaydet(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var rol string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, id).Scan(&rol); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "hesap bulunamadı")
		return
	}
	if rol != middleware.RolBayi {
		httpx.WriteError(w, http.StatusBadRequest, "limitler yalnız bayi hesapları için tanımlanır")
		return
	}

	var b struct {
		MaxMusteri int `json:"max_customer"`
		MaxDomain  int `json:"max_domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if b.MaxMusteri < 0 || b.MaxDomain < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "limitler negatif olamaz (0 = sınırsız)")
		return
	}

	if b.MaxMusteri == 0 && b.MaxDomain == 0 {
		if _, err := h.DB.ExecContext(r.Context(),
			`DELETE FROM reseller_limits WHERE user_id=?`, id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "limitler kaldırılamadı")
			return
		}
	} else if _, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO reseller_limits(user_id, max_customer, max_domain)
		VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE max_customer=VALUES(max_customer), max_domain=VALUES(max_domain)`,
		id, b.MaxMusteri, b.MaxDomain); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "limitler kaydedilemedi: "+err.Error())
		return
	}

	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "bayi.limit", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Sil: DELETE /users/{id}
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.yetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden, "root hesabı silinemez")
		return
	}
	if id == c.UserID {
		httpx.WriteError(w, http.StatusForbidden, "kendi hesabınızı silemezsiniz")
		return
	}
	if yalniz, err := h.sonAdminMi(r, id); err != nil || yalniz {
		httpx.WriteError(w, http.StatusForbidden, "sistemdeki son yönetici silinemez")
		return
	}
	// Bayi siliniyorsa altındaki hesaplar sahipsiz kalır (silinmez) — veri
	// kaybını önlemek için bağ koparılır, hesaplar admin'e devredilmiş olur.
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET reseller_id=NULL WHERE reseller_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "bağlı hesaplar devredilemedi")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM users WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.sil", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
