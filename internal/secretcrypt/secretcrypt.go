// Package secretcrypt: panelin kendi metadata'sında (db_pass_plain gibi) tuttuğu
// müşteri parolalarını AES-256-GCM ile şifreler. Anahtar DB'nin DIŞINDA
// (PANEL_SECRET_KEY, /etc/sanalcp/env) tutulur — yalnızca DB dump'ı sızarsa
// (yedek çalınması, yanlış izinli mysqldump vb.) parolalar düz metin olarak
// ifşa olmaz. Sunucunun TAMAMI ele geçirilirse (env dosyasına da erişim) bu
// koruma aşılabilir — o senaryoya karşı ayrı bir savunma katmanı değildir.
//
// Yalnızca panelin KENDİ Go koduyla okunan/yazılan alanlar için uygundur.
// Harici bir servisin (örn. Pure-FTPd'nin doğrudan SQL ile okuduğu FTP
// parolası) aynı sütunu şifresiz beklediği durumlarda KULLANILMAZ — bkz.
// internal/hesaplar paketindeki yescrypt tabanlı FTP/SSH parola hash'i.
package secretcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// prefix: şifreli değerlerin biçim sürümü. Göç (migration) kodu bu önekle
// zaten-şifrelenmiş satırları düz-metin eski satırlardan ayırt eder.
const prefix = "v1:"

// ErrUnknownFormat: değer "v1:" önekiyle başlamıyor (henüz göç edilmemiş
// düz-metin bir eski kayıt veya bozuk veri).
var ErrUnknownFormat = errors.New("secretcrypt: bilinmeyen/şifrelenmemiş biçim")

// Box: tek bir AES-256-GCM anahtarına bağlı şifrele/çöz çifti.
type Box struct {
	gcm cipher.AEAD
}

// New: 32 baytlık (AES-256) anahtardan bir Box kurar.
func New(key [32]byte) (*Box, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

// Encrypt: düz metni şifreleyip "v1:" önekli, URL-safe base64 bir dize döner.
func (b *Box) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := b.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(ct), nil
}

// Decrypt: Encrypt'in ürettiği bir dizeyi düz metne çevirir.
func (b *Box) Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, prefix) {
		return "", ErrUnknownFormat
	}
	raw, err := base64.RawURLEncoding.DecodeString(s[len(prefix):])
	if err != nil {
		return "", err
	}
	if len(raw) < b.gcm.NonceSize() {
		return "", errors.New("secretcrypt: veri çok kısa")
	}
	nonce, ct := raw[:b.gcm.NonceSize()], raw[b.gcm.NonceSize():]
	pt, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// IsEncrypted: değer zaten bizim biçimimizde mi (göç taramasında eski
// düz-metin satırları ayıklamak için kullanılır).
func IsEncrypted(s string) bool { return strings.HasPrefix(s, prefix) }
