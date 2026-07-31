// Package musteri: müşteri (domain sahibi) panel girişi.
//
// Kimlik yalnızca users tablosundadır (rol=user, bcrypt). Kapsam token'a
// GÖMÜLMEZ; her istekte domains → customers → users zincirinden çözülür
// (bkz. middleware.MusteriKullanicisininDomainiMi), çünkü bir müşterinin
// birden çok domaini olabilir ve sahiplik değiştiğinde eski token anında
// geçersizleşmelidir.
//
// TARİHÇE: Faz 5C'de müşteriler FTP kimliğiyle giriyordu ve göçle üretilen
// hesapların parolası boş olduğu için eski FTP yolu bir "geçiş köprüsü"
// olarak korunmuştu. Köprü 2026-07-27'de kaldırıldı: FTP parolaları
// veritabanında düz metin tutulduğu için (Pure-FTPd MYSQLCrypt cleartext)
// panel girişini düz metin bir parolaya bağlıyordu. Parolası olmayan bir
// müşteri artık giriş yapamaz — yönetici/bayi Kullanıcılar ekranından parola
// atamalıdır.
package musteri

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
)

type Handlers struct {
	DB     *sql.DB
	Secret []byte
}

type loginReq struct {
	Kullanici string `json:"kullanici"`
	Parola    string `json:"parola"`
}

// Login: müşteri panel hesabıyla giriş; başarılıysa rol=user token'ı döner.
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

	var (
		uid   int64
		hash  string
		rol   string
		durum string
		surum uint64
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash, role, status, auth_version FROM users WHERE username=?`,
		req.Kullanici).Scan(&uid, &hash, &rol, &durum, &surum)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Tek ve aynı ret mesajı: hangi kullanıcı adlarının var olduğu sızmasın.
	// Boş password_hash hiçbir parolayla eşleşmez (bkz. auth.ParolaEslesiyorMu),
	// dolayısıyla parolası atanmamış hesap buradan geçemez.
	if err != nil || rol != "user" || !auth.ParolaEslesiyorMu(hash, req.Parola) {
		auth.WriteAudit(h.DB, 0, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı veya parola hatalı")
		return
	}
	if durum != "active" {
		auth.WriteAudit(h.DB, uid, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusForbidden, "hesap askıya alınmış")
		return
	}

	// Buradaki ilk domain yalnız arayüzün açılışta nereye gideceğini bilmesi
	// için — yetki kararı değildir, her istekte zincirden yeniden çözülür.
	var ilkDomainID int64
	var ilkAlanAdi string
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT d.id, d.alan_adi
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE c.user_id = ?
		ORDER BY d.id LIMIT 1`, uid).Scan(&ilkDomainID, &ilkAlanAdi)

	// Hesabı var ama tanımlı hizmeti yok (ör. bayi hesabı açtı, henüz domain
	// atamadı). Token verip arayüzü /abonelikler/0'a göndermek "domain
	// bulunamadı" hatasıyla biterdi; nedeni açıkça söylemek daha doğru.
	if ilkDomainID == 0 {
		auth.WriteAudit(h.DB, uid, req.Kullanici, ip, "musteri.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusForbidden,
			"hesabınıza tanımlı bir hizmet yok — sağlayıcınızla görüşün")
		return
	}

	tok, err := auth.Issue(h.Secret, 24*3600, uid, req.Kullanici, rol, surum)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token üretilemedi")
		return
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
}
