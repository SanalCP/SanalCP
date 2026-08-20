package auth

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

// claims: RequireAuth middleware zaten doğruladı; header'dan tekrar parse ederek
// (auth→middleware import cycle'ından kaçınmak için) UserID'yi alırız.
func (h *Handlers) claims(r *http.Request) *Claims {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	c, err := Parse(h.Secret, raw)
	if err != nil {
		return nil
	}
	return c
}

// PUT /me — profil bilgileri (ad soyad + e-posta + tercihler)
func (h *Handlers) ProfilGuncelle(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b struct {
		AdSoyad    string `json:"ad_soyad"`
		Eposta     string `json:"eposta"`
		TercihTema string `json:"tercih_tema"`
		TercihDil  string `json:"tercih_dil"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	b.AdSoyad = strings.TrimSpace(b.AdSoyad)
	b.Eposta = strings.TrimSpace(b.Eposta)
	if b.Eposta != "" && !strings.Contains(b.Eposta, "@") {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz e-posta adresi")
		return
	}
	tema := "system"
	if b.TercihTema == "light" || b.TercihTema == "dark" || b.TercihTema == "system" {
		tema = b.TercihTema
	}
	dil := "tr"
	if b.TercihDil == "en" {
		dil = "en"
	}
	if _, err := h.DB.Exec(
		`UPDATE users SET full_name=?, email=?, tercih_tema=?, tercih_dil=?, updated_at=NOW() WHERE id=?`,
		b.AdSoyad, b.Eposta, tema, dil, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /me/parola — sunucu root parolasını değiştir (mevcut parola doğrulanır → chpasswd)
func (h *Handlers) ParolaDegistir(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b struct {
		Mevcut string `json:"mevcut"`
		Yeni   string `json:"yeni"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if len(b.Yeni) < ParolaEnAzKarakter {
		httpx.WriteError(w, http.StatusBadRequest, "yeni parola en az 8 karakter olmalı")
		return
	}

	// root'un parolası sistemdedir (/etc/shadow), panel DB'sinde değil —
	// doğrulama shadow'dan, değiştirme chpasswd ile. Bayi hesaplarının
	// sistemde karşılığı yoktur; onlar users.password_hash kullanır.
	if KullaniciRootMu(c.Username) {
		// Panel root girişi KAPALIYKEN panel /etc/shadow'a hiç dokunmaz —
		// giriş kapısıyla (bkz. Login) aynı ilke. Bayrak kapatıldıktan sonra
		// hâlâ elinde root oturumu olan bir istemci bu uçtan sistem
		// parolasını değiştirmeye devam edemesin diye.
		//
		// Burada Login'deki genel mesaj KULLANILMAZ: çağıran zaten kimlik
		// doğrulamış bir root oturumu, yani sızdırılacak bir bilgi yok;
		// ne yapması gerektiğini söyleyen açık bir mesaj daha doğru.
		if !panelbayrak.RootGirisiAcik(r.Context(), h.DB) {
			WriteAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.parola", "root", false)
			httpx.WriteError(w, http.StatusForbidden,
				"panel root girişi kapalı; sunucu root parolası SSH üzerinden 'passwd' ile değiştirilir")
			return
		}
		if !rootParolaDogrulaFn(b.Mevcut) {
			WriteAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.parola", "root", false)
			httpx.WriteError(w, http.StatusUnauthorized, "mevcut parola hatalı")
			return
		}
		cmd := exec.Command("chpasswd")
		cmd.Stdin = strings.NewReader("root:" + b.Yeni)
		if out, err := cmd.CombinedOutput(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "parola değiştirilemedi: "+strings.TrimSpace(string(out)))
			return
		}
		if _, err := h.DB.Exec(`UPDATE users SET auth_version=auth_version+1, updated_at=NOW() WHERE id=?`, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "oturumlar sonlandırılamadı")
			return
		}
		WriteAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.parola", "root", true)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	var mevcutHash string
	if err := h.DB.QueryRow(`SELECT password_hash FROM users WHERE id=?`, c.UserID).Scan(&mevcutHash); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hesap okunamadı")
		return
	}
	if !ParolaEslesiyorMu(mevcutHash, b.Mevcut) {
		WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "auth.parola", c.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "mevcut parola hatalı")
		return
	}
	yeniHash, err := ParolaHashle(b.Yeni)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET password_hash=?, auth_version=auth_version+1, updated_at=NOW() WHERE id=?`, yeniHash, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "parola değiştirilemedi")
		return
	}
	WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "auth.parola", c.Username, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /me/2fa/setup — yeni secret üret (henüz aktifleştirilmez), otpauth URI döndür
func (h *Handlers) TwoFASetup(w http.ResponseWriter, r *http.Request) {
	if h.claims(r) == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	secret, err := TOTPGenerateSecret()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "secret üretilemedi")
		return
	}
	uri := TOTPURI(secret, "root", "SanalCP")
	resp := map[string]any{
		"secret":      secret,
		"otpauth":     uri, // geriye dönük uyum (elle giriş fallback)
		"otpauth_uri": uri,
	}
	// QR PNG data-URI (authenticator ile taransın). Üretilemezse elle giriş fallback kalır.
	if dataURI, err := TOTPQRDataURI(uri); err == nil {
		resp["qr_data_uri"] = dataURI
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// POST /me/2fa/enable — {secret, kod}: kod secret ile doğrulanırsa 2FA açılır
func (h *Handlers) TwoFAEnable(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b struct {
		Secret string `json:"secret"`
		Kod    string `json:"kod"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	b.Secret = strings.TrimSpace(b.Secret)
	if !TOTPVerify(b.Secret, b.Kod) {
		httpx.WriteError(w, http.StatusBadRequest, "kod doğrulanamadı — uygulamadaki 6 haneli kodu girin")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET totp_secret=?, totp_enabled=1, auth_version=auth_version+1 WHERE id=?`, b.Secret, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi")
		return
	}
	WriteAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.2fa.enable", "root", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /me/2fa/disable — {kod}: geçerli kodla 2FA kapatılır
func (h *Handlers) TwoFADisable(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b struct {
		Kod string `json:"kod"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	var secret string
	_ = h.DB.QueryRow(`SELECT totp_secret FROM users WHERE id=?`, c.UserID).Scan(&secret)
	if !TOTPVerify(secret, b.Kod) {
		httpx.WriteError(w, http.StatusBadRequest, "kod doğrulanamadı")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET totp_secret='', totp_enabled=0, auth_version=auth_version+1 WHERE id=?`, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kapatılamadı")
		return
	}
	WriteAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.2fa.disable", "root", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
