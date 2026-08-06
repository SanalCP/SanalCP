package tenanthesap

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 🔴 Asıl regresyon testi: bayinin oluşturduğu domainde customers.owner_user_id
// BAYİYE yazılmalı. NULL kalırsa müşteri doğrudan admin'e ait olur ve domaini
// oluşturan bayi KENDİ domainini göremez (middleware.BayiDomainiMi sahipliği
// owner_user_id üzerinden çözer).
func TestHazirlaSahipBayiyiYazar(t *testing.T) {
	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	bayi := int64(42)
	mock2.ExpectBegin()
	mock2.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM users WHERE username=?`)).
		WithArgs("c_ornek_com").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // boş = ErrNoRows
	mock2.ExpectExec(`INSERT INTO users`).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock2.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM customers WHERE user_id=?`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// owner_user_id = 42 BEKLENİYOR — testin çekirdeği bu.
	mock2.ExpectExec(`INSERT INTO customers`).
		WithArgs("ornek.com", int64(7), bayi).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock2.ExpectExec(`UPDATE domains SET customer_id=\?`).
		WithArgs(int64(9), "c_ornek_com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock2.ExpectCommit()

	cid, err := Hazirla(context.Background(), db2, "c_ornek_com", "ornek.com", &bayi)
	if err != nil {
		t.Fatalf("Hazirla hata verdi: %v", err)
	}
	if cid != 9 {
		t.Errorf("customerID = %d, beklenen 9", cid)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Errorf("beklentiler karşılanmadı (owner_user_id yazılmamış olabilir): %v", err)
	}
}

// Admin oluşturduğunda owner_user_id NULL kalmalı — "doğrudan admin'e ait"
// anlamı bu (bkz. migrations/0048).
func TestHazirlaAdmindeSahipNil(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM users WHERE username=?`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO users`).WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM customers WHERE user_id=?`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO customers`).
		WithArgs("ornek.com", int64(3), nil). // sahip NULL
		WillReturnResult(sqlmock.NewResult(4, 1))
	mock.ExpectExec(`UPDATE domains SET customer_id=\?`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if _, err := Hazirla(context.Background(), db, "c_ornek_com", "ornek.com", nil); err != nil {
		t.Fatalf("Hazirla hata verdi: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("beklentiler karşılanmadı: %v", err)
	}
}

// IDEMPOTENT olmalı: her domain oluşturmada çağrılıyor ve açılıştaki toplu
// doldurma da aynı tenant'ı yeniden görebiliyor. Var olan kayıtlar YENİDEN
// ÜRETİLMEMELİ (ikinci bir users satırı unique kısıtı patlatırdı).
func TestHazirlaMevcutKayitlariYenidenKullanir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM users WHERE username=?`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM customers WHERE user_id=?`)).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(22))
	// INSERT BEKLENMİYOR — ikisi de mevcut.
	mock.ExpectExec(`UPDATE domains SET customer_id=\?`).
		WithArgs(int64(22), "c_ornek_com").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	cid, err := Hazirla(context.Background(), db, "c_ornek_com", "ornek.com", nil)
	if err != nil {
		t.Fatalf("Hazirla hata verdi: %v", err)
	}
	if cid != 22 {
		t.Errorf("mevcut müşteri yeniden kullanılmalıydı, customerID = %d", cid)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("beklentiler karşılanmadı: %v", err)
	}
}

// Boş tenant adı sessizce çıkmalı, transaction bile açılmamalı.
func TestHazirlaBosTenantNoop(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := Hazirla(context.Background(), db, "   ", "ornek.com", nil); err != nil {
		t.Fatalf("boş tenant hata vermemeli: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("boş tenant için DB'ye dokunulmamalıydı: %v", err)
	}
}
