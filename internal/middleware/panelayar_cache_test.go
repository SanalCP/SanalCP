package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Test yardımcıları: paket-seviyesi panel_ayarlari önbelleği bütün testlerce
// paylaşıldığı için her test kendi durumunu kurar (doldur/eskit/temizle).

func panelAyarCacheTemizle() {
	panelAyarMu.Lock()
	defer panelAyarMu.Unlock()
	panelAyarDeger = nil
	panelAyarOkunan = time.Time{}
	panelAyarYenileniyor = false
	panelAyarBekle = nil
}

func panelAyarCacheDoldur(o *panelAyarOzet) {
	panelAyarMu.Lock()
	defer panelAyarMu.Unlock()
	panelAyarDeger = o
	panelAyarOkunan = time.Now()
	panelAyarYenileniyor = false
}

// panelAyarCacheEskit: son okumayı geriye alarak TTL'in dolmasını simüle eder.
func panelAyarCacheEskit(d time.Duration) {
	panelAyarMu.Lock()
	defer panelAyarMu.Unlock()
	panelAyarOkunan = time.Now().Add(-d)
}

func panelAyarSatiri(ham, gecici string, aktif int, profil string, istek, burst int, istisna string, oturum int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"erisim", "gecici", "aktif", "profil", "istek", "burst", "istisna", "oturum"}).
		AddRow(ham, gecici, aktif, profil, istek, burst, istisna, oturum)
}

func TestPanelAyarlariOkuOnbellektenServisEder(t *testing.T) {
	mock := mockDB(t)
	t.Cleanup(panelAyarCacheTemizle)
	panelAyarCacheTemizle()

	mock.ExpectQuery("SELECT COALESCE").
		WillReturnRows(panelAyarSatiri("192.0.2.0/24", "", 0, "dengeli", 600, 100, "", 30))

	o1, err := panelAyarlariOku(context.Background())
	if err != nil {
		t.Fatalf("ilk okuma: %v", err)
	}
	if o1.ErisimHam != "192.0.2.0/24" || o1.HizProfil != "dengeli" || o1.OturumBosta != 30 || o1.GeciciAktif {
		t.Fatalf("yanlış değer: %+v", o1)
	}

	// TTL içindeki ikinci okuma DB'ye gitmemeli (ek sorgu beklenmiyor).
	o2, err := panelAyarlariOku(context.Background())
	if err != nil {
		t.Fatalf("önbellekten okuma: %v", err)
	}
	if o2 != o1 {
		t.Fatal("önbellek aynı örneği dönmeli")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelAyarlariOkuIlkDolumHatasi(t *testing.T) {
	mock := mockDB(t)
	t.Cleanup(panelAyarCacheTemizle)
	panelAyarCacheTemizle()

	mock.ExpectQuery("SELECT COALESCE").WillReturnError(sqlmock.ErrCancelled)

	if _, err := panelAyarlariOku(context.Background()); err == nil {
		t.Fatal("ilk dolum hatasında hata dönmeli (fail-closed)")
	}
	// Hata önbelleğe yazılmamalı: bir sonraki okuma yeniden denemeli.
	mock.ExpectQuery("SELECT COALESCE").
		WillReturnRows(panelAyarSatiri("", "", 0, "kapali", 0, 0, "", 0))
	if _, err := panelAyarlariOku(context.Background()); err != nil {
		t.Fatalf("yeniden deneme başarılı olmalıydı: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelAyarlariOkuBayatSunum(t *testing.T) {
	mock := mockDB(t)
	t.Cleanup(panelAyarCacheTemizle)
	panelAyarCacheTemizle()
	panelAyarCacheDoldur(&panelAyarOzet{ErisimHam: "192.0.2.0/24", HizProfil: "dengeli", OturumBosta: 30})

	// TTL dolsun; yenileme hatası verirse bayat değer sunulmalı.
	panelAyarCacheEskit(panelAyarTTL + time.Second)
	mock.ExpectQuery("SELECT COALESCE").WillReturnError(sqlmock.ErrCancelled)

	o, err := panelAyarlariOku(context.Background())
	if err != nil {
		t.Fatalf("bayat değer sunulmalıydı: %v", err)
	}
	if o.ErisimHam != "192.0.2.0/24" {
		t.Fatalf("bayat değer yanlış: %+v", o)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelAyarlariOkuCokBayatDegeriReddeder(t *testing.T) {
	mock := mockDB(t)
	t.Cleanup(panelAyarCacheTemizle)
	panelAyarCacheTemizle()
	panelAyarCacheDoldur(&panelAyarOzet{ErisimHam: "192.0.2.0/24"})
	panelAyarCacheEskit(panelAyarMaxBayat + time.Second)
	mock.ExpectQuery("SELECT COALESCE").WillReturnError(sqlmock.ErrCancelled)

	if _, err := panelAyarlariOku(context.Background()); err == nil {
		t.Fatal("çok bayat değer yenileme hatasında sunulmamalı")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelAyarlariOkuScopeDBYok(t *testing.T) {
	eski := scopeDB
	scopeDB = nil
	defer func() { scopeDB = eski }()

	if _, err := panelAyarlariOku(context.Background()); !errors.Is(err, errPanelAyarDBYok) {
		t.Fatalf("scopeDB yokken hata dönmeli, %v", err)
	}
}

func TestPanelAyarlariOkuIlkDolumdaTekSorgu(t *testing.T) {
	mock := mockDB(t)
	t.Cleanup(panelAyarCacheTemizle)
	panelAyarCacheTemizle()

	mock.ExpectQuery("SELECT COALESCE").
		WillDelayFor(30 * time.Millisecond).
		WillReturnRows(panelAyarSatiri("", "", 0, "dengeli", 600, 100, "", 30))

	const istek = 12
	var wg sync.WaitGroup
	hatalar := make(chan error, istek)
	wg.Add(istek)
	for range istek {
		go func() {
			defer wg.Done()
			_, err := panelAyarlariOku(context.Background())
			hatalar <- err
		}()
	}
	wg.Wait()
	close(hatalar)
	for err := range hatalar {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
