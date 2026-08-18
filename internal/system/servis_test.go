package system

import (
	"testing"

	"sanalcp/internal/osfam"
)

// Yanlış unit adı = UI'da "absent" + yeniden başlatma düğmesinin sessizce
// çalışmaması. Her iki ailenin adları sabitleniyor.

func birimler(l []servisTanim) map[string]bool {
	m := map[string]bool{}
	for _, s := range l {
		m[s.Birim] = true
	}
	return m
}

func TestServisAllowRHEL(t *testing.T) {
	b := birimler(servisAllowSec(osfam.Bilgi{Aile: osfam.RHEL, ID: "almalinux", Surum: "10"}))
	for _, istenen := range []string{"nginx", "httpd", "mariadb", "valkey", "named", "php-fpm", "php82-php-fpm", "php74-php-fpm", "pure-ftpd", "crond"} {
		if !b[istenen] {
			t.Errorf("RHEL listesinde %q yok: %v", istenen, b)
		}
	}
}

func TestServisAllowDebian(t *testing.T) {
	b := birimler(servisAllowSec(osfam.Bilgi{Aile: osfam.Debian, ID: "debian", Surum: "13", KodAdi: "trixie"}))
	for _, istenen := range []string{"nginx", "mariadb", "valkey-server", "bind9", "php8.3-fpm", "php8.2-fpm", "php7.4-fpm", "pure-ftpd-mysql", "cron"} {
		if !b[istenen] {
			t.Errorf("Debian listesinde %q yok: %v", istenen, b)
		}
	}
	// RHEL unit adları sızmamalı.
	for _, olmamali := range []string{"named", "crond", "php-fpm", "pure-ftpd", "httpd"} {
		if b[olmamali] {
			t.Errorf("Debian listesinde RHEL unit'i %q var: %v", olmamali, b)
		}
	}
	// Apache backend Debian'da kapalı (osfam.ApacheBackendDestekli ile tutarlı).
	if b["apache2"] {
		t.Errorf("Apache backend Debian'da v1'de kapalı olmalı: %v", b)
	}
}

// Debian 12'de valkey-server paketi yok → redis-server'a düşülür; servis
// listesi de bunu izlemeli (bkz. osfam.ozelServis).
func TestServisAllowDebian12Redis(t *testing.T) {
	b := birimler(servisAllowSec(osfam.Bilgi{Aile: osfam.Debian, ID: "debian", Surum: "12", KodAdi: "bookworm"}))
	if !b["redis-server"] || b["valkey-server"] {
		t.Errorf("Debian 12'de redis-server bekleniyordu: %v", b)
	}
}
