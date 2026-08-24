package panelbayrak

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const demoSorgu = `SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`

func TestDemoModuAcik_BayrakBirIseAcik(t *testing.T) {
	OnbellekSifirla()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(1))

	if !DemoModuAcik(context.Background(), db) {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}

func TestDemoModuAcik_BayrakSifirIseKapali(t *testing.T) {
	OnbellekSifirla()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(0))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("bayrak 0 iken açık raporlandı")
	}
}

// FAIL-OPEN (bilinçli, RootGirisiAcik'in TERSİ): DB okunamıyorsa demo modu
// KAPALI sayılır. Aksi (fail-closed) olsaydı, demo_modu_acik=0 olan HER
// normal kurulumda geçici bir DB hatası tüm panel genelinde yazmayı
// kilitlerdi — blast radius'u demo VPS'inin çok ötesine taşırdı.
func TestDemoModuAcik_DBHatasindaKapali(t *testing.T) {
	OnbellekSifirla()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).WillReturnError(errors.New("bağlantı koptu"))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("DB hatasında açık raporlandı (fail-open ihlali)")
	}
}

func TestDemoModuAcik_NilDBKapali(t *testing.T) {
	OnbellekSifirla()
	if DemoModuAcik(context.Background(), (*sql.DB)(nil)) {
		t.Fatal("nil db'de açık raporlandı")
	}
}

func TestDemoModuAcik_SatirYoksaKapali(t *testing.T) {
	OnbellekSifirla()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("satır yokken açık raporlandı")
	}
}

// TestDemoModuAcik_OnbellekTekSorguYapar: TTL penceresi içindeki ikinci
// çağrı DB'ye tekrar gitmemeli — mock tek sorgu bekler, ikinci çağrı
// önbellekten dönmeli.
func TestDemoModuAcik_OnbellekTekSorguYapar(t *testing.T) {
	OnbellekSifirla()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(1))

	birinci := DemoModuAcik(context.Background(), db)
	ikinci := DemoModuAcik(context.Background(), db)

	if !birinci || !ikinci {
		t.Fatalf("beklenmeyen sonuç: birinci=%v ikinci=%v", birinci, ikinci)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ikinci çağrı DB'ye tekrar gitti (önbellek çalışmadı): %v", err)
	}
}
