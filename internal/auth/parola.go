package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"sanalcp/internal/hesaplar"
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

// ParolaEnAzKarakter / ParolaGucluMu / ParolaGecerli: TEK politika
// hesaplar paketindedir (import döngüsü olmaması için). Buradakiler
// yalnız ad takma adlarıdır — farklı eşik tanımlamayın.
const ParolaEnAzKarakter = hesaplar.ParolaEnAzKarakter

var ErrParolaKisa = errors.New("parola en az 12 karakter olmalı")

// ParolaGecerli: parola tek-satır mı? chpasswd/mysql satır-enjeksiyonunu engeller.
func ParolaGecerli(pw string) bool {
	return hesaplar.ParolaGecerli(pw)
}

// ParolaGucluMu: kullanıcı parolası yeterince güçlü mü?
// >=12 karakter + karışık + tek satır. Ayrıntı: hesaplar.ParolaGucluMu.
func ParolaGucluMu(pw string) (bool, string) {
	return hesaplar.ParolaGucluMu(pw)
}

// KullaniciRootMu: bu kullanıcı adı sistemin root hesabı mı?
// true ise parola /etc/shadow'dan okunur/yazılır, users tablosundan değil.
func KullaniciRootMu(kullaniciAdi string) bool {
	return strings.EqualFold(strings.TrimSpace(kullaniciAdi), "root")
}

// ParolaHashle: panel hesabı parolasını bcrypt ile hash'ler.
// Yalnız uzunluk değil, tam ParolaGucluMu politikasını uygular.
func ParolaHashle(parola string) (string, error) {
	if ok, _ := ParolaGucluMu(parola); !ok {
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
