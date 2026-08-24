package panelbayrak

import (
	"context"
	"database/sql"
)

// DemoModuAcik — panel demo modunda mı? (bkz. migrations/0070)
//
// FAIL-OPEN (RootGirisiAcik'in TERSİ yönde): db nil ise ya da sorgu
// başarısız olursa demo modu KAPALI (normal davranış) kabul edilir. Bu
// bayrak varsayılan 0'dır ve dünyadaki neredeyse tüm kurulumlarda hiç
// açılmaz; fail-closed olsaydı (demo=AÇIK varsayımı) geçici bir DB
// hatası, hiç demo modunda olmayan bir panelde TÜM yazma isteklerini
// 403'e çevirirdi. Yalnız demo_modu_acik'ı bilinçli açan tek kurulumda
// (demo.sanalcp.com) bu fail-open, DB blip'i sırasında yazmanın kısa
// süreliğine açık kalması riskini taşır — o sunucuda gerçek müşteri
// verisi olmadığı için kabul edilebilir.
func DemoModuAcik(ctx context.Context, db *sql.DB) bool {
	if db == nil {
		return false
	}
	var acik int
	if err := db.QueryRowContext(ctx,
		`SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`).Scan(&acik); err != nil {
		return false
	}
	return acik == 1
}
