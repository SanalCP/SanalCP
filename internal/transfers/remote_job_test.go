package transfers

import (
	"bytes"
	"strings"
	"testing"
)

func TestSinirliYazici(t *testing.T) {
	var b bytes.Buffer
	w := &sinirliYazici{w: &b, kalan: 4}
	if n, err := w.Write([]byte("1234")); err != nil || n != 4 {
		t.Fatalf("ilk yazma n=%d err=%v", n, err)
	}
	if _, err := w.Write([]byte("5")); err == nil {
		t.Fatal("sınır aşımı kabul edildi")
	}
}

func TestUzakHesapSaglayiciyaGoreDogrulanir(t *testing.T) {
	if !uzakHesapGecerli("plesk", "example.com", "example.com") {
		t.Fatal("Plesk site kimliği reddedildi")
	}
	if uzakHesapGecerli("plesk", "other.example", "example.com") {
		t.Fatal("Plesk'te başka site kimliği kabul edildi")
	}
	if !uzakHesapGecerli("directadmin", "demo_user", "example.com") {
		t.Fatal("DirectAdmin kullanıcı adı reddedildi")
	}
}

func TestTumSaglayicilarAkisArsiviUretir(t *testing.T) {
	for _, p := range []string{"sanalcp", "cpanel", "plesk", "directadmin"} {
		hesap := "demo"
		if p == "plesk" {
			hesap = "example.com"
		}
		cmd, err := uzakPaketKomutu(remoteStartReq{Provider: p, Hesap: hesap, Domain: "example.com"})
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if p == "sanalcp" {
			if cmd != "sanalcp-transfer-export export example.com" {
				t.Errorf("SanalCP native exporter kullanılmadı: %q", cmd)
			}
			continue
		}
		if !strings.Contains(cmd, "tar") || !strings.Contains(cmd, "cpmove-") {
			t.Errorf("%s ortak arşiv sözleşmesini üretmedi", p)
		}
	}
}

func TestHTTPSaglikSiniri(t *testing.T) {
	for _, v := range []int{200, 301, 404} {
		if !httpSaglikli(v) {
			t.Errorf("%d sağlıklı sayılmalı", v)
		}
	}
	for _, v := range []int{0, 199, 500, 503} {
		if httpSaglikli(v) {
			t.Errorf("%d sağlıksız sayılmalı", v)
		}
	}
}

func TestNativeVarsayilanVeritabaniOnceEslestirilir(t *testing.T) {
	got := nativeDatabaseSources([]string{"demo_extra", "demo_main"}, []byte(`{"version":1,"default_db":"demo_main"}`))
	if len(got) != 2 || got[0] != "demo_main" || got[1] != "demo_extra" {
		t.Fatalf("varsayılan veritabanı sırası hatalı: %#v", got)
	}
}

func TestUzakPaketHesabiYalnizGuvenliUnixAdi(t *testing.T) {
	for _, s := range []string{"root;id", "x$(id)", "../root", "UPPER"} {
		if uzakUserRe.MatchString(s) {
			t.Errorf("tehlikeli hesap kabul edildi: %q", s)
		}
	}
}

// FPM'in açılış NOTICE'leri teşhis değeri taşımaz; gerçek hata satırlarını
// gölgelememeleri gerekir (bkz. hedefHataOzeti).
func TestHataSatirlariAcilisGurultusunuEler(t *testing.T) {
	gurultu := "[03-Sep-2026 07:22:36] NOTICE: configuration file /etc/php-fpm-tenant/c_x/php-fpm.conf test is successful\n" +
		"[03-Sep-2026 07:22:38] NOTICE: fpm is running, pid 178293\n" +
		"[03-Sep-2026 07:22:38] NOTICE: ready to handle connections\n" +
		"[03-Sep-2026 07:22:38] NOTICE: systemd monitor interval set to 10000ms\n"
	if got := hataSatirlari(gurultu); got != "" {
		t.Fatalf("açılış gürültüsü elenmeliydi: %q", got)
	}
	ile := gurultu + `[03-Sep-2026 07:22:41] NOTICE: PHP message: PHP Fatal error: Uncaught PDOException: SQLSTATE[HY000] [1045] Access denied in /home/c_x/public_html/db.php:9` + "\n"
	got := hataSatirlari(ile)
	if !strings.Contains(got, "PDOException") {
		t.Fatalf("gerçek hata satırı kayboldu: %q", got)
	}
	if strings.Contains(got, "fpm is running") {
		t.Fatalf("açılış satırı sızdı: %q", got)
	}
}

func TestHataSatirlariNginxHatasiniYakalar(t *testing.T) {
	tail := "2026/09/03 07:22:41 [error] 812#812: *3 FastCGI sent in stderr: \"PHP message: ...\" while reading response header from upstream, client: 127.0.0.1\n"
	if got := hataSatirlari(tail); !strings.Contains(got, "[error]") {
		t.Fatalf("nginx hata satırı yakalanamadı: %q", got)
	}
}

// Aktarım başarılı olsa bile eksik kalanlar yöneticiye görünmeli; bu site
// sessizce kırık kalmasının tek sebebiydi.
func TestBasariMesajiAtlananlariEkler(t *testing.T) {
	got := basariMesaji("Aktarım tamamlandı", []string{"", "  ", "src/, vendor/ aktarılmadı"})
	if !strings.Contains(got, "DİKKAT") || !strings.Contains(got, "vendor/") {
		t.Fatalf("atlananlar mesaja girmedi: %q", got)
	}
	if bos := basariMesaji("Aktarım tamamlandı", []string{" ", ""}); bos != "Aktarım tamamlandı" {
		t.Fatalf("boş uyarı listesi mesajı bozdu: %q", bos)
	}
	if bos := basariMesaji("Aktarım tamamlandı", nil); bos != "Aktarım tamamlandı" {
		t.Fatalf("nil uyarı listesi mesajı bozdu: %q", bos)
	}
}
