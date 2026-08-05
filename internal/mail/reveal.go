package mail

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// revealTTL: parola gösterim token'ının geçerlilik süresi. UI, oluşturma/
// sıfırlama isteğinden hemen sonra bu token'la tek bir gösterim çağrısı
// yapar; normal kullanımda saniyeler içinde tüketilir.
const revealTTL = 30 * time.Second

type revealEntry struct {
	mailboxID int64
	parola    string
	expires   time.Time
}

var (
	revealMu    sync.Mutex
	revealStore = map[string]revealEntry{}
)

// revealSakla, yeni üretilen/sıfırlanan mailbox parolasını kısa ömürlü, tek
// kullanımlık bir token arkasında saklar. create/reset yanıtı artık parolayı
// düz metin döndürmez (bkz. ParolaGoster) — böylece response'u gören bir
// proxy/erişim logu veya tarayıcı eklentisi parolayı doğrudan göremez.
func revealSakla(mailboxID int64, parola string) string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	token := hex.EncodeToString(buf)

	revealMu.Lock()
	defer revealMu.Unlock()
	revealGCLocked()
	revealStore[token] = revealEntry{mailboxID: mailboxID, parola: parola, expires: time.Now().Add(revealTTL)}
	return token
}

// revealAl token'ı bir kerelik tüketir: bulunur bulunmaz depodan silinir
// (replay imkansız), süresi geçmişse veya başka bir mailbox'a aitse
// bulunamadı döner.
func revealAl(token string, mailboxID int64) (string, bool) {
	revealMu.Lock()
	defer revealMu.Unlock()
	e, ok := revealStore[token]
	if !ok {
		return "", false
	}
	delete(revealStore, token)
	if time.Now().After(e.expires) || e.mailboxID != mailboxID {
		return "", false
	}
	return e.parola, true
}

// revealGCLocked süresi geçmiş kayıtları temizler; revealMu tutulu iken çağrılmalı.
func revealGCLocked() {
	now := time.Now()
	for k, v := range revealStore {
		if now.After(v.expires) {
			delete(revealStore, k)
		}
	}
}
