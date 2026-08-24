package panelbayrak

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// demoModuOnbellekSuresi: DemoModuAcik'in DB sonucunu ne kadar süre önbellekte
// tutacağı. DemoSaltOkunur her istekte (kimlik doğrulamadan ÖNCE, /healthz ve
// 404'ler dahil) bu fonksiyonu çağırır — önbellek olmadan her anonim istek bir
// DB round-trip'ine dönüşür (bkz. kod incelemesi bulgusu: istek seli = DB sorgu
// seli). 5 saniye, scripts/demo_seed.go'nun bayrağı çalışırken değiştirmesinin
// neredeyse anında fark edilmesi için yeterince kısa.
const demoModuOnbellekSuresi = 5 * time.Second

var (
	demoModuOnbellekKilit sync.Mutex
	demoModuOnbellekDeger bool
	demoModuOnbellekAn    time.Time
	demoModuOnbellekDolu  bool
)

// OnbellekSifirla: yalnız testler için — paket-seviyeli önbelleği temizler.
//
// demomodu_test.go VE internal/middleware/demo_test.go (panelbayrak.DemoModuAcik'i
// scopeDB üzerinden dolaylı çağırır) her testin başında bunu çağırmalı; aksi
// halde process-level önbellek testler arası sızar ve sqlmock'un "her testte
// tam bir sorgu bekleniyor" varsayımını bozar (üçüncü+ test, yeni mock yerine
// önbellekteki eski değeri sessizce kullanır).
func OnbellekSifirla() {
	demoModuOnbellekKilit.Lock()
	defer demoModuOnbellekKilit.Unlock()
	demoModuOnbellekDolu = false
}

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
	demoModuOnbellekKilit.Lock()
	if demoModuOnbellekDolu && time.Since(demoModuOnbellekAn) < demoModuOnbellekSuresi {
		deger := demoModuOnbellekDeger
		demoModuOnbellekKilit.Unlock()
		return deger
	}
	demoModuOnbellekKilit.Unlock()

	acik := demoModuAcikDBdenOku(ctx, db)

	demoModuOnbellekKilit.Lock()
	demoModuOnbellekDeger = acik
	demoModuOnbellekAn = time.Now()
	demoModuOnbellekDolu = true
	demoModuOnbellekKilit.Unlock()

	return acik
}

// demoModuAcikDBdenOku: önbelleksiz gerçek okuma — fail-open semantiği
// (nil db / sorgu hatası / satır yok → false) değişmeden korunur.
func demoModuAcikDBdenOku(ctx context.Context, db *sql.DB) bool {
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
