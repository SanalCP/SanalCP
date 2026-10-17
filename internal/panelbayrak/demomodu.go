package panelbayrak

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

// DemoModuSorgu: DemoModuAcik'in çalıştırdığı sorgu. Testler sqlmock
// eşleşmesi için bunu kullanır — kopyası tutulmasın, sessizce ayrışmasın.
const DemoModuSorgu = `SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`

// demoModuSorguZamanAsimi: bayrak sorgusunun üst sınırı. Sorgu isteğin
// context'inden KOPARILIR (bkz. DemoModuAcik) — bu yüzden kendi sınırı olmalı.
const demoModuSorguZamanAsimi = 2 * time.Second

// DemoModuOnbellekSuresi: DemoModuAcik'in DB sonucunu ne kadar süre önbellekte
// tutacağı. DemoSaltOkunur her istekte (kimlik doğrulamadan ÖNCE, /healthz ve
// 404'ler dahil) bu fonksiyonu çağırır — önbellek olmadan her anonim istek bir
// DB round-trip'ine dönüşür (bkz. kod incelemesi bulgusu: istek seli = DB sorgu
// seli). 5 saniye, scripts/demo_seed.go'nun bayrağı çalışırken değiştirmesinin
// neredeyse anında fark edilmesi için yeterince kısa.
//
// scripts/demo_seed.go bayrağı çevirdikten sonra bu kadar bekler.
const DemoModuOnbellekSuresi = 5 * time.Second

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

// DemoModuAcik — panel demo modunda mı? (bkz. migrations/0092)
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
//
// Önbellek: yalnız BAŞARILI okumalar önbelleğe yazılır. Sorgu hatası
// önbelleği değiştirmez; son bilinen değer (süresi dolmuş olsa bile)
// döner. Aksi halde bir anonim istemcinin bağlantıyı koparıp sorguyu
// iptal ettirmesi "kapalı" sonucunu tüm process için önbelleğe yazar ve
// demo panelinde yazma engelini istediği zaman kaldırabilirdi. Aynı
// nedenle sorgu isteğin iptalinden bağımsız (context.WithoutCancel)
// çalışır. Fail-open yalnız hiç başarılı okuma yokken geçerlidir.
func DemoModuAcik(ctx context.Context, db *sql.DB) bool {
	demoModuOnbellekKilit.Lock()
	if demoModuOnbellekDolu && time.Since(demoModuOnbellekAn) < DemoModuOnbellekSuresi {
		deger := demoModuOnbellekDeger
		demoModuOnbellekKilit.Unlock()
		return deger
	}
	demoModuOnbellekKilit.Unlock()

	acik, err := demoModuAcikDBdenOku(ctx, db)

	demoModuOnbellekKilit.Lock()
	defer demoModuOnbellekKilit.Unlock()
	if err != nil {
		if demoModuOnbellekDolu {
			return demoModuOnbellekDeger
		}
		return false
	}
	demoModuOnbellekDeger = acik
	demoModuOnbellekAn = time.Now()
	demoModuOnbellekDolu = true
	return acik
}

// demoModuAcikDBdenOku: önbelleksiz gerçek okuma. nil db ve sorgu hatası
// err döner (önbelleğe yazılmaz); satır yoksa geçerli bir "kapalı"dır.
func demoModuAcikDBdenOku(ctx context.Context, db *sql.DB) (bool, error) {
	if db == nil {
		return false, errors.New("panel db yok")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), demoModuSorguZamanAsimi)
	defer cancel()
	var acik int
	err := db.QueryRowContext(ctx, DemoModuSorgu).Scan(&acik)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return acik == 1, nil
}
