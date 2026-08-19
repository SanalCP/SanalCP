package provisioner

import (
	"context"
	"database/sql"
	"log"

	"sanalcp/internal/osfam"
)

// HealApacheBackendOnStartup: bu sistemde ÇALIŞMAYAN Apache backend'ine ayarlı
// domainleri php-fpm'e indirir ve vhost'larını yeniden yazar.
//
// 🔴 NEDEN GEREKLİ: Apache backend'i yalnız RHEL ailesinde çalışır
// (writeApacheVhost /etc/httpd/conf.d ve `httpd` ikilisine gömülü). API katmanı
// artık desteklenmeyen seçimi reddediyor, ama DB'de hâlâ 'apache' yazan satırlar
// olabilir:
//
//   - kapı UYGULANMADAN önce yapılmış seçimler (Ubuntu 24.04'te canlı görüldü:
//     seçim DB'ye yazıldı, Apache vhost yazımı patladı, nginx 127.0.0.1:10080'e
//     proxy'lemeye devam etti ve SİTE 502 OLARAK KALDI),
//   - bir RHEL sunucusundan alınan yedeğin Debian/Ubuntu'ya geri yüklenmesi.
//
// Bu satırlar kendiliğinden düzelmiyordu: sanalcp-repair kiracı vhost'larını
// yeniden render etmiyor, dolayısıyla site birisi backend'i elle değiştirene
// kadar 502'de kalıyordu. Onarım açılışa bağlandı çünkü güncellemeden sonra
// çalışan tek nokta orası (bkz. HealRoundcubeSMTP'deki aynı ders).
//
// Asla fatal değil: hata yalnız loglanır, panel açılışı engellenmez.
func HealApacheBackendOnStartup(ctx context.Context, db *sql.DB) {
	if db == nil || osfam.ApacheBackendDestekli() {
		return
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, alan_adi, sistem_kullanici, php_surum FROM domains WHERE web_backend='apache'`)
	if err != nil {
		log.Printf("apache backend onarımı: domain listesi okunamadı: %v", err)
		return
	}
	type hedef struct {
		id               int64
		alanAdi, sk, php string
	}
	var hedefler []hedef
	for rows.Next() {
		var h hedef
		if err := rows.Scan(&h.id, &h.alanAdi, &h.sk, &h.php); err != nil {
			continue
		}
		hedefler = append(hedefler, h)
	}
	rows.Close()
	if len(hedefler) == 0 {
		return
	}

	log.Printf("apache backend onarımı: %d domain 'apache' backend'ine ayarlı ama bu sistemde "+
		"desteklenmiyor — php-fpm'e indiriliyor (siteler aksi hâlde 502 verir)", len(hedefler))

	for _, h := range hedefler {
		if _, err := db.ExecContext(ctx,
			`UPDATE domains SET web_backend='php-fpm' WHERE id=?`, h.id); err != nil {
			log.Printf("apache backend onarımı (%s): DB güncellenemedi: %v", h.alanAdi, err)
			continue
		}
		// Vhost'u yeniden yaz: nginx'teki 127.0.0.1:10080 proxy bloğu ancak
		// böyle kalkar. DB güncellendikten SONRA render edilir; renderAndReload
		// backend'i DB'den değil opts'tan okur, o yüzden ApplyVhostForDomain
		// çağrısı taze satırı görsün diye sıra bu.
		soket, _ := PHPSocketFor(h.sk, h.php)
		if err := ApplyVhostForDomain(db, h.id, soket, h.php); err != nil {
			log.Printf("apache backend onarımı (%s): vhost yeniden yazılamadı: %v", h.alanAdi, err)
			continue
		}
		log.Printf("apache backend onarımı (%s): php-fpm'e indirildi, vhost yenilendi", h.alanAdi)
	}
}
