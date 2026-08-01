package hesaplar

import (
	"crypto/rand"
	"fmt"

	yescrypt "github.com/openwall/yescrypt-go"
)

// itoa64: crypt/yescrypt'in kullandığı özel base64 alfabesi (standart base64
// DEĞİL). yescrypt-go'nun kendi encode64'ü paket-dışına kapalı olduğundan,
// TAM AYNI algoritma burada yeniden uygulanır (bkz. yescrypt-go/yescrypt_wrapper.go).
const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func encode64(src []byte) []byte {
	dst := make([]byte, 0, (len(src)*8+5)/6)
	for i := 0; i < len(src); {
		value, bits := uint32(0), 0
		for ; bits < 24 && i < len(src); bits += 8 {
			value |= uint32(src[i]) << bits
			i++
		}
		for ; bits > 0; bits -= 6 {
			dst = append(dst, itoa64[value&0x3f])
			value >>= 6
		}
	}
	return dst
}

// yescryptHashFTP: FTP/SSH parolasını yescrypt ($y$) hash'ine çevirir.
//
// Sabit "$y$j9T$" ayarı — N=4096 (2^12), r=32, p=1 — libxcrypt'in AlmaLinux 10
// üzerindeki yescrypt VARSAYILANIYLA aynıdır (bkz. internal/auth/handlers.go'daki
// root parola doğrulaması, aynı formatı zaten okuyor). Bu format hem
// `chpasswd -e` ile /etc/shadow'a doğrudan yazılabilir hem de Pure-FTPd'nin
// `MYSQLCrypt crypt` modu (glibc crypt(3) üzerinden) doğrulayabilir — parola
// hiçbir noktada düz metin olarak diskte kalmaz.
func yescryptHashFTP(parola string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	setting := append([]byte("$y$j9T$"), encode64(salt)...)
	hash, err := yescrypt.Hash([]byte(parola), setting)
	if err != nil {
		return "", fmt.Errorf("yescrypt hash: %w", err)
	}
	return string(hash), nil
}

// isYescryptHash: değer zaten $y$ formatında mı (göç taramasında eski
// düz-metin FTP parolalarını ayıklamak için kullanılır).
func isYescryptHash(s string) bool {
	return len(s) > 4 && s[:4] == "$y$j"
}
