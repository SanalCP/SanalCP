package islemmerkezi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListeBoskenNullDegilDiziDondurur(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(listeSQL).WillReturnRows(sqlmock.NewRows([]string{"anahtar", "tur", "baslik", "aciklama", "durum", "ilerleme", "mesaj", "yol", "baslangic", "bitis"}))
	w := httptest.NewRecorder()
	(&Handlers{DB: db}).Liste(w, httptest.NewRequest("GET", "/api/v1/islemler", nil))
	if w.Code != 200 {
		t.Fatalf("durum=%d govde=%s", w.Code, w.Body.String())
	}
	var got []Islem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("boş dizi bekleniyordu: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListeOrtakIslemSozlesmesiniDondurur(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := sqlmock.NewRows([]string{"anahtar", "tur", "baslik", "aciklama", "durum", "ilerleme", "mesaj", "yol", "baslangic", "bitis"}).
		AddRow("laravel:7", "laravel", "ornek.com Laravel işi", "deploy · deploy", "calisiyor", 65, "Composer tamamlandı", "/abonelikler/3/laravel", "2026-08-30 10:00:00", "")
	mock.ExpectQuery(listeSQL).WillReturnRows(rows)
	w := httptest.NewRecorder()
	(&Handlers{DB: db}).Liste(w, httptest.NewRequest("GET", "/api/v1/islemler", nil))
	var got []Islem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Durum != "calisiyor" || got[0].Ilerleme != 65 || got[0].Yol != "/abonelikler/3/laravel" {
		t.Fatalf("beklenmeyen yanıt: %#v", got)
	}
}
