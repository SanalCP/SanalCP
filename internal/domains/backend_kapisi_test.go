package domains

import (
	"testing"

	"sanalcp/internal/osfam"
)

// Apache backend'i Debian ailesinde v1'de KAPALI.
//
// 🔴 NEDEN BU TEST VAR: kapı fonksiyonu (osfam.ApacheBackendDestekli) uzun süre
// VARDI ama hiçbir yerde ÇAĞRILMIYORDU — yalnız yorumlarda anılıyordu. Ubuntu
// 24.04'te canlı denendiğinde DB 'apache' oldu, nginx 127.0.0.1:10080'e proxy'ledi
// ve site 502 verdi. Bir kapının var olması, uygulandığı anlamına gelmez.
// osIle: testin süresince tespit edilen işletim sistemini değiştirir.
// osfam.Ayarla global durumu değiştirdiği için önceki değer geri konur.
func osIle(t *testing.T, b osfam.Bilgi, f func()) {
	t.Helper()
	onceki := osfam.Mevcut()
	defer osfam.Ayarla(onceki)
	osfam.Ayarla(b)
	f()
}

var (
	debian13 = osfam.Bilgi{Aile: osfam.Debian, ID: "debian", Surum: "13", KodAdi: "trixie"}
	ubuntu24 = osfam.Bilgi{Aile: osfam.Debian, ID: "ubuntu", Surum: "24.04", KodAdi: "noble"}
	alma10   = osfam.Bilgi{Aile: osfam.RHEL, ID: "almalinux", Surum: "10.0"}
)

func TestApacheBackendDebianAilesindeSecilemez(t *testing.T) {
	for ad, b := range map[string]osfam.Bilgi{"debian13": debian13, "ubuntu2404": ubuntu24} {
		t.Run(ad, func(t *testing.T) {
			osIle(t, b, func() {
				if backendKullanilabilir("apache") {
					t.Error("apache seçilebilir görünüyor — DB 'apache' olur, nginx 10080'e proxy'ler, site 502 verir")
				}
				// Diğer ikisi her yerde çalışmalı.
				for _, b := range []string{"php-fpm", "static"} {
					if !backendKullanilabilir(b) {
						t.Errorf("%s seçilebilir olmalı", b)
					}
				}
			})
		})
	}
}

func TestApacheBackendRHELdeSecilebilir(t *testing.T) {
	osIle(t, alma10, func() {
		if !backendKullanilabilir("apache") {
			t.Error("RHEL'de apache backend kapatılmamalı — mevcut kurulumlarda kullanılıyor")
		}
	})
}

// UI'a gönderilen liste de sunucunun gerçeğini yansıtmalı: çalışmayacak bir
// seçeneği menüde göstermek, kullanıcıyı siteyi düşüren bir tıklamaya davet eder.
func TestMevcutBackendListesiSistemeGoreDegisir(t *testing.T) {
	osIle(t, ubuntu24, func() {
		got := kullanilabilirBackendler()
		for _, b := range got {
			if b == "apache" {
				t.Fatalf("Ubuntu'da liste apache içeriyor: %v", got)
			}
		}
		if len(got) != 2 {
			t.Errorf("beklenen [php-fpm static], gelen %v", got)
		}
	})
	osIle(t, alma10, func() {
		got := kullanilabilirBackendler()
		var apacheVar bool
		for _, b := range got {
			if b == "apache" {
				apacheVar = true
			}
		}
		if !apacheVar {
			t.Errorf("RHEL'de liste apache içermeli: %v", got)
		}
	})
}
