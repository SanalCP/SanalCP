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
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.durum", strconv.FormatInt(id, 10), true)
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
