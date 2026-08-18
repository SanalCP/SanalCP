package auth

// Yönetim API'si için kişisel erişim token'ları.
//
// Token, JWT'nin YERİNE geçen ikinci bir kimlik taşıyıcıdır; ama AYRI BİR YETKİ
// SİSTEMİ DEĞİLDİR. Doğrulandığında sahibinin hesabından okunan bilgilerle
// normal bir *Claims üretilir ve istek, oturum açmış o kullanıcıymış gibi
// mevcut yetki katmanından (AdminOnly / BayiVeUstu / MusteriScope / KapsamSQL)
// geçer. İkinci bir izin matrisi olmadığı için "API'de yetki kontrolü unutuldu"
// sınıfı hatalar yapısal olarak mümkün değildir.
//
// Ham token saklanmaz; yalnız SHA-256 özeti tutulur.

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// APITokenOnEk: API token'larını JWT'den ayırt eden ön ek. JWT'ler noktayla
// ayrılmış üç bölümden oluşur; bu ön ek çakışmaz ve sızdırılmış bir anahtarın
// günlüklerde/depolarda ne olduğunun anlaşılmasını da kolaylaştırır.
const APITokenOnEk = "scp_"

// APITokenMi: verilen bearer değeri bir API token'ı mı (JWT değil)?
func APITokenMi(raw string) bool { return strings.HasPrefix(raw, APITokenOnEk) }

// ErrAPITokenGecersiz: token yok, iptal edilmiş, süresi dolmuş ya da sahibi
// aktif değil. Çağıran taraf ayrımı KULLANICIYA yansıtmamalıdır — hangi
// token'ların var olduğu bilgisini sızdırmamak için tek bir yanıt döner.
var ErrAPITokenGecersiz = errors.New("geçersiz API token")

// APITokenUret: yeni bir token üretir, özetini kaydeder ve HAM değeri döner.
// Ham değer yalnız burada elde edilebilir — çağıran onu kullanıcıya bir kez
// gösterip atmalıdır.
func APITokenUret(db *sql.DB, userID int64, ad string, bitisAt any) (ham string, id int64, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", 0, err
	}
	ham = APITokenOnEk + hex.EncodeToString(b)
	// Ön ek + ilk 8 karakter: listede token'ı tanımaya yeter, geri kalanı gizli kalır.
	onek := ham[:len(APITokenOnEk)+8]
	res, err := db.Exec(
		`INSERT INTO api_tokenlari (ad, token_hash, onek, user_id, bitis_at) VALUES (?,?,?,?,?)`,
		ad, APITokenHash(ham), onek, userID, bitisAt)
	if err != nil {
		return "", 0, err
	}
	id, _ = res.LastInsertId()
	return ham, id, nil
}

// apiTokenSorgusu: token doğrulama sorgusu. Sabite çıkarılmıştır ki testler
// güvenlik koşullarının (aktif=1, bitis_at) sessizce düşürülmediğini
// doğrulayabilsin — bu koşullardan biri kaybolursa iptal edilmiş ya da süresi
// dolmuş bir token çalışmaya devam eder.
const apiTokenSorgusu = `
		SELECT t.id, u.id, u.username, u.role, u.status, u.auth_version
		  FROM api_tokenlari t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ?
		   AND t.aktif = 1
		   AND (t.bitis_at IS NULL OR t.bitis_at > NOW())`

// APITokenSahibi: ham token'dan sahibinin kimlik bilgilerini çözer.
//
// FAIL-CLOSED: token bulunamazsa, iptal edilmişse, süresi dolmuşsa veya sahibi
// 'active' değilse hata döner. Rol ve auth_version DB'den TAZE okunur — token'a
// gömülmez; böylece hesabın rolü değiştiğinde ya da oturumları geçersiz
// kılındığında token'ın yetkisi de anında değişir.
func APITokenSahibi(db *sql.DB, ham string) (*Claims, int64, error) {
	if db == nil || !APITokenMi(ham) {
		return nil, 0, ErrAPITokenGecersiz
	}
	var (
		tokenID int64
		uid     int64
		kadi    string
		rol     string
		durum   string
		surum   uint64
	)
	err := db.QueryRow(apiTokenSorgusu, APITokenHash(ham)).
		Scan(&tokenID, &uid, &kadi, &rol, &durum, &surum)
	if err != nil || durum != "active" {
		return nil, 0, ErrAPITokenGecersiz
	}
	return &Claims{UserID: uid, Username: kadi, Role: rol, Version: surum}, tokenID, nil
}

// APITokenKullanimIsle: son kullanım damgasını günceller. Hata yutulur —
// istatistik amaçlıdır, yetki kararını etkilemez.
func APITokenKullanimIsle(db *sql.DB, tokenID int64, ip string) {
	if db == nil || tokenID <= 0 {
		return
	}
	// Throttle: her istekte yazmak yerine 60 saniyede bir (RequireAuth'taki
	// last_activity_at deseniyle aynı gerekçe — yazma baskısı oluşturmasın).
	_, _ = db.Exec(`
		UPDATE api_tokenlari SET son_kullanim_at = NOW(), son_kullanim_ip = ?
		 WHERE id = ? AND (son_kullanim_at IS NULL OR son_kullanim_at < NOW() - INTERVAL 60 SECOND)`,
		ip, tokenID)
}

// APITokenHash: ham token'ın saklanan biçimi.
func APITokenHash(ham string) string {
	sum := sha256.Sum256([]byte(ham))
	return hex.EncodeToString(sum[:])
}
