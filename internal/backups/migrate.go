package backups

import (
	"context"
	"database/sql"
	"log"

	"sanalcp/internal/secretcrypt"
)

// HealLegacyPlaintextDestinationPasswords: bu göç öncesi oluşturulmuş
// düz-metin backup_destinations.parola satırlarını yerinde şifreler.
// Idempotent — zaten şifrelenmiş (v1: önekli) satırları atlar; panel her
// açılışta çağırır, göçecek satır yoksa no-op'tur (bkz. hesaplar paketindeki
// HealLegacyPlaintextSecrets ile aynı desen).
func HealLegacyPlaintextDestinationPasswords(ctx context.Context, db *sql.DB) {
	if box == nil {
		log.Printf("backup destination secret göçü: şifreleme kutusu ayarlanmamış (backups.Init çağrılmadı), atlanıyor")
		return
	}
	rows, err := db.QueryContext(ctx, `SELECT id, parola FROM backup_destinations`)
	if err != nil {
		log.Printf("backup destination secret göçü okunamadı: %v", err)
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
			log.Printf("backup destination secret göçü (id=%d) şifreleme hatası: %v", it.id, err)
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE backup_destinations SET parola=? WHERE id=?`, enc, it.id); err != nil {
			log.Printf("backup destination secret göçü (id=%d) yazılamadı: %v", it.id, err)
			continue
		}
		migrated++
	}
	log.Printf("backup destination secret göçü: %d/%d parola şifrelendi", migrated, len(legacy))
}
