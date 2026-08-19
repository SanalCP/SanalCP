package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dovecot 2.3 ve 2.4 şablonları AYRI DİLLER.
//
// NEDEN BU TEST VAR: 2.4, yapılandırma sözdizimini kırdı. İki şablon yan yana
// duruyor ve biri düzenlenirken diğerinden satır kopyalamak çok kolay — sonuç,
// yalnız o dağıtımda ve yalnız posta kurulumunda ortaya çıkan bir hata olurdu
// (dovecot açılmaz, sanal kutular çalışmaz). Bu test, her şablonun kendi
// sözdiziminde kalmasını sabitler.
//
// 2.4 şablonu gerçek Dovecot 2.4.1 ayrıştırıcısına karşı doğrulandı
// (`doveconf -n`, Faz 5b hazırlığı); bu test o doğrulamanın yerini tutmaz,
// yalnız geriye kaymayı engeller.

func sablonOku(t *testing.T, ad string) string {
	t.Helper()
	yol, err := filepath.Abs(filepath.Join("../../assets/mail/dovecot", ad))
	if err != nil {
		t.Fatal(err)
	}
	ham, err := os.ReadFile(yol)
	if err != nil {
		t.Skipf("şablon okunamadı: %v", err)
	}
	return string(ham)
}

// yalnizAyarSatirlari: yorumları atar — açıklama metinlerinde eski adların
// geçmesi normaldir, asıl mesele AYAR satırlarıdır.
func yalnizAyarSatirlari(icerik string) string {
	var sb strings.Builder
	for _, ln := range strings.Split(icerik, "\n") {
		if t := strings.TrimSpace(ln); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		sb.WriteString(ln)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestDovecot23SablonuEskiSozdiziminde(t *testing.T) {
	a := yalnizAyarSatirlari(sablonOku(t, "10-sanalcp-mail.conf.tmpl"))
	for _, gerekli := range []string{"mail_location", "passdb {", "userdb {", "ssl_cert"} {
		if !strings.Contains(a, gerekli) {
			t.Errorf("2.3 şablonunda %q yok — 2.4 sözdizimi buraya sızmış olabilir", gerekli)
		}
	}
	for _, olmamali := range []string{"mail_driver", "passdb sql", "sql_driver", "ssl_server_cert_file"} {
		if strings.Contains(a, olmamali) {
			t.Errorf("2.3 şablonunda 2.4'e ait %q var — Dovecot 2.3 bunu tanımaz", olmamali)
		}
	}
}

func TestDovecot24SablonuYeniSozdiziminde(t *testing.T) {
	a := yalnizAyarSatirlari(sablonOku(t, "10-sanalcp-mail-2.4.conf.tmpl"))
	for _, gerekli := range []string{
		"mail_driver", "mail_path", "sql_driver", "passdb sql", "userdb sql",
		"ssl_server_cert_file", "ssl_server_key_file",
	} {
		if !strings.Contains(a, gerekli) {
			t.Errorf("2.4 şablonunda %q yok", gerekli)
		}
	}
	// 🔴 2.4'te KALKMIŞ ayarlar: biri kalırsa dovecot hiç açılmaz.
	for _, olmamali := range []string{"mail_location", "ssl_cert =", "ssl_key =", "driver = sql", "args = /etc/dovecot"} {
		if strings.Contains(a, olmamali) {
			t.Errorf("2.4 şablonunda 2.3'e ait %q var — Dovecot 2.4 bunu tanımaz", olmamali)
		}
	}
	// 2.4 değişken sözdizimi: %u yerine %{user}
	if strings.Contains(a, "'%u'") {
		t.Error("2.4 şablonunda %u var — 2.4'te %{user} kullanılır")
	}
	if !strings.Contains(a, "%{user}") {
		t.Error("2.4 şablonunda %{user} yok — SQL sorguları kullanıcıyı hiç eşleştiremez")
	}
}

// Her iki şablon da DB parolası yer tutucusunu taşımalı: kurulum betiği onu
// sed ile değiştiriyor, eksikse dovecot parolasız bağlanmaya çalışır.
func TestDovecotSablonlariParolaYerTutucusuTasir(t *testing.T) {
	if !strings.Contains(sablonOku(t, "10-sanalcp-mail-2.4.conf.tmpl"), "__PANEL_MAIL_DB_PASS__") {
		t.Error("2.4 şablonunda __PANEL_MAIL_DB_PASS__ yok")
	}
	if !strings.Contains(sablonOku(t, "dovecot-sql.conf.ext.tmpl"), "__PANEL_MAIL_DB_PASS__") {
		t.Error("2.3 SQL şablonunda __PANEL_MAIL_DB_PASS__ yok")
	}
}

// 🔴 Faz 5b canlı testinde bulundu: 2.4 şablonu mail_driver + mail_path yazıyor
// ama Debian'ın stok 10-mail.conf'u AYRI bir `mail_inbox_path` ayarı
// (/var/mail/%{user}, mbox) tanımlıyor ve bizim satırlarımız onu ezmiyordu.
// Sonuç: dovecot açılır, IMAP girişi çalışır, ama INBOX /var/mail altında mbox
// olarak açılmaya çalışıldığı için teslimat "Permission denied" ile düşerdi.
// %{home} 2.3'teki `mail_location = maildir:~/` yerleşimini birebir tekrarlar.
func TestDovecot24SablonuInboxYolunuEziyor(t *testing.T) {
	a := yalnizAyarSatirlari(sablonOku(t, "10-sanalcp-mail-2.4.conf.tmpl"))
	if !strings.Contains(a, "mail_inbox_path = %{home}") {
		t.Error("2.4 şablonunda `mail_inbox_path = %{home}` yok — stok mbox INBOX yolu ayakta kalır, posta teslim edilmez")
	}
}

// 🔴 Faz 5b canlı testinde bulundu: stok 20-lmtp.conf, protocol lmtp içinde
// `auth_username_format = %{user | username | lower}` yazar; `username` filtresi
// alan adını kırpar ve sanal kutu userdb sorgusu hiç eşleşmez
// (postfix: "550 5.1.1 User doesn't exist"). Dovecot conf.d/*.conf dosyalarını
// ALFABETİK yükler, yani 20-lmtp.conf bizim 10- dosyamızı ezer — override'ın
// 99- önekiyle AYRI dosyada durması bu testin asıl konusu.
func TestDovecot24LMTPKullaniciBicimiOverride(t *testing.T) {
	a := yalnizAyarSatirlari(sablonOku(t, "99-sanalcp-lmtp-2.4.conf.tmpl"))
	if !strings.Contains(a, "protocol lmtp") {
		t.Error("99- override dosyasında `protocol lmtp` bloğu yok")
	}
	if !strings.Contains(a, "auth_username_format = %{user | lower}") {
		t.Error("99- override dosyasında `auth_username_format = %{user | lower}` yok")
	}
	// `username` filtresi tam olarak kaçındığımız şey — sızarsa hata geri gelir.
	if strings.Contains(a, "| username") {
		t.Error("99- override dosyasında `| username` filtresi var — alan adını kırpar, sanal kutulara teslimat durur")
	}
	// Ana şablona yazmak İŞE YARAMAZ (20-lmtp.conf sonra yüklenir); oraya
	// konursa bu test sessizce yeşil kalmasın diye açıkça kontrol ediliyor.
	ana := yalnizAyarSatirlari(sablonOku(t, "10-sanalcp-mail-2.4.conf.tmpl"))
	if strings.Contains(ana, "auth_username_format") {
		t.Error("auth_username_format ana 2.4 şablonunda — stok 20-lmtp.conf onu ezer, override 99- dosyasında olmalı")
	}
}
