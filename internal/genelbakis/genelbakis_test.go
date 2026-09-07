package genelbakis

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMailGruplanmisJoinlerleSayimlariTarar(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		q := strings.Join(strings.Fields(strings.ToLower(actual)), " ")
		if strings.Contains(q, "select count(*) from mailboxes") || strings.Contains(q, "select count(*) from mail_aliases") {
			return fmt.Errorf("correlated SELECT bulundu: %s", actual)
		}
		if strings.Count(q, "group by domain_id") != 2 {
			return fmt.Errorf("iki derived-table GROUP BY bekleniyordu: %s", actual)
		}
		if !strings.Contains(q, "left join ( select domain_id, count(*) as kutu_sayisi") ||
			!strings.Contains(q, "left join ( select domain_id, count(*) as alias_sayisi") {
			return fmt.Errorf("gruplanmış JOIN'ler bulunamadı: %s", actual)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("mail overview").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "durum", "kutu_sayisi", "alias_sayisi", "pasif_kutu"},
	).AddRow(int64(7), "ornek.test", "active", 4, 3, 1))

	w := httptest.NewRecorder()
	(&Handlers{DB: db}).Mail(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var got []MailSatir
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := MailSatir{DomainID: 7, AlanAdi: "ornek.test", MailAktif: true, MailDurum: "active", KutuSayisi: 4, AliasSayisi: 3, PasifKutu: 1}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("beklenmeyen çıktı: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
