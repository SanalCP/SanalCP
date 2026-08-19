package osfam

import "testing"

// MariaDB soketi: yol yanlışsa panel Debian'da HİÇ açılmaz — Faz 5a canlı
// testinde tam olarak bu oldu (hesaplar.Init root mysql ping başarısız →
// crash-loop). Sonda + varsayılan davranışı burada sabitleniyor.
func TestMariaDBSoketSec(t *testing.T) {
	yok := func(string) bool { return false }
	deb := Bilgi{Aile: Debian, ID: "debian", Surum: "12", KodAdi: "bookworm"}
	rhel := Bilgi{Aile: RHEL, ID: "almalinux", Surum: "10"}

	if g := mariaDBSoketSec(deb, yok); g != "/run/mysqld/mysqld.sock" {
		t.Errorf("Debian varsayılanı: %q", g)
	}
	if g := mariaDBSoketSec(rhel, yok); g != "/var/lib/mysql/mysql.sock" {
		t.Errorf("RHEL varsayılanı: %q", g)
	}

	// Var olan aday aile varsayılanını EZER: özelleştirilmiş my.cnf senaryosu.
	sadece := func(hedef string) func(string) bool {
		return func(y string) bool { return y == hedef }
	}
	if g := mariaDBSoketSec(deb, sadece("/var/lib/mysql/mysql.sock")); g != "/var/lib/mysql/mysql.sock" {
		t.Errorf("Debian'da var olan RHEL yolu seçilmeliydi: %q", g)
	}
	if g := mariaDBSoketSec(rhel, sadece("/run/mysqld/mysqld.sock")); g != "/run/mysqld/mysqld.sock" {
		t.Errorf("RHEL'de var olan Debian yolu seçilmeliydi: %q", g)
	}
}
