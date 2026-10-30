package kota

import (
	"context"
	"database/sql"
	"errors"
)

// MaxMailboxQuotaMB: posta kutusu kotası için kabul edilen üst sınır (MB) —
// 1 TiB. Hem plan editörü (service_plans.mailbox_quota_mb, INT) hem kutu başına
// kota ucu aynı sınırı kullanır; MB→bayt çarpımı int64'te taşmaz ve
// mailboxes.quota_bytes (BIGINT UNSIGNED) içine sığar.
const MaxMailboxQuotaMB = 1 << 20

// PlanMailboxQuotaMB: domain'in KANONİK hizmet planındaki posta kutusu kotasını
// (MB) okur. 0 = sınırsız (ya da plansız domain).
//
// Ek alan adlarının (domains.ana_domain_id) satırında oluşturulma anındaki bir
// plan kopyası durur; plan yöneticiden sonradan değiştirildiğinde o kopya
// bayatlar. Bu yüzden zincir ana hesaba kadar yürünür (bkz. planfeatures.Resolve
// — aynı kural). Bozuk zincir/eksik plan hata döner; çağıran KISITLAMAYI
// sessizce atlamamalı.
func PlanMailboxQuotaMB(ctx context.Context, db *sql.DB, domainID int64) (int64, error) {
	if db == nil || domainID <= 0 {
		return 0, errors.New("plan bilgisi okunamadı")
	}
	var seen [32]int64
	for depth := range seen {
		for _, previous := range seen[:depth] {
			if previous == domainID {
				return 0, errors.New("döngüsel domain hesabı")
			}
		}
		seen[depth] = domainID
		var parent, planID, foundPlan, quotaMB sql.NullInt64
		err := db.QueryRowContext(ctx, `SELECT d.ana_domain_id, d.plan_id, p.id, p.mailbox_quota_mb
			FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id WHERE d.id=?`, domainID).
			Scan(&parent, &planID, &foundPlan, &quotaMB)
		if err != nil {
			return 0, err
		}
		if parent.Valid {
			if parent.Int64 <= 0 {
				return 0, errors.New("geçersiz ana domain")
			}
			domainID = parent.Int64
			continue
		}
		if !planID.Valid {
			return 0, nil
		}
		if !foundPlan.Valid || foundPlan.Int64 != planID.Int64 {
			return 0, errors.New("plan bulunamadı")
		}
		if !quotaMB.Valid || quotaMB.Int64 < 0 {
			return 0, nil
		}
		return quotaMB.Int64, nil
	}
	return 0, errors.New("domain hesabı zinciri çok uzun")
}
