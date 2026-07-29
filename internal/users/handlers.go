package users

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sanalpanel/internal/auth"
	"sanalpanel/internal/bayipaketleri"
	"sanalpanel/internal/domains"
	"sanalpanel/internal/httpx"
	"sanalpanel/internal/kilit"
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
	// Parolasiz: password_hash boş — hesap var ama GİRİŞ YAPAMAZ. Göçle
	// üretilen müşteri hesapları böyle doğar (bkz. gocis.MusteriHesapGocu);
	// eskiden FTP köprüsü telafi ediyordu, artık etmiyor, bu yüzden
	// yöneticinin bu hesapları listede görebilmesi gerekir.
	Parolasiz bool `json:"parolasiz"`
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
	             COALESCE(DATE_FORMAT(created_at,'%Y-%m-%d'),''),
	             CASE WHEN username = 'root' THEN 0
	                  WHEN COALESCE(password_hash,'') = '' THEN 1 ELSE 0 END
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
		var iki, parolasiz int
		if err := rows.Scan(&s.ID, &s.KullaniciAdi, &s.Eposta, &s.AdSoyad, &s.Rol, &s.Durum,
			&s.BayiID, &iki, &s.SonGiris, &s.SonGirisIP, &s.Olusturma, &parolasiz); err != nil {
			continue
		}
		s.IkiFA = iki == 1
		s.Parolasiz = parolasiz == 1
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
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET role=?, auth_version=auth_version+1, updated_at=NOW() WHERE id=?`, *b.Rol, id); err != nil {
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
		`UPDATE users SET password_hash=?, auth_version=auth_version+1, updated_at=NOW() WHERE id=?`, hash, id); err != nil {
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
		`UPDATE users SET status=?, auth_version=auth_version+1, updated_at=NOW() WHERE id=?`, b.Durum, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "durum değiştirilemedi")
		return
	}

	// Zincirleme askı: bir bayi askıya alınınca hem altındaki müşteri hesaplarının
	// panel girişi hem de o müşterilere ait TÜM domainlerin yayını (vhost + FTP +
	// mail + panel girişi) askıya düşer — aksi hâlde bayi kapatılır ama müşteri
	// siteleri yayında kalmaya devam eder ki bu "askıya alma" beklentisini
	// karşılamaz. Geri alma da zincirlemedir.
	//
	// Kilit: askı zinciri sürerken aynı bayi için uçan bir "yeni hosting oluştur"
	// isteği bitene kadar bekletilir; aksi hâlde tam o anda oluşturulan domain
	// zincirin dışında kalıp askıdan muaf canlı kalabilir.
	var hedefRol string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, id).Scan(&hedefRol)
	var zincir int64
	var hostingEtkilenen, hostingBasarisiz int
	if hedefRol == middleware.RolBayi {
		res, err := h.DB.ExecContext(r.Context(),
			`UPDATE users SET status=?, auth_version=auth_version+1, updated_at=NOW() WHERE reseller_id=?`, b.Durum, id)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "bağlı hesapların durumu değiştirilemedi")
			return
		}
		zincir, _ = res.RowsAffected()

		k := kilit.Bayi(id)
		k.Lock()
		hostingEtkilenen, hostingBasarisiz, err = domains.BayiAskisiUygula(r.Context(), h.DB, id, b.Durum == "suspended")
		k.Unlock()
		if err != nil {
			log.Printf("bayi %d askı zinciri: %v", id, err)
		}
		if hostingBasarisiz > 0 {
			// DB ile vhost/ftp/mail ayrışmış olabilir: sessizce "tamam" deme, operatöre söyle.
			log.Printf("bayi %d askı zinciri: %d hosting uygulanamadı", id, hostingBasarisiz)
		}
	}

	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.durum", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "zincirleme": zincir,
		"hosting_etkilenen": hostingEtkilenen, "hosting_basarisiz": hostingBasarisiz,
	})
}

// ---------- Bayi limitleri (reseller_limits) ----------
//
// 🔴 Bu iki uç YALNIZ ADMIN'e açıktır (bkz. cmd/server/main.go). Bayinin
// kendi kotasını okuması zararsız görünse de yazması yetki yükseltmesidir;
// ikisi de AdminOnly tutulur ki "okuma açık, yazma kapalı" gibi ince bir
// ayrımın yanlış tarafına düşme riski olmasın.

type BayiLimit struct {
	UserID         int64   `json:"user_id"`
	PaketID        *int64  `json:"paket_id"` // hangi bayi paketinden dolduruldu (yalnız köken bilgisi, bkz. migrations/0056)
	PaketAd        string  `json:"paket_ad"`
	MaxMusteri     int     `json:"max_customer"`
	MaxDomain      int     `json:"max_domain"`
	DiskKotaMB     int64   `json:"disk_kota_mb"`
	TrafikKotaMB   int64   `json:"trafik_kota_mb"`
	IzinliPlanlar  []int64 `json:"izinli_planlar"`  // boş/nil = kısıt yok, tüm hizmet planlarını atayabilir
	FazlaSatis     bool    `json:"fazla_satis"`     // true (varsayılan) = taahhüt kontrol edilmez (bkz. migrations/0057)
	TanimliMi      bool    `json:"tanimli"`         // reseller_limits satırı var mı
	MevcutMusteri  int     `json:"mevcut_customer"` // şu anki kullanım
	MevcutDomain   int     `json:"mevcut_domain"`
	MevcutDiskMB   int64   `json:"mevcut_disk_mb"`
	MevcutTrafikMB int64   `json:"mevcut_trafik_mb"`
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

	out := BayiLimit{UserID: id, FazlaSatis: true}
	var paketID sql.NullInt64
	var izinliJSON sql.NullString
	var fazlaSatis sql.NullBool
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT rl.max_customer, rl.max_domain, rl.disk_kota_mb, rl.trafik_kota_mb, rl.reseller_plan_id,
		        COALESCE(rp.ad,''), rl.izinli_planlar, rl.fazla_satis
		 FROM reseller_limits rl LEFT JOIN reseller_plans rp ON rp.id = rl.reseller_plan_id
		 WHERE rl.user_id=?`, id).
		Scan(&out.MaxMusteri, &out.MaxDomain, &out.DiskKotaMB, &out.TrafikKotaMB, &paketID, &out.PaketAd, &izinliJSON, &fazlaSatis)
	out.TanimliMi = err == nil // satır yoksa sınırsız
	if paketID.Valid {
		out.PaketID = &paketID.Int64
	}
	if izinliJSON.Valid && izinliJSON.String != "" {
		_ = json.Unmarshal([]byte(izinliJSON.String), &out.IzinliPlanlar)
	}
	if fazlaSatis.Valid {
		out.FazlaSatis = fazlaSatis.Bool
	}

	// Kullanım: limitin anlamlı olması için yanında gösterilir.
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM customers WHERE owner_user_id=?`, id).Scan(&out.MevcutMusteri)
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, id).Scan(&out.MevcutDomain)
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(d.boyut_kb),0) DIV 1024, COALESCE(SUM(d.trafik_kb),0) DIV 1024
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, id).Scan(&out.MevcutDiskMB, &out.MevcutTrafikMB)

	httpx.WriteJSON(w, http.StatusOK, out)
}

// LimitKaydet: PUT /users/{id}/limitler
//
// 0 = sınırsız (service_plans'taki kota sözleşmesiyle aynı). Sayısal limitlerin
// hepsi 0 VE izinli_planlar boş VE paket seçilmemişse satır tamamen silinir —
// "sınırsız" durumu tek biçimde (satır yok) temsil edilsin, iki farklı
// gösterimi olmasın. paket_id verilirse (bkz. bayipaketleri.Limitleri) sayısal
// alanlar pakettEN anlık-görüntü olarak kopyalanır, gövdedeki elle girilen
// sayılar YOK SAYILIR — paket seçiliyken karışık bir kaynaktan limit
// oluşmasın.
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
		PaketID       int64   `json:"paket_id"`
		MaxMusteri    int     `json:"max_customer"`
		MaxDomain     int     `json:"max_domain"`
		DiskKotaMB    int64   `json:"disk_kota_mb"`
		TrafikKotaMB  int64   `json:"trafik_kota_mb"`
		IzinliPlanlar []int64 `json:"izinli_planlar"`
		FazlaSatis    *bool   `json:"fazla_satis"` // nil = varsayılan (true, mevcut davranış)
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	fazlaSatis := b.FazlaSatis == nil || *b.FazlaSatis

	var paketIDArg any
	if b.PaketID > 0 {
		mm, md, dk, tr, fs, ok := bayipaketleri.Limitleri(r, h.DB, b.PaketID)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bayi paketi bulunamadı")
			return
		}
		b.MaxMusteri, b.MaxDomain, b.DiskKotaMB, b.TrafikKotaMB = mm, md, dk, tr
		fazlaSatis = fs
		paketIDArg = b.PaketID
	}
	if b.MaxMusteri < 0 || b.MaxDomain < 0 || b.DiskKotaMB < 0 || b.TrafikKotaMB < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "limitler negatif olamaz (0 = sınırsız)")
		return
	}

	var izinliArg any
	if len(b.IzinliPlanlar) > 0 {
		bs, err := json.Marshal(b.IzinliPlanlar)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz izinli_planlar")
			return
		}
		izinliArg = string(bs)
	}

	tumuBos := b.MaxMusteri == 0 && b.MaxDomain == 0 && b.DiskKotaMB == 0 && b.TrafikKotaMB == 0 &&
		izinliArg == nil && paketIDArg == nil
	if tumuBos {
		if _, err := h.DB.ExecContext(r.Context(),
			`DELETE FROM reseller_limits WHERE user_id=?`, id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "limitler kaldırılamadı")
			return
		}
	} else if _, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO reseller_limits(user_id, max_customer, max_domain, disk_kota_mb, trafik_kota_mb, reseller_plan_id, izinli_planlar, fazla_satis)
		VALUES(?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE max_customer=VALUES(max_customer), max_domain=VALUES(max_domain),
		                        disk_kota_mb=VALUES(disk_kota_mb), trafik_kota_mb=VALUES(trafik_kota_mb),
		                        reseller_plan_id=VALUES(reseller_plan_id), izinli_planlar=VALUES(izinli_planlar),
		                        fazla_satis=VALUES(fazla_satis)`,
		id, b.MaxMusteri, b.MaxDomain, b.DiskKotaMB, b.TrafikKotaMB, paketIDArg, izinliArg, fazlaSatis); err != nil {
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
	// Bayi siliniyorsa altındaki hesaplar VE müşteri kayıtları sahipsiz kalır
	// (silinmez) — veri kaybını önlemek için bağ koparılır, admin'e devredilmiş
	// olur (migrations/0048: "NULL = doğrudan admin"). users.reseller_id zaten
	// ON DELETE SET NULL FK'lidir (bu UPDATE onunla örtüşür, zararsız); ama
	// customers.owner_user_id'de FK YOK — elle NULL'lanmazsa var olmayan bir
	// bayi id'sine sarkık kalır (reseller_limits ise ON DELETE CASCADE FK'li,
	// kendiliğinden silinir). Üçü de TEK transaction'da: yarısı uygulanıp
	// yarısı uygulanmayan bir silme, bayinin müşterilerini kalıcı sarkık bırakır.
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi: "+err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE users SET reseller_id=NULL WHERE reseller_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "bağlı hesaplar devredilemedi")
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE customers SET owner_user_id=NULL WHERE owner_user_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "bağlı müşteriler devredilemedi")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM users WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi: "+err.Error())
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.sil", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Bayi özet paneli ----------

type BayiOzet struct {
	PaketAd          string `json:"paket_ad"`
	FazlaSatis       bool   `json:"fazla_satis"`
	MusteriAdet      int    `json:"musteri_adet"`
	MusteriLimit     int    `json:"musteri_limit"`
	DomainAdet       int    `json:"domain_adet"`
	DomainLimit      int    `json:"domain_limit"`
	AskidaAdet       int    `json:"askida_adet"`
	DiskKullanimMB   int64  `json:"disk_kullanim_mb"`
	DiskTaahhutMB    int64  `json:"disk_taahhut_mb"`
	DiskLimitMB      int64  `json:"disk_limit_mb"`
	TrafikKullanimMB int64  `json:"trafik_kullanim_mb"`
	TrafikTaahhutMB  int64  `json:"trafik_taahhut_mb"`
	TrafikLimitMB    int64  `json:"trafik_limit_mb"`
	IzinliPlanSayisi int    `json:"izinli_plan_sayisi"` // 0 = kısıt yok, tüm planları kullanabilir
}

// Ozet: GET /bayi/ozet — bayinin kendi panosu (RequireRole ile yalnız 'reseller'
// rolüne açık, bkz. cmd/server/main.go). reseller_limits satırı yoksa (sınırsız
// bayi) limit alanları 0 döner — aynı "0 = sınırsız" sözleşmesi burada da geçerli.
func (h *Handlers) Ozet(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	out := BayiOzet{FazlaSatis: true}

	// Satır yoksa Scan hiçbir hedefe dokunmadan sql.ErrNoRows döner — tüm
	// limitler 0 (sınırsız) ve FazlaSatis varsayılan true kalır.
	var izinliJSON sql.NullString
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(rp.ad,''), rl.fazla_satis, rl.max_customer, rl.max_domain, rl.disk_kota_mb, rl.trafik_kota_mb, rl.izinli_planlar
		 FROM reseller_limits rl LEFT JOIN reseller_plans rp ON rp.id = rl.reseller_plan_id
		 WHERE rl.user_id=?`, c.UserID).
		Scan(&out.PaketAd, &out.FazlaSatis, &out.MusteriLimit, &out.DomainLimit, &out.DiskLimitMB, &out.TrafikLimitMB, &izinliJSON)
	if izinliJSON.Valid && izinliJSON.String != "" {
		var izinli []int64
		if json.Unmarshal([]byte(izinliJSON.String), &izinli) == nil {
			out.IzinliPlanSayisi = len(izinli)
		}
	}

	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM customers WHERE owner_user_id=?`, c.UserID).Scan(&out.MusteriAdet)
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT COUNT(*), COALESCE(SUM(COALESCE(d.askida,0)),0),
		       COALESCE(SUM(d.boyut_kb),0) DIV 1024, COALESCE(SUM(d.trafik_kb),0) DIV 1024
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, c.UserID).
		Scan(&out.DomainAdet, &out.AskidaAdet, &out.DiskKullanimMB, &out.TrafikKullanimMB)
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(COALESCE(p.disk_kota_mb,0)),0), COALESCE(SUM(COALESCE(p.trafik_kota_mb,0)),0)
		FROM domains d JOIN customers c ON c.id = d.customer_id LEFT JOIN service_plans p ON p.id = d.plan_id
		WHERE c.owner_user_id = ?`, c.UserID).
		Scan(&out.DiskTaahhutMB, &out.TrafikTaahhutMB)

	httpx.WriteJSON(w, http.StatusOK, out)
}
