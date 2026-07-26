package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Panel hesaplarının parola katmanı.
//
// İKİ AYRI PAROLA DÜNYASI VAR, karıştırılmamalı:
//
//   - root (id=1): parolası /etc/shadow'dadır, panel DB'sinde değil. Doğrulama
//     rootParolaDogrula (yescrypt), değiştirme chpasswd ile yapılır. Bu yol
//     çok kullanıcılı desteğe geçerken HİÇ DEĞİŞTİRİLMEDİ — panelden
//     kilitlenme riskini sıfırda tutmanın tek yolu buydu.
//
//   - bayi / müşteri hesapları: parolaları users.password_hash içinde bcrypt
//     ile saklanır. Bu hesapların sistemde karşılığı olan bir Unix kullanıcısı
//     yoktur; yalnız panel oturumu açarlar.
//
// KullaniciRootMu bu ayrımın tek karar noktasıdır.

// bcryptMaliyet: 12, mevcut root hash'iyle aynı seviye ($2a$12$...).
const bcryptMaliyet = 12

// ParolaEnAzKarakter: yeni parolalar için asgari uzunluk. Mevcut
// ParolaDegistir ucundaki 8 karakter kuralıyla aynı tutuldu.
const ParolaEnAzKarakter = 8

var ErrParolaKisa = errors.New("parola en az 8 karakter olmalı")

// KullaniciRootMu: bu kullanıcı adı sistemin root hesabı mı?
// true ise parola /etc/shadow'dan okunur/yazılır, users tablosundan değil.
func KullaniciRootMu(kullaniciAdi string) bool {
	return strings.EqualFold(strings.TrimSpace(kullaniciAdi), "root")
}

// ParolaHashle: panel hesabı parolasını bcrypt ile hash'ler.
func ParolaHashle(parola string) (string, error) {
	if len(parola) < ParolaEnAzKarakter {
		return "", ErrParolaKisa
	}
	h, err := bcrypt.GenerateFromPassword([]byte(parola), bcryptMaliyet)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// ParolaEslesiyorMu: users.password_hash ile verilen parolayı karşılaştırır.
//
// bcrypt.CompareHashAndPassword zaten sabit zamanlıdır. Boş hash (parolası
// hiç atanmamış hesap) daima false döner — aksi hâlde boş parolayla giriş
// mümkün olurdu.
func ParolaEslesiyorMu(hash, parola string) bool {
	if strings.TrimSpace(hash) == "" || parola == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(parola)) == nil
}
