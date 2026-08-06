// Package tenanthesap — bir tenant (sistem kullanıcısı) için panel sahiplik
// zincirini kurar:
//
//	domains.customer_id -> customers.id
//	                       ├─ user_id       -> müşterinin panel giriş hesabı
//	                       └─ owner_user_id -> sahip bayi (NULL = doğrudan admin)
//
// NEDEN AYRI PAKET: bu mantık eskiden yalnız internal/gocis içinde, panel
// açılışında çalışan bir VERİ GÖÇÜ olarak vardı. Pratikte hesap üretmenin tek
// yolu oydu — yeni bir domain eklendiğinde hesap OLUŞMUYOR, ancak bir sonraki
// panel yeniden başlatmasında beliriyordu. Artık domain oluşturma da aynı
// mantığı çağırıyor; "göç" paketini normal akıştan çağırmak yanlış olacağı için
// ortak yer buraya taşındı. gocis yalnızca geriye dönük doldurma için kullanır.
package tenanthesap

import (
	"context"
	"database/sql"
	"strings"
)

// Hazirla: sistemKullanici için users + customers zincirini kurar ve tenant'ın
// SAHİPSİZ (customer_id IS NULL) domainlerini bu müşteriye bağlar.
//
// IDEMPOTENT: var olan kayıtları yeniden kullanır, ikinci çağrıda hiçbir şey
// bozulmaz. Bu yüzden hem açılıştaki toplu doldurmadan hem de tek bir domain
// oluşturulurken güvenle çağrılabilir.
//
// 🔑 PAROLASIZ ÜRETİLİR: users.password_hash boş bırakılır. Boş hash hiçbir
// parolayla eşleşmez (bkz. auth.ParolaEslesiyorMu), yani hesap Kullanıcılar
// ekranından parola atanana kadar GİRİŞ YAPAMAZ. Rastgele parola üretip
// kimseye söylememek yerine bu tercih edildi — "parolası var ama kimse
// bilmiyor" durumundaki hesaplar bırakmaz.
//
// sahipBayiID: domaini oluşturan bayinin users.id'si; admin oluşturduysa nil.
// 🔴 BUNU DOĞRU GEÇMEK ŞART: customers.owner_user_id NULL kalırsa müşteri
// doğrudan admin'e ait olur ve domaini oluşturan bayi KENDİ domainini göremez
// (middleware.BayiDomainiMi sahipliği owner_user_id üzerinden çözer).
func Hazirla(ctx context.Context, db *sql.DB, sistemKullanici, alanAdi string, sahipBayiID *int64) (customerID int64, err error) {
	sistemKullanici = strings.TrimSpace(sistemKullanici)
	if sistemKullanici == "" {
		return 0, nil // tenant'ı olmayan kayıt — yapacak iş yok
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }() // commit başarılıysa no-op

	// users: aynı kullanıcı adı varsa yeniden kullan (önceki çağrı yarıda
	// kalmış ya da göç zaten üretmiş olabilir).
	var userID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, sistemKullanici).Scan(&userID)
	if err == sql.ErrNoRows {
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO users(username, email, password_hash, role, full_name, status)
			VALUES(?, '', '', 'user', ?, 'active')`, sistemKullanici, alanAdi)
		if iErr != nil {
			return 0, iErr
		}
		userID, _ = res.LastInsertId()
	} else if err != nil {
		return 0, err
	}

	// customers: bu panel hesabına bağlı kayıt varsa yeniden kullan.
	err = tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE user_id=?`, userID).Scan(&customerID)
	if err == sql.ErrNoRows {
		var sahip any
		if sahipBayiID != nil && *sahipBayiID > 0 {
			sahip = *sahipBayiID
		}
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO customers(ad, eposta, durum, notlar, user_id, owner_user_id)
			VALUES(?, '', 'aktif', 'domain oluşturulurken otomatik açıldı', ?, ?)`,
			alanAdi, userID, sahip)
		if iErr != nil {
			return 0, iErr
		}
		customerID, _ = res.LastInsertId()
	} else if err != nil {
		return 0, err
	}

	// Tenant'ın sahipsiz domainlerini bu müşteriye bağla. Zaten bir müşteriye
	// bağlı olanlara DOKUNULMAZ — yönetici bilinçli bir atama yapmış olabilir.
	if _, err := tx.ExecContext(ctx, `
		UPDATE domains SET customer_id=?
		WHERE sistem_kullanici=? AND customer_id IS NULL`, customerID, sistemKullanici); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return customerID, nil
}
