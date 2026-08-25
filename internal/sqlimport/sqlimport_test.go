package sqlimport

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDefinerSuzTemizler(t *testing.T) {
	girdi := strings.Join([]string{
		"/*!50001 CREATE ALGORITHM=UNDEFINED */",
		"/*!50013 DEFINER=`root`@`localhost` SQL SECURITY DEFINER */",
		"/*!50001 VIEW `v_ozet` AS select 1 */;",
		"CREATE DEFINER=`eski_user`@`10.0.0.1` PROCEDURE `p`() BEGIN END;",
		"CREATE DEFINER='q'@'%' TRIGGER t BEFORE INSERT ON x FOR EACH ROW SET @a=1;",
		"INSERT INTO `notlar` VALUES ('burada DEFINER kelimesi metin icinde');",
	}, "\n")

	var out bytes.Buffer
	if err := definerSuz(strings.NewReader(girdi), &out); err != nil {
		t.Fatal(err)
	}
	sonuc := out.String()

	for _, yasak := range []string{"DEFINER=`root`", "DEFINER=`eski_user`", "DEFINER='q'"} {
		if strings.Contains(sonuc, yasak) {
			t.Errorf("DEFINER yan tümcesi kaldı: %q", yasak)
		}
	}
	// SQL SECURITY yan tümcesine DOKUNULMAMALI (bkz. reDefiner yorumu).
	if !strings.Contains(sonuc, "SQL SECURITY DEFINER") {
		t.Error("SQL SECURITY DEFINER yan tümcesi değiştirilmemeliydi")
	}
	// Nesne tanımlarının kendisi bozulmamalı.
	for _, kalmali := range []string{"PROCEDURE `p`()", "TRIGGER t BEFORE INSERT", "VIEW `v_ozet`"} {
		if !strings.Contains(sonuc, kalmali) {
			t.Errorf("ifade bozuldu, %q kaybolmuş:\n%s", kalmali, sonuc)
		}
	}
	// Veri satırındaki "DEFINER" kelimesi metindir, dokunulmamalı.
	if !strings.Contains(sonuc, "burada DEFINER kelimesi metin icinde") {
		t.Error("veri satırındaki DEFINER kelimesi değiştirildi")
	}
}

// Tampondan uzun satırlar (dev extended-insert) süzgeçsiz ama KAYIPSIZ geçmeli.
func TestDefinerSuzUzunSatirKayipsiz(t *testing.T) {
	uzun := "INSERT INTO t VALUES (" + strings.Repeat("'x',", (filtreTampon/4)+50) + "'son');"
	girdi := uzun + "\nSELECT 1;\n"
	var out bytes.Buffer
	if err := definerSuz(strings.NewReader(girdi), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != girdi {
		t.Fatalf("uzun satır kayıpsız aktarılmadı: %d bayt girdi, %d bayt çıktı",
			len(girdi), out.Len())
	}
}

func TestDefaultsDosyaIzinVeKacis(t *testing.T) {
	h := Hedef{
		DBAdi:     "c_test_wp",
		Kullanici: "c_test_wp",
		Parola:    `p"a\s"s`,
		Host:      "localhost",
	}
	yol, temizle, err := defaultsDosya(h)
	if err != nil {
		t.Fatal(err)
	}
	defer temizle()

	fi, err := os.Stat(yol)
	if err != nil {
		t.Fatal(err)
	}
	// Parola dosyada duruyor → yalnız sahibi okuyabilmeli.
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("izin 0600 olmalı, %o", fi.Mode().Perm())
	}
	b, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	icerik := string(b)
	if !strings.Contains(icerik, `password="p\"a\\s\"s"`) {
		t.Errorf("parola option-file için kaçırılmadı:\n%s", icerik)
	}
	// Parola argv'ye HİÇ girmemeli — bu testin varlık sebebi.
	if strings.Contains(icerik, "\n\n") {
		t.Error("beklenmeyen boş satır")
	}
}

func TestHedefDogrula(t *testing.T) {
	gecerli := Hedef{DBAdi: "c_a_wp", Kullanici: "c_a_wp", Parola: "x"}
	if err := gecerli.dogrula(); err != nil {
		t.Errorf("geçerli hedef reddedildi: %v", err)
	}
	kotu := []Hedef{
		{DBAdi: "a b", Kullanici: "u", Parola: "x"},
		{DBAdi: "a`b", Kullanici: "u", Parola: "x"},
		{DBAdi: "mysql; DROP", Kullanici: "u", Parola: "x"},
		{DBAdi: "a", Kullanici: "u'--", Parola: "x"},
		{DBAdi: "a", Kullanici: "u", Parola: ""},                           // parolasız → root'a düşme riski
		{DBAdi: "a", Kullanici: "u", Parola: "x\ninjected=evil", Host: ""}, // parolada satır sonu → option-file enjeksiyonu
		{DBAdi: "a", Kullanici: "u", Parola: "x", Host: "localhost\r\ninjected=evil"},
	}
	for _, h := range kotu {
		if err := h.dogrula(); err == nil {
			t.Errorf("geçersiz hedef kabul edildi: %+v", h)
		}
	}
}
