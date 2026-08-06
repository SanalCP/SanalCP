package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

// 🔴 Kazara-silme koruması: /mail/hizmet YIKICI uçtur, /mail/{mid} ise tek bir
// kutuyu siler. chi statik segmenti parametreli olana tercih etmeseydi
// "hizmet" bir {mid} sanılır (ya da tersi olur) ve kullanıcı tek kutu silmek
// isterken TÜM hizmeti kaybederdi. Bu testi rota eklerken kırmak kolay.
func TestRotaHizmetVeMailboxKarismaz(t *testing.T) {
	vurulan := ""
	r := chi.NewRouter()
	r.Delete("/domains/{id}/mail/etkinlestir", func(http.ResponseWriter, *http.Request) { vurulan = "devredisi" })
	r.Delete("/domains/{id}/mail/hizmet", func(http.ResponseWriter, *http.Request) { vurulan = "hizmet" })
	r.Delete("/domains/{id}/mail/{mid}", func(_ http.ResponseWriter, req *http.Request) {
		vurulan = "mailbox:" + chi.URLParam(req, "mid")
	})

	for _, d := range []struct{ yol, bekleyen string }{
		{"/domains/7/mail/hizmet", "hizmet"},
		{"/domains/7/mail/etkinlestir", "devredisi"},
		{"/domains/7/mail/42", "mailbox:42"},
	} {
		vurulan = ""
		req := httptest.NewRequest(http.MethodDelete, d.yol, nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
		if vurulan != d.bekleyen {
			t.Errorf("%s -> %q, beklenen %q", d.yol, vurulan, d.bekleyen)
		}
	}
}

// TumunuKaldir'ın SQL sözleşmesi: mail_domains'e CASCADE ETMEYEN tablolar
// (mail_aliases, mail_send_log, mail_spam_settings — hepsi domains(id)'e bağlı)
// elle silinmeli, hepsi TEK transaction'da olmalı. Biri unutulursa domain
// yeniden etkinleştirildiğinde eski alias'lar/spam ayarları geri gelir.
func TestTumunuKaldirTumTablolariTemizler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT sistem_kullanici FROM domains WHERE id=?`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici"}).AddRow("c_ornek"))
	mock.ExpectBegin()
	for _, tablo := range []string{"mail_aliases", "mail_send_log", "mail_spam_settings", "mail_domains"} {
		mock.ExpectExec(`DELETE FROM ` + tablo + ` WHERE domain_id=\?`).
			WithArgs(int64(7)).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	// /home/c_ornek/mail yok → jailpath.Sil ENOENT döner. Bu BAŞARIDIR: silinecek
	// dosya zaten yoksa iş bitmiştir. Hem err hem diskHata nil olmalı.
	diskHata, err := TumunuKaldir(context.Background(), db, 7)
	if err != nil {
		t.Fatalf("DB temizliği başarısız oldu: %v", err)
	}
	if diskHata != nil {
		t.Errorf("maildir zaten yokken diskHata raporlanmamalı (ENOENT = başarı), geldi: %v", diskHata)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL beklentileri karşılanmadı: %v", err)
	}
}

// Disk silme GERÇEKTEN başarısızsa bu yutulmamalı: DB temizliği başarılı olsa
// bile kullanıcı, posta dosyalarının diskte kaldığını öğrenmeli (yer kaplarlar).
// Geçersiz tenant adı jailpath.TenantHome'un güvenlik kapısına takılır — bu
// aynı zamanda "c_" öneki doğrulamasının hâlâ yerinde olduğunu da sınar.
func TestTumunuKaldirDiskHatasiYutulmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT sistem_kullanici FROM domains WHERE id=?`)).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici"}).AddRow("kotu/../ad"))
	mock.ExpectBegin()
	for _, tablo := range []string{"mail_aliases", "mail_send_log", "mail_spam_settings", "mail_domains"} {
		mock.ExpectExec(`DELETE FROM ` + tablo + ` WHERE domain_id=\?`).
			WithArgs(int64(3)).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	diskHata, err := TumunuKaldir(context.Background(), db, 3)
	if err != nil {
		t.Fatalf("DB temizliği başarılı olmalıydı: %v", err)
	}
	if diskHata == nil {
		t.Error("geçersiz tenant adında diskHata bekleniyordu — disk hatası yutulursa " +
			"kullanıcı dosyaların kaldığını öğrenemez")
	}
}

// DB hatası dosyaları SİLDİRMEMELİ ve transaction commit EDİLMEMELİ: aksi hâlde
// yarım temizlenmiş bir durumda kalırız (kutular DB'de, dosyalar diskte yok).
func TestTumunuKaldirDBHatasindaDiskeDokunmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT sistem_kullanici FROM domains WHERE id=?`)).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici"}).AddRow("c_ornek"))
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM mail_aliases WHERE domain_id=\?`).
		WithArgs(int64(9)).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	diskHata, err := TumunuKaldir(context.Background(), db, 9)
	if err == nil {
		t.Fatal("DB hatası err olarak dönmeliydi")
	}
	if diskHata != nil {
		t.Errorf("DB başarısızken disk hatası raporlanmamalı, geldi: %v", diskHata)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("rollback beklentisi karşılanmadı: %v", err)
	}
}
