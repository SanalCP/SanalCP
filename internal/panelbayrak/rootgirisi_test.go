package panelbayrak

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const sorgu = `SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`

func TestRootGirisiAcik_BayrakBirIseAcik(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))

	if !RootGirisiAcik(context.Background(), db) {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}

func TestRootGirisiAcik_BayrakSifirIseKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("bayrak 0 iken açık raporlandı")
	}
}

// FAIL-CLOSED: DB okunamıyorsa root girişi REDDEDİLİR. Fail-open olsaydı,
// DB'yi bozabilen bir saldırgan root giriş yolunu kendiliğinden açardı.
func TestRootGirisiAcik_DBHatasindaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).WillReturnError(errors.New("bağlantı koptu"))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("DB hatasında açık raporlandı (fail-open)")
	}
}

// panel_ayarlari satırı hiç yoksa (bozuk/yarım kurulum) da kapalı.
func TestRootGirisiAcik_SatirYoksaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("satır yokken açık raporlandı")
	}
}
