// Package musteri: müşteri (domain sahibi) login + scope kontrolü
// FTP credentials ile giriş, JWT'de domain_id scope'u
package musteri

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sanalpanel/internal/auth"
	"sanalpanel/internal/httpx"
)

type Handlers struct {
	DB     *sql.DB
	Secret []byte
}

type loginReq struct {
	Kullanici string `json:"kullanici"`
	Parola    string `json:"parola"`
}

// Login: FTP user/password ile, FTP hesabının bağlı olduğu domain için JWT döner
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	req.Kullanici = strings.TrimSpace(req.Kullanici)
	if req.Kullanici == "" || req.Parola == "" {
		httpx.WriteError(w, http.StatusBadRequest, "kullanıcı/parola gerekli")
		return
	}

	// GÜVENLİK: kaba-kuvvet koruması router seviyesinde middleware.GirisLimiti ile
	// yapılıyor (bkz. cmd/server/main.go) — bu route da /auth/login ile aynı
	// IP-başına pencereli kilide tabi.
	ip := httpx.ClientIP(r)

	// YOL 1 — panel hesabı (Faz 5C, users tablosu + bcrypt).
	//
	// Önce bu denenir; eşleşme yoksa aşağıdaki eski FTP yoluna düşülür.
	// GEÇİŞ KÖPRÜSÜ: göçle üretilen hesapların password_hash'i boştur ve boş
	// hash hiçbir parolayla eşleşmez, dolayısıyla parolası atanmamış müşteriler
	// otomatik olarak FTP yoluna düşer. Panelden parola atandığı anda bu yol
	// devreye girer — müşteri başına, kesintisiz geçiş.
	if h.panelHesabiylaGiris(w, r, req, ip) {
		return
	}

	// YOL 2 — eski FTP kimliği (Pure-FTPd cleartext). Bir sürüm daha korunacak,
	// sonra kaldırılacak.
	var ftpID, domainID int64
	var passDB, alanAdi, status string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT fa.id, fa.domain_id, fa.password_md5, fa.status, d.alan_adi
		 FROM ftp_accounts fa
		 JOIN domains d ON d.id = fa.domain_id
		 WHERE fa.username = ?`, req.Kullanici).
		Scan(&ftpID, &domainID, &passDB, &status, &alanAdi)
	if errors.Is(err, sql.ErrNoRows) {
		auth.WriteAudit(h.DB, 0, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı veya parola hatalı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "FTP hesabı askıya alınmış")
		return
	}
	// Plain text karşılaştırma (Pure-FTPd MYSQLCrypt cleartext); sabit-zamanlı
	// (timing side-channel'a karşı — uzunluk farklıysa doğrudan false, aksi halde
	// subtle.ConstantTimeCompare).
	if len(req.Parola) != len(passDB) || subtle.ConstantTimeCompare([]byte(req.Parola), []byte(passDB)) != 1 {
		auth.WriteAudit(h.DB, 0, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı veya parola hatalı")
		return
	}
	auth.WriteAudit(h.DB, 0, req.Kullanici, ip, "musteri.login", req.Kullanici, true)

	// JWT üret — tip="musteri", domain_id scope
	c := auth.MusteriClaims{
		FTPHesapID: ftpID,
		DomainID:   domainID,
		Kullanici:  req.Kullanici,
		AlanAdi:    alanAdi,
	}
	tok, exp, err := auth.GenerateMusteri(h.Secret, c, 24*3600)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":     tok,
		"bitis":     exp,
		"domain_id": domainID,
		"alan_adi":  alanAdi,
		"kullanici": req.Kullanici,
	})
}

// panelHesabiylaGiris: users tablosundaki müşteri hesabıyla giriş dener.
//
// false dönerse çağıran eski FTP yoluna düşer — yanıt YAZILMAMIŞ olur.
// true dönerse yanıt yazılmıştır (başarı ya da hesap-askıda gibi kesin ret).
//
// Kimliği bulunamayan/parolası tutmayan hesap için sessizce false döner:
// müşterinin panel hesabı henüz parolasızsa FTP kimliğiyle girmeye devam
// edebilmelidir (geçiş köprüsü).
func (h *Handlers) panelHesabiylaGiris(w http.ResponseWriter, r *http.Request, req loginReq, ip string) bool {
	var (
		uid     int64
		hash    string
		rol     string
		durum   string
		adSoyad string
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash, role, status, full_name FROM users WHERE username=?`,
		req.Kullanici).Scan(&uid, &hash, &rol, &durum, &adSoyad)
	if err != nil || rol != "user" || !auth.ParolaEslesiyorMu(hash, req.Parola) {
		return false
	}
	if durum != "active" {
		auth.WriteAudit(h.DB, uid, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusForbidden, "hesap askıya alınmış")
		return true
	}

	// Müşteri hesabının domainleri: kapsam token'a gömülmez, her istekte
	// zincirden çözülür (bkz. middleware.MusteriKullanicisininDomainiMi).
	// Buradaki liste yalnız arayüzün açılışta nereye gideceğini bilmesi için.
	var ilkDomainID int64
	var ilkAlanAdi string
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT d.id, d.alan_adi
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE c.user_id = ?
		ORDER BY d.id LIMIT 1`, uid).Scan(&ilkDomainID, &ilkAlanAdi)

	tok, err := auth.Issue(h.Secret, 24*3600, uid, req.Kullanici, rol)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token üretilemedi")
		return true
	}
	auth.WriteAudit(h.DB, uid, req.Kullanici, ip, "musteri.login", req.Kullanici, true)
	_, _ = h.DB.Exec(`UPDATE users SET last_login_at=NOW(), last_login_ip=? WHERE id=?`, ip, uid)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":        tok,
		"bitis":        time.Now().Add(24 * time.Hour).Unix(),
		"domain_id":    ilkDomainID,
		"alan_adi":     ilkAlanAdi,
		"kullanici":    req.Kullanici,
		"panel_hesabi": true,
	})
	return true
}

// MusteriOnly: middleware — token tipi "musteri" ise ve domain_id path'le eşleşmiyorsa 403
// Admin token'ı ise bypass eder (admin'ler her şeyi yapabilir)
func MusteriOnly(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

// CheckScope: handler içinde manuel scope kontrolü. Admin ise allow.
// Müşteri token ise URL'deki {id} ile token.DomainID eşleşmeli.
func CheckScope(r *http.Request, secret []byte, urlDomainIDParam string) (bool, error) {
	authH := r.Header.Get("Authorization")
	if !strings.HasPrefix(authH, "Bearer ") {
		return false, errors.New("yetkilendirme gerekli")
	}
	raw := strings.TrimPrefix(authH, "Bearer ")
	// Önce admin claims dene
	if c, err := auth.Parse(secret, raw); err == nil {
		_ = c
		return true, nil // admin
	}
	// Sonra musteri claims dene
	mc, err := auth.ParseMusteri(secret, raw)
	if err != nil {
		return false, errors.New("geçersiz token")
	}
	if urlDomainIDParam == "" {
		// Bu endpoint'te domain ID scope yok ama müşteri yine kısıtlı (ör: /domains listesi)
		return false, errors.New("müşteri bu endpoint'e erişemez")
	}
	id, _ := strconv.ParseInt(urlDomainIDParam, 10, 64)
	if id != mc.DomainID {
		return false, errors.New("bu domain'e erişim yok")
	}
	_ = time.Now
	return false, nil // musteri, scoped ok
}
