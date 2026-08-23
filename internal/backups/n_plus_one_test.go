package backups

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// N+1 düzeltmesi: Temizle artık domain başına ayrı SELECT/DELETE atmak yerine
// tumBilinenKayitlar ile TEK SELECT...IN(...), topluSil ile TEK DELETE...IN(...)
// çalıştırıyor. Bu testler o iki fonksiyonun sözleşmesini doğrular.
func TestTumBilinenKayitlarTekSorguIleGruplar(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"domain_id", "id", "tip", "dosya", "uzak_durum", "boyut_b", "UNIX_TIMESTAMP(created_at)"}).
		AddRow(int64(7), int64(101), "oto", "c_a-auto-20260812-030000.tar.gz", "yüklendi", int64(1000), int64(1755000000)).
		AddRow(int64(9), int64(202), "manuel", "c_b-20260812-131500.tar.gz", "", int64(2000), int64(1755001000))

	mock.ExpectQuery(`SELECT domain_id, id, tip, dosya, uzak_durum, boyut_b, UNIX_TIMESTAMP\(created_at\)\s+FROM backups WHERE domain_id IN \(\?,\?\)`).
		WithArgs(int64(7), int64(9)).
		WillReturnRows(rows)

	out, err := tumBilinenKayitlar(context.Background(), db, []int64{7, 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("2 domain bekleniyordu, %d geldi", len(out))
	}
	if k, ok := out[7]["c_a-auto-20260812-030000.tar.gz"]; !ok || k.ID != 101 {
		t.Errorf("domain 7 kaydı yanlış: %+v", out[7])
	}
	if k, ok := out[9]["c_b-20260812-131500.tar.gz"]; !ok || k.ID != 202 {
		t.Errorf("domain 9 kaydı yanlış: %+v", out[9])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("beklenmeyen sorgu deseni: %v", err)
	}
}

func TestTumBilinenKayitlarBosListeSorguAtmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	out, err := tumBilinenKayitlar(context.Background(), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("boş domain listesinde boş map bekleniyordu, %v geldi", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("boş listede hiç sorgu beklenmiyordu: %v", err)
	}
}

func TestTopluSilTekDeleteIleSiler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM backups WHERE id IN \(\?,\?,\?\)`).
		WithArgs(int64(101), int64(202), int64(303)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	if err := topluSil(context.Background(), db, []int64{101, 202, 303}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("beklenmeyen sorgu deseni: %v", err)
	}
}

func TestTopluSilBosListeSorguAtmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := topluSil(context.Background(), db, nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("boş listede hiç sorgu beklenmiyordu: %v", err)
	}
}

// temizlikAdaylari artık DB'ye dokunmuyor (saf fonksiyon, bilinen kayıtlar dışarıdan
// gelir) — mod=gun ayrımını ve dosya boyutunu bilinen map'ten doğru okuduğunu doğrular.
func TestTemizlikAdaylariBilinenMapKullanir(t *testing.T) {
	esik, err := time.Parse(time.RFC3339, "2026-08-10T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	bilinen := map[string]dbKayit{
		"c_yok-auto-20260801-030000.tar.gz": {ID: 55, Tip: TipOto, BoyutB: 4096, Olusturma: esik.AddDate(0, 0, -5).Unix()},
	}
	adaylar, err := temizlikAdaylari(7, "c_yok", ModOto, esik, bilinen)
	if err != nil {
		t.Fatal(err)
	}
	if len(adaylar) != 1 || adaylar[0].BackupID != 55 || adaylar[0].BoyutB != 4096 {
		t.Errorf("beklenmeyen aday listesi: %+v", adaylar)
	}
}
