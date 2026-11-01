package domains

import (
	"context"
	"database/sql"
	"log"
	"time"

	"sanalcp/internal/provisioner"
)

// SSL güvenlik ağı. Yenilemeyi normalde acme.sh cron'u yapar (bitişe 30 gün
// kala), ama acme.sh deposunda kaydı olmayan cert'ler (sunucu taşıma, reuse
// yolu) hiç yenilenmez ve DB'deki ssl_bitis acme.sh yenilemelerinden habersiz
// kalır. Bu döngü: (1) ssl_bitis'i dosyadaki gerçek tarihle eşitler, (2) acme.sh
// yenilememiş LE cert'lerini panelin kendi akışıyla yeniden alır (o akış acme.sh
// kaydını da oluşturur, sonraki yenilemeler yine acme.sh'te).
const (
	sslYenileAraligi = 12 * time.Hour
	// acme.sh 30 günde yeniler; 20 gün ona öncelik bırakır, LE'nin uyarı
	// e-postalarından ve sitenin düşmesinden önce yine de yetişir.
	sslYenileEsikGun = 20
)

func sslYenilenmeli(kaynak string, bitis, simdi time.Time) bool {
	return kaynak == "letsencrypt" && simdi.Add(sslYenileEsikGun*24*time.Hour).After(bitis)
}

// SSLYenileRun: ilk tur açılıştan 10 dk sonra, ardından 12 saatte bir.
func SSLYenileRun(ctx context.Context, db *sql.DB) {
	t := time.NewTimer(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := sslYenileTur(ctx, db, time.Now()); err != nil && ctx.Err() == nil {
			log.Printf("ssl-yenile: %v", err)
		}
		t.Reset(sslYenileAraligi)
	}
}

type sslHedef struct {
	id                                int64
	alanAdi, sk, php, backend, kaynak string
	certYol, keyYol, dbBitis          string
}

func sslYenileTur(ctx context.Context, db *sql.DB, simdi time.Time) error {
	rows, err := db.QueryContext(ctx, `SELECT id, alan_adi, sistem_kullanici, php_surum, COALESCE(web_backend,'php-fpm'),
		ssl_kaynak, cert_path, key_path, COALESCE(DATE_FORMAT(ssl_bitis,'%Y-%m-%d'),'')
		FROM domains WHERE ssl_aktif=1 AND askida=0 AND is_demo=0 AND cert_path<>'' ORDER BY id`)
	if err != nil {
		return err
	}
	var hedefler []sslHedef
	for rows.Next() {
		var h sslHedef
		if err := rows.Scan(&h.id, &h.alanAdi, &h.sk, &h.php, &h.backend, &h.kaynak, &h.certYol, &h.keyYol, &h.dbBitis); err != nil {
			rows.Close()
			return err
		}
		hedefler = append(hedefler, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, h := range hedefler {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		bitis, ok := provisioner.CertBitis(h.certYol, h.keyYol)
		if !ok {
			continue // dosya yok/bozuk: startup-heal'in işi; burada tahminle yazma
		}
		kaynak, certYol, keyYol := h.kaynak, h.certYol, h.keyYol
		if sslYenilenmeli(h.kaynak, bitis, simdi) {
			cp, kp, gercek, not, e := provisioner.EnableLetsEncrypt(ctx, h.alanAdi, h.sk, h.php, h.backend)
			if e != nil {
				log.Printf("ssl-yenile: %s: %v", h.alanAdi, e)
				continue
			}
			log.Printf("ssl-yenile: %s yenilendi (gercek=%v) %s", h.alanAdi, gercek, not)
			certYol, keyYol = cp, kp
			if !gercek {
				kaynak = "self-signed" // handler ile aynı kural: DB fiilen kurulanı söyler
			}
			if b, ok := provisioner.CertBitis(cp, kp); ok {
				bitis = b
			}
		}
		if bitis.Format("2006-01-02") == h.dbBitis && kaynak == h.kaynak && certYol == h.certYol && keyYol == h.keyYol {
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE domains SET ssl_kaynak=?, cert_path=?, key_path=?, ssl_bitis=? WHERE id=?`,
			kaynak, certYol, keyYol, bitis, h.id); err != nil {
			return err
		}
	}
	return nil
}
