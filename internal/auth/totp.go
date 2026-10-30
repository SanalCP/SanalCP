package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sanalcp/internal/httpx"
)

// RFC 6238 TOTP (HMAC-SHA1, 6 hane, 30sn period) — harici bağımlılık YOK.

// TOTPGenerateSecret: 160-bit rastgele base32 secret üretir (padding'siz).
// rand.Read hatasında ("", error) döner — hatayı yutup tahmin edilebilir
// (all-zero) bir secret üretmek 2FA'yı komple çökertirdi.
func TOTPGenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

func totpAnahtari(secret string) ([]byte, bool) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	if m := len(s) % 8; m != 0 {
		s += strings.Repeat("=", 8-m)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil || len(key) == 0 {
		return nil, false
	}
	return key, true
}

func hotpAnahtar(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	val := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", val%1000000)
}

// gecerliAdim: kodu ±1 pencerede SABİT ZAMANLI doğrular ve kabul edilen 30sn
// zaman-adımını döndürür. minAdim'dan küçük/eşit adımlar REDDEDİLİR (replay
// koruması: aynı kodun tekrar kullanımını engeller). ok=false ise adım=0.
func gecerliAdim(secret, code string, minAdim int64) (int64, bool) {
	key, ok := totpAnahtari(secret)
	if !ok {
		return 0, false
	}
	return gecerliAnahtarAdimi(key, code, minAdim)
}

func gecerliAnahtarAdimi(key []byte, code string, minAdim int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, false
	}
	t := time.Now().Unix() / 30
	for _, c := range []int64{t - 1, t, t + 1} {
		if c <= minAdim {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotpAnahtar(key, uint64(c))), []byte(code)) == 1 {
			return c, true
		}
	}
	return 0, false
}

// TOTPVerify: geriye-uyumlu, replay-korumasız doğrulama (2FA kapatma için).
func TOTPVerify(secret, code string) bool {
	_, ok := gecerliAdim(secret, code, -1)
	return ok
}

// TOTPVerifyAdim: login için replay-korumalı doğrulama; sonAdim'dan SONRAKİ
// bir adımda eşleşen kodu kabul eder ve o adımı döndürür (çağıran DB'ye
// totp_last_step olarak yazmalı — aynı kod ikinci kez kabul edilmez).
func TOTPVerifyAdim(secret, code string, sonAdim int64) (int64, bool) {
	return gecerliAdim(secret, code, sonAdim)
}

// DogrulaGiris2FA iki giriş ucunun ortak 2FA kapısıdır. izin=false olduğunda
// challenge/hata yanıtı yazılmıştır; çağıran oturum üretmeden dönmelidir.
// Kod ancak koşullu DB yazımı tek satırı güncelledikten sonra kabul edilir.
func DogrulaGiris2FA(w http.ResponseWriter, r *http.Request, db *sql.DB, uid int64, kullanici, kod string) (surum uint64, izin bool) {
	var etkin int
	var secret string
	var sonAdim int64
	if err := db.QueryRowContext(r.Context(),
		`SELECT totp_enabled, totp_secret, totp_last_step, auth_version FROM users WHERE id=?`, uid).
		Scan(&etkin, &secret, &sonAdim, &surum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA durumu doğrulanamadı")
		return 0, false
	}
	if etkin == 0 {
		return surum, true
	}
	key, ok := totpAnahtari(secret)
	if etkin != 1 || sonAdim < 0 || !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA yapılandırması hatalı")
		return 0, false
	}
	if strings.TrimSpace(kod) == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"iki_fa_gerekli": true})
		return 0, false
	}
	adim, ok := gecerliAnahtarAdimi(key, kod, sonAdim)
	if !ok {
		WriteAudit(db, uid, kullanici, httpx.ClientIP(r), "auth.2fa", kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "2FA kodu hatalı veya tekrar kullanıldı")
		return 0, false
	}
	// Aynı adımı okuyan eşzamanlı isteklerden yalnız biri kazanabilir.
	// Secret/sürüm koşulları da arada kapatılan/değiştirilen 2FA'yı korur.
	sonuc, err := db.ExecContext(r.Context(),
		`UPDATE users SET totp_last_step=? WHERE id=? AND totp_enabled=1 AND totp_secret=? AND totp_last_step<? AND auth_version=?`,
		adim, uid, secret, adim, surum)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA kodu kaydedilemedi")
		return 0, false
	}
	adet, err := sonuc.RowsAffected()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA kodu kaydedilemedi")
		return 0, false
	}
	if adet != 1 {
		WriteAudit(db, uid, kullanici, httpx.ClientIP(r), "auth.2fa", kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "2FA kodu hatalı veya tekrar kullanıldı")
		return 0, false
	}
	return surum, true
}

// TOTPURI: authenticator uygulamalarının okuduğu otpauth:// URI'si (QR için).
func TOTPURI(secret, account, issuer string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(issuer), url.PathEscape(account), v.Encode())
}
