package uyari

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/monitor"
	"sanalcp/internal/secretcrypt"
)

var testSimdi = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

type turKurulum struct {
	db       *sql.DB
	mock     sqlmock.Sqlmock
	g        Gonderici
	metinler []string
	durum    int
}

func yeniTur(t *testing.T) *turKurulum {
	t.Helper()
	b, err := secretcrypt.New([32]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	Init(b)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	k := &turKurulum{db: db, mock: mock, durum: 200}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		k.metinler = append(k.metinler, r.PostForm.Get("text"))
		w.WriteHeader(k.durum)
	}))
	t.Cleanup(func() {
		srv.Close()
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	k.g = Gonderici{TelegramURL: srv.URL, HTTP: srv.Client()}
	return k
}

func (k *turKurulum) ayar(t *testing.T, acik int) {
	enc, err := box.Encrypt("TOK")
	if err != nil {
		t.Fatal(err)
	}
	k.mock.ExpectQuery("FROM panel_ayarlari").WillReturnRows(sqlmock.NewRows(
		[]string{"bildirim_acik", "telegram_token_enc", "telegram_chat_id", "bildirim_eposta", "varsayilan_dil"}).
		AddRow(acik, enc, "42", "", "tr"))
}

// Tek domain: SSL 3 gün sonra bitiyor (hata), yedek doğrulaması yok.
func (k *turKurulum) domain() {
	k.mock.ExpectQuery("FROM domains").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "ssl_aktif", "ssl_kaynak", "ssl_bitis", "yedek"}).
		AddRow(1, "ornek.com", 1, "letsencrypt", testSimdi.AddDate(0, 0, 3).Format("2006-01-02"), ""))
}

func (k *turKurulum) durumlar(satirlar ...[]any) {
	rows := sqlmock.NewRows([]string{"domain_id", "kontrol", "durum", "ardisik_hata"})
	for _, s := range satirlar {
		vals := make([]driver.Value, len(s))
		for i, v := range s {
			vals[i] = v
		}
		rows.AddRow(vals...)
	}
	k.mock.ExpectQuery("FROM uyari_durumlari").WillReturnRows(rows)
}

func probeKod(kod int) func(string) monitor.DomainHealth {
	return func(string) monitor.DomainHealth { return monitor.DomainHealth{Erisilebilir: true, DurumKodu: kod} }
}

func TestIlkGozlemSessiz(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.domain()
	k.durumlar()
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "web", "ok", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "ssl", "hata", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 0 {
		t.Fatalf("ilk gözlemde mesaj gitmemeli: %q", k.metinler)
	}
}

func TestGecisteBildirir(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.domain()
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "ssl", "ok", 0})
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "ssl", "hata", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 1 || !strings.Contains(k.metinler[0], "ornek.com") || !strings.Contains(k.metinler[0], "🔴") {
		t.Fatalf("tek 🔴 mesaj bekleniyordu: %q", k.metinler)
	}
}

func TestKurtarmadaBildirir(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.domain()
	k.durumlar([]any{1, "web", "hata", 3}, []any{1, "ssl", "hata", 0})
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "web", "ok", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 1 || !strings.Contains(k.metinler[0], "🟢") {
		t.Fatalf("🟢 kurtarma mesajı bekleniyordu: %q", k.metinler)
	}
}

func TestGonderimHatasindaDurumYineKaydedilir(t *testing.T) {
	k := yeniTur(t)
	k.durum = 500
	k.ayar(t, 1)
	k.domain()
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "ssl", "ok", 0})
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "ssl", "hata", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatalf("gönderim hatası turu düşürmemeli: %v", err)
	}
}

func TestKapaliykenGondermez(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 0)
	k.domain()
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "ssl", "ok", 0})
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "ssl", "hata", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 0 {
		t.Fatalf("kapalıyken mesaj gitmemeli: %q", k.metinler)
	}
}

func TestUygulanmayanKontrolSilinir(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.mock.ExpectQuery("FROM domains").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "ssl_aktif", "ssl_kaynak", "ssl_bitis", "yedek"}).
		AddRow(1, "ornek.com", 0, "", "", ""))
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "ssl", "hata", 0})
	k.mock.ExpectExec("DELETE FROM uyari_durumlari").WithArgs(1, "ssl").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 0 {
		t.Fatalf("mesaj gitmemeli: %q", k.metinler)
	}
}

func TestDBHatasindaOncekiBildirimlerGider(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	bitis := testSimdi.AddDate(0, 0, 3).Format("2006-01-02")
	k.mock.ExpectQuery("FROM domains").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "ssl_aktif", "ssl_kaynak", "ssl_bitis", "yedek"}).
		AddRow(1, "ornek.com", 1, "letsencrypt", bitis, "").
		AddRow(2, "silindi.com", 1, "letsencrypt", bitis, ""))
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "ssl", "ok", 0}, []any{2, "web", "ok", 0}, []any{2, "ssl", "ok", 0})
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "ssl", "hata", 0, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	// Tur sırasında silinen domain: FK hatası.
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(2, "ssl", "hata", 0, testSimdi).WillReturnError(errors.New("Error 1452"))
	_ = Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi)
	if len(k.metinler) != 1 || !strings.Contains(k.metinler[0], "ornek.com") {
		t.Fatalf("kaydedilmiş geçiş bildirilmeliydi: %q", k.metinler)
	}
}

func TestDogrulamaSurerkenYedekDurumuKorunur(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.mock.ExpectQuery("FROM domains").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "ssl_aktif", "ssl_kaynak", "ssl_bitis", "yedek"}).
		AddRow(1, "ornek.com", 0, "", "", "dogrulaniyor"))
	k.durumlar([]any{1, "web", "ok", 0}, []any{1, "yedek", "ok", 0})
	// yedek satırı silinmemeli: sonuç 'basarisiz' gelince ok→hata geçişi bildirilmeli.
	if err := Tur(context.Background(), k.db, k.g, probeKod(200), testSimdi); err != nil {
		t.Fatal(err)
	}
}

func TestIlkGozlemdeErisilemezSessizHata(t *testing.T) {
	k := yeniTur(t)
	k.ayar(t, 1)
	k.mock.ExpectQuery("FROM domains").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "alan_adi", "ssl_aktif", "ssl_kaynak", "ssl_bitis", "yedek"}).
		AddRow(1, "ornek.com", 0, "", "", ""))
	k.durumlar()
	// Kurulumda zaten erişilemeyen site doğrudan 'hata' kaydedilir; 2. turda ok→hata seli olmaz.
	k.mock.ExpectExec("INSERT INTO uyari_durumlari").WithArgs(1, "web", "hata", 2, testSimdi).WillReturnResult(sqlmock.NewResult(0, 1))
	erisilemez := func(string) monitor.DomainHealth { return monitor.DomainHealth{} }
	if err := Tur(context.Background(), k.db, k.g, erisilemez, testSimdi); err != nil {
		t.Fatal(err)
	}
	if len(k.metinler) != 0 {
		t.Fatalf("mesaj gitmemeli: %q", k.metinler)
	}
}
