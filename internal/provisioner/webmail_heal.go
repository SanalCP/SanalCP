package provisioner

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
)

// HealWebmailVhosts: mevcut domainlerin vhost'larına `/webmail/` bloğunu işler.
//
// Blok her vhost render'ında hesaplanır (bkz. webmailBloku), ama mevcut
// kurulumlarda vhost dosyaları zaten yazılmış durumdadır ve kendiliğinden
// yenilenmez — bir sonraki SSL yenilemesi veya PHP sürümü değişikliğine kadar
// müşteri kendi alan adından webmail'e erişemezdi.
//
// Yalnız bloğu EKSİK olan vhost'lar yeniden render edilir; normal render yolu
// kullanıldığı için nginx doğrulaması ve rollback korunur (bkz. renderAndReload).
func HealWebmailVhosts(ctx context.Context, db *sql.DB) {
	if !WebmailKurulu() {
		return // Roundcube yok — eklenecek bir şey de yok.
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, sistem_kullanici FROM domains WHERE COALESCE(is_demo,0)=0 ORDER BY id`)
	if err != nil {
		return
	}
	type kayit struct {
		id int64
		sk string
	}
	var liste []kayit
	for rows.Next() {
		var k kayit
		if err := rows.Scan(&k.id, &k.sk); err == nil {
			liste = append(liste, k)
		}
	}
	rows.Close()

	guncellenen := 0
	for _, k := range liste {
		yol := "/etc/nginx/conf.d/dom_" + k.sk + ".conf"
		b, err := os.ReadFile(yol)
		if err != nil {
			continue // vhost yok (askıda/henüz kurulmamış) — dokunma
		}
		if strings.Contains(string(b), "location ^~ /webmail/") {
			continue // zaten var
		}
		if err := RerenderVhost(db, k.id); err != nil {
			log.Printf("webmail vhost onarımı domain=%d: %v", k.id, err)
			continue
		}
		guncellenen++
	}
	if guncellenen > 0 {
		log.Printf("webmail vhost onarımı: %d domainin vhost'una /webmail/ eklendi", guncellenen)
	}
}
