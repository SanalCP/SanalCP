package hesaplar

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const veritabaniBoyutlariSQL = `SELECT table_schema, COALESCE(SUM(data_length + index_length),0) DIV 1024 FROM information_schema.TABLES GROUP BY table_schema`

func TestVeritabaniBoyutlariRootDBYok(t *testing.T) {
	eski := rootDB
	rootDB = nil
	t.Cleanup(func() { rootDB = eski })

	if _, err := VeritabaniBoyutlari(context.Background()); err == nil {
		t.Fatal("rootDB nil iken hata bekleniyordu")
	}
}

func TestVeritabaniBoyutlariSorgular(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eski := rootDB
	rootDB = db
	t.Cleanup(func() { rootDB = eski })

	mock.ExpectQuery(regexp.QuoteMeta(veritabaniBoyutlariSQL)).WillReturnRows(
		sqlmock.NewRows([]string{"table_schema", "boyut_kb"}).
			AddRow("site_db", int64(123)).
			AddRow("diger_db", int64(456)),
	)
	got, err := VeritabaniBoyutlari(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["site_db"] != 123 || got["diger_db"] != 456 {
		t.Fatalf("beklenmeyen boyutlar: %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVeritabaniBoyutlariRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eski := rootDB
	rootDB = db
	t.Cleanup(func() { rootDB = eski })

	wantErr := errors.New("satır koptu")
	mock.ExpectQuery(regexp.QuoteMeta(veritabaniBoyutlariSQL)).WillReturnRows(
		sqlmock.NewRows([]string{"table_schema", "boyut_kb"}).
			AddRow("site_db", int64(123)).
			RowError(0, wantErr),
	)
	if _, err := VeritabaniBoyutlari(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("rows.Err aktarılmadı: %v", err)
	}
}
