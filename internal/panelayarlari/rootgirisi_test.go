package panelayarlari

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Kilitlenme koruması: root girişi kapatılırken sistemde aktif ve root
// OLMAYAN bir admin yoksa istek reddedilmeli. Aksi halde tek adımda kendini
// dışarı kilitlemek mümkün olurdu — root kapanır, girebilecek hesap kalmaz.
func TestRootGirisiKaydet_BaskaAdminYokkenKapatilamaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`role='admin' AND status='active' AND id<>1`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":false}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("beklenen 400, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

func TestRootGirisiKaydet_BaskaAdminVarkenKapatilir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`role='admin' AND status='active' AND id<>1`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE panel_ayarlari SET root_girisi_acik=\?`).
		WithArgs(0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET auth_version=auth_version\+1`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE api_tokenlari SET aktif=0 WHERE user_id=\?`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":false}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	var yanit struct {
		Acik bool `json:"acik"`
	}
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if yanit.Acik {
		t.Fatal("kapatma sonrası açık raporlandı")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// AÇMA yönü admin sayımı gerektirmez — kilitlenme riski yaratmaz. Ayrıca
// açma, oturum/token İPTALİ de yapmaz: sqlmock sıralı eşleştirdiği için
// beklenen akış Begin -> bayrak UPDATE -> Commit'tir; handler araya
// auth_version veya api_tokenlari yazsaydı bu test kırılırdı.
func TestRootGirisiKaydet_AcmaAdminSaymaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE panel_ayarlari SET root_girisi_acik=\?`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":true}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

func TestRootGirisi_DurumOkur(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/root-girisi", nil)
	w := httptest.NewRecorder()

	h.RootGirisi(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	var yanit struct {
		Acik bool `json:"acik"`
	}
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if !yanit.Acik {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}

// ATOMİKLİK: token iptali başarısız olursa bayrak da kapanmamalı. Yarım
// uygulanmış bir kapatma (bayrak kapalı, root token'ları canlı) tam olarak bu
// düzeltmenin engellemek için var olduğu durumdur.
func TestRootGirisiKaydet_TokenIptaliBasarisizsaBayrakDaKapanmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`role='admin' AND status='active' AND id<>1`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE panel_ayarlari SET root_girisi_acik=\?`).
		WithArgs(0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET auth_version=auth_version\+1`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE api_tokenlari SET aktif=0 WHERE user_id=\?`).
		WithArgs(1).
		WillReturnError(errors.New("token tablosu kilitli"))
	mock.ExpectRollback()

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":false}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("beklenen 500, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// GET, geçici DB hatasında "Kapalı" DEMEZ — hata döner. Fail-closed okuyucu
// (panelbayrak.RootGirisiAcik) dört güvenlik kararı için doğrudur; burada
// güvenlik kararı yok, ekrana durum yazılıyor: bayrağı aslında 1 olan bir
// sunucu için "Kapalı" göstermek operatöre yanlış bir "güvendesin" olurdu.
func TestRootGirisi_OkunamazsaHataDoner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnError(errors.New("DB düştü"))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/root-girisi", nil)
	w := httptest.NewRecorder()

	h.RootGirisi(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("beklenen 500, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
