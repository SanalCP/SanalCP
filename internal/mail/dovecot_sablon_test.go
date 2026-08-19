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
