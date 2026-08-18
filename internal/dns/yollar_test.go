package dns

import (
	"strings"
	"testing"

	"sanalcp/internal/osfam"
)

// BIND yerleşimi yanlış çözülürse zone dosyaları named'in okuyamadığı bir yere
// yazılır ve DNS sessizce çalışmaz — bu yüzden her iki aile de sabitlenir.

func TestYollarSecRHEL(t *testing.T) {
	y := YollarSec(osfam.Bilgi{Aile: osfam.RHEL, ID: "almalinux", Surum: "10"})
	if y.ZoneDir != "/var/named" || y.Include != "/etc/named/sanalcp-zones.conf" {
		t.Fatalf("RHEL yolları değişmiş: %+v", y)
	}
	if y.Kullanici != "named" || y.Servis != "named" {
		t.Fatalf("RHEL kullanıcı/servis: %+v", y)
	}
	if !strings.HasPrefix(y.DNSSECKeyDir, y.ZoneDir+"/") {
		t.Fatalf("DNSSEC anahtar dizini zone dizininin altında olmalı: %+v", y)
	}
}

func TestYollarSecDebian(t *testing.T) {
	for _, b := range []osfam.Bilgi{
		{Aile: osfam.Debian, ID: "debian", Surum: "13", KodAdi: "trixie"},
		{Aile: osfam.Debian, ID: "ubuntu", Surum: "26.04", KodAdi: "resolute"},
	} {
		y := YollarSec(b)
		// 🔴 AppArmor: named /var/named'e YAZAMAZ. Yol oraya kayarsa DNS ölür.
		if strings.HasPrefix(y.ZoneDir, "/var/named") {
			t.Fatalf("%s: Debian zone dizini AppArmor'ın yazılabilir ağacında olmalı, /var/named değil: %+v", b.ID, y)
		}
		if y.ZoneDir != "/var/lib/bind" || y.Include != "/etc/bind/sanalcp-zones.conf" {
			t.Fatalf("%s: Debian yolları: %+v", b.ID, y)
		}
		if y.Kullanici != "bind" {
			t.Fatalf("%s: Debian'da zone sahibi bind olmalı: %+v", b.ID, y)
		}
		if y.Servis != "bind9" {
			t.Fatalf("%s: Debian servis adı bind9 olmalı (systemctl reload named başarısız olurdu): %+v", b.ID, y)
		}
		if !strings.HasPrefix(y.DNSSECKeyDir, y.ZoneDir+"/") {
			t.Fatalf("%s: DNSSEC anahtar dizini zone dizininin altında olmalı: %+v", b.ID, y)
		}
	}
}

// Paket düzeyindeki değişkenler init() ile dolduruluyor; boş kalırlarsa
// chown/systemctl çağrıları geçersiz argümanla gider.
func TestPaketDegiskenleriDolu(t *testing.T) {
	if ZoneDir == "" || NamedConfInclude == "" || DNSSECKeyDir == "" || dnsKullanici == "" || dnsServis == "" {
		t.Fatalf("init() eksik: ZoneDir=%q include=%q key=%q kullanici=%q servis=%q",
			ZoneDir, NamedConfInclude, DNSSECKeyDir, dnsKullanici, dnsServis)
	}
}
