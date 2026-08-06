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

	"sanalcp/internal/tenanthesap"
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
// 🔑 KRİTİK — PAROLASIZ ÜRETİLİR: users kayıtlarının password_hash'i BOŞ
// bırakılır. Boş hash hiçbir parolayla eşleşmez (bkz. auth.ParolaEslesiyorMu),
// yani göçle üretilen hesap Kullanıcılar ekranından parola atanana kadar
// GİRİŞ YAPAMAZ. Rastgele parola üretip kimseye söylememek yerine bu tercih
// edildi: sessizce "parolası var ama kimse bilmiyor" hesaplar bırakmaz.
//
// 2026-07-27'ye kadar bunun bir telafisi vardı — müşteri eski FTP kimliğiyle
// girebiliyordu (geçiş köprüsü). Köprü kaldırıldı (bkz. internal/musteri),
// çünkü FTP parolaları veritabanında düz metin duruyor. Dolayısıyla göç
// artık tek başına yeterli değil: yöneticinin parola ataması ŞART.
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
		// Toplu doldurma: sahipsiz tenant'lar doğrudan admin'e bağlanır
		// (sahip bayi bilgisi geçmiş veride yok) — bu yüzden nil.
		if _, err := tenanthesap.Hazirla(ctx, db, t.sk, t.alanAdi, nil); err != nil {
			log.Printf("müşteri hesap göçü: %s atlandı: %v", t.sk, err)
			atlanan++
			continue
		}
		uretilen++
	}
	log.Printf("müşteri hesap göçü: %d tenant taşındı, %d atlandı — üretilen hesaplar PAROLASIZ, "+
		"Kullanıcılar ekranından parola atanmadan giriş yapamazlar", uretilen, atlanan)
}
