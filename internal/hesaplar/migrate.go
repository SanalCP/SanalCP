package hesaplar

import (
	"context"
	"database/sql"
	"log"

	"sanalcp/internal/secretcrypt"
)

// HealLegacyPlaintextSecrets: bu göç öncesi oluşturulmuş düz-metin
// db_pass_plain (MySQL DB parolası) / ftp_accounts.password_md5 (FTP parolası)
// satırlarını yerinde göçürür. Idempotent — zaten göç edilmiş satırları
// (v1:/$y$ önekli) atlar; panel her açılışta çağırır, göçecek satır yoksa
// no-op'tur (bkz. main.go'daki diğer Heal*/Seed* fonksiyonlarıyla aynı desen).
//
// 🔴 FTP göçü sonrası Pure-FTPd'nin MYSQLCrypt ayarı "cleartext"ten "crypt"e
// alınmadıysa FTP girişleri BAŞARISIZ olur (Pure-FTPd artık hash'i düz metinle
// karşılaştırır) — bu değişiklikle birlikte sanalcp-ftp-setup'ın yeniden
// çalıştırılması gerekir.
func HealLegacyPlaintextSecrets(ctx context.Context, db *sql.DB) {
	if box == nil {
		log.Printf("secret göçü: şifreleme kutusu ayarlanmamış (hesaplar.Init çağrılmadı), atlanıyor")
		return
	}
	healDBPasswords(ctx, db)
	healFTPPasswords(ctx, db)
}

func healDBPasswords(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `SELECT id, db_pass_plain FROM db_accounts`)
	if err != nil {
		log.Printf("secret göçü (db_accounts) okunamadı: %v", err)
		return
	}
	type item struct {
		id  int64
		raw string
	}
	var legacy []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.raw); err != nil {
			continue
		}
		if it.raw != "" && !secretcrypt.IsEncrypted(it.raw) {
			legacy = append(legacy, it)
		}
	}
	rows.Close()
	if len(legacy) == 0 {
		return
	}
	migrated := 0
	for _, it := range legacy {
		enc, err := box.Encrypt(it.raw)
		if err != nil {
			log.Printf("secret göçü (db_accounts id=%d) şifreleme hatası: %v", it.id, err)
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE db_accounts SET db_pass_plain=? WHERE id=?`, enc, it.id); err != nil {
			log.Printf("secret göçü (db_accounts id=%d) yazılamadı: %v", it.id, err)
			continue
		}
		migrated++
	}
	log.Printf("secret göçü: %d/%d db_accounts parolası şifrelendi", migrated, len(legacy))
}

func healFTPPasswords(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `SELECT id, password_md5 FROM ftp_accounts`)
	if err != nil {
		log.Printf("secret göçü (ftp_accounts) okunamadı: %v", err)
		return
	}
	type item struct {
		id  int64
		raw string
	}
	var legacy []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.raw); err != nil {
			continue
		}
		if it.raw != "" && !isYescryptHash(it.raw) {
			legacy = append(legacy, it)
		}
	}
	rows.Close()
	if len(legacy) == 0 {
		return
	}
	migrated := 0
	for _, it := range legacy {
		hash, err := yescryptHashFTP(it.raw)
		if err != nil {
			log.Printf("secret göçü (ftp_accounts id=%d) hash hatası: %v", it.id, err)
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE ftp_accounts SET password_md5=? WHERE id=?`, hash, it.id); err != nil {
			log.Printf("secret göçü (ftp_accounts id=%d) yazılamadı: %v", it.id, err)
			continue
		}
		migrated++
	}
	log.Printf("secret göçü: %d/%d ftp_accounts parolası yescrypt hash'ine çevrildi — "+
		"Pure-FTPd MYSQLCrypt=crypt moduna alınmadıysa (sanalcp-ftp-setup) FTP girişleri bozulur",
		migrated, len(legacy))
}
