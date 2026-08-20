package users

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Root girişi KAPALI: sayım root satırını dışlamalı. Dışlamazsa, geriye tek
// gerçek admin kaldığında koruma devreye girmez, o admin silinir ve panele
// kimse giremez.
func TestSonAdminMi_RootKapaliykenRootSayilmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))
	// id<>? İKİ KEZ geçmeli: biri silinen hesap, biri root satırı.
	mock.ExpectQuery(`role='admin' AND status='active' AND id<>\? AND id<>\?`).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

	h := &Handlers{DB: db}
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/users/7", nil)

	yalniz, err := h.sonAdminMi(r, 7)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if !yalniz {
		t.Fatal("root kapalıyken son gerçek admin korunmadı")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// Root girişi AÇIK: mevcut davranış birebir korunmalı — root gerçekten
// kullanılabilir bir kurtarma yolu olduğu için sayıma dahildir.
func TestSonAdminMi_RootAcikkenRootSayilir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))
	mock.ExpectQuery(`role='admin' AND status='active' AND id<>\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))

	h := &Handlers{DB: db}
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/users/7", nil)

	yalniz, err := h.sonAdminMi(r, 7)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if yalniz {
		t.Fatal("root açıkken root sayılmadı, davranış değişti")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
