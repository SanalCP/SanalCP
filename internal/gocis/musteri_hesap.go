// Package gocis — sürüm yükseltmelerinde bir kez çalışan veri göçleri.
//
// SQL migration'larından farkı: şema değil VERİ üretirler ve Go tarafında
// hesaplama gerektirirler (bcrypt hash'i, gruplama vb.). Hepsi idempotenttir —
// her açılışta çalışır, yapacak iş yoksa sessizce çıkar.
package gocis

import (
	"context"
	"database/sql"
	"log"
)

// MusteriHesapGocu: mevcut domainleri çok kullanıcılı hesap modeline taşır.
//
// Faz 5C. Panel bugüne kadar tek yöneticiliydi; müşteriler yalnız FTP
// kimliğiyle /cp'ye giriyordu ve ortada ne customers kaydı ne de panel hesabı
// vardı. Bu göç, var olan her tenant için eksik halkaları üretir:
//
//	domains.sistem_kullanici  (tenant)
//	   -> customers           (fatura/iletişim kaydı)
//	        -> users          (rol='user', panel giriş hesabı)
//
// TENANT BAZINDA GRUPLANIR, domain bazında değil: bir tenant'ın birden çok
// domaini olabilir (addon/parked) ve cPanel modelinde bunlar TEK hesaba aittir.
// Domain başına hesap üretmek aynı sistem kullanıcısı için birden çok panel
// hesabı yaratırdı.
//
// 🔑 KRİTİK — GEÇİŞ KÖPRÜSÜ: üretilen users kayıtlarının password_hash'i BOŞ
// bırakılır. Boş hash hiçbir parolayla eşleşmez (bkz. auth.ParolaEslesiyorMu),
// yani bu hesaplar HENÜZ giriş yapamaz ve mevcut müşteriler eski FTP kimlikli
// yollarını kullanmaya devam eder — göç kimseyi dışarıda bırakmaz. Yönetici
// ya da bayi panelden parola atadığı anda o müşteri için yeni yol açılır.
// Rastgele parola üretip kimseye söylememek yerine bu tercih edildi: sessizce
// "parolası var ama kimse bilmiyor" hesaplar bırakmaz.
func MusteriHesapGocu(ctx context.Context, db *sql.DB) {
	// Zaten müşteri kaydına bağlanmamış domainleri olan tenant'lar.
	rows, err := db.QueryContext(ctx, `
		SELECT sistem_kullanici, MIN(alan_adi), COUNT(*)
		FROM domains
		WHERE customer_id IS NULL AND sistem_kullanici <> ''
		GROUP BY sistem_kullanici`)
	if err != nil {
		log.Printf("müşteri hesap göçü: tenant listesi okunamadı: %v", err)
		return
	}
	type tenant struct {
		sk      string
		alanAdi string
		domain  int
	}
	var liste []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.sk, &t.alanAdi, &t.domain); err == nil {
			liste = append(liste, t)
		}
	}
	rows.Close()
	if len(liste) == 0 {
		return
	}

	var uretilen, atlanan int
	for _, t := range liste {
		if err := tenantGocur(ctx, db, t.sk, t.alanAdi); err != nil {
			log.Printf("müşteri hesap göçü: %s atlandı: %v", t.sk, err)
			atlanan++
			continue
		}
		uretilen++
	}
	log.Printf("müşteri hesap göçü: %d tenant taşındı, %d atlandı (parola atanana kadar FTP girişi geçerli)",
		uretilen, atlanan)
}

func tenantGocur(ctx context.Context, db *sql.DB, sistemKullanici, alanAdi string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // commit başarılıysa no-op

	// users: aynı kullanıcı adı varsa yeniden kullan (göç yarıda kalmış olabilir).
	var userID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, sistemKullanici).Scan(&userID)
	if err == sql.ErrNoRows {
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO users(username, email, password_hash, role, full_name, status)
			VALUES(?, '', '', 'user', ?, 'active')`, sistemKullanici, alanAdi)
		if iErr != nil {
			return iErr
		}
		userID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	}

	// customers: bu panel hesabına bağlı kayıt varsa yeniden kullan.
	var customerID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE user_id=?`, userID).Scan(&customerID)
	if err == sql.ErrNoRows {
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO customers(ad, eposta, durum, notlar, user_id)
			VALUES(?, '', 'aktif', 'panel göçüyle otomatik oluşturuldu', ?)`, alanAdi, userID)
		if iErr != nil {
			return iErr
		}
		customerID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	}

	// Bu tenant'ın sahipsiz domainlerini müşteriye bağla.
	if _, err := tx.ExecContext(ctx, `
		UPDATE domains SET customer_id=?
		WHERE sistem_kullanici=? AND customer_id IS NULL`, customerID, sistemKullanici); err != nil {
		return err
	}

	return tx.Commit()
}
