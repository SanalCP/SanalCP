package backups

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Gün sınırı 0 iken hiçbir sorgu atılmamalı: varsayılan "sınır yok"tur ve
// göç sonrası kimsenin yedeği kendiliğinden silinmemelidir.
func TestPruneEskiSifirGunDokunmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := pruneEski(db, 7, "c_ornek", 0); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Süresi dolan otomatik yedekler silinir; sorgu yalnız tip='oto' satırlarını
// ve en yeni otomatik yedeği HARİÇ tutarak seçmelidir.
func TestPruneEskiSuresiDolanlariSiler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, dosya, uzak_durum FROM backups\s+WHERE domain_id=\? AND tip=\?\s+AND created_at < NOW\(\) - INTERVAL \? DAY\s+AND id < \(SELECT MAX\(id\) FROM backups WHERE domain_id=\? AND tip=\?\)`).
		WithArgs(int64(7), TipOto, 3, int64(7), TipOto).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dosya", "uzak_durum"}).
			AddRow(int64(11), "c_ornek-auto-20260901-030000.tar.gz", "").
			AddRow(int64(12), "c_ornek-auto-20260902-030000.tar.gz", ""))
	mock.ExpectExec(`DELETE FROM backups WHERE id=\?`).WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM backups WHERE id=\?`).WithArgs(int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pruneEski(db, 7, "c_ornek", 3); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
