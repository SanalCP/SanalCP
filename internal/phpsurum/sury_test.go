package phpsurum

import (
	"strings"
	"testing"
)

// sury düzeninde yollar ve servis adları. Buradaki bir hata, PHP sürümünün
// kurulmuş ama panelin onu hiç bulamaması demektir.
func TestYollarSury(t *testing.T) {
	m := SurumMeta{Surum: "8.3", Kod: "8.3", Kaynak: "sury"}
	pool, sock, svc, bin := yollar(m)

	if pool != "/etc/php/8.3/fpm/pool.d" {
		t.Errorf("pool: %q", pool)
	}
	if sock != "/run/php" {
		t.Errorf("sock: %q", sock)
	}
	if svc != "php8.3-fpm" {
		t.Errorf("service: %q", svc)
	}
	if bin != "/usr/bin/php8.3" {
		t.Errorf("bin: %q", bin)
	}
}

// Remi düzeni bozulmamalı — Debian eklenirken RHEL yolu değişmemeli.
func TestYollarRemiVeAppstreamKorundu(t *testing.T) {
	pool, sock, svc, bin := yollar(SurumMeta{Surum: "8.2", Kod: "82", Kaynak: "remi"})
	if pool != "/etc/opt/remi/php82/php-fpm.d" || svc != "php82-php-fpm" ||
		sock != "/var/opt/remi/php82/run/php-fpm" || bin != "/opt/remi/php82/root/usr/bin/php" {
		t.Errorf("remi yolları değişmiş: %q %q %q %q", pool, sock, svc, bin)
	}
	pool, sock, svc, bin = yollar(SurumMeta{Surum: "8.3", Kod: "", Kaynak: "appstream"})
	if pool != "/etc/php-fpm.d" || sock != "/run/php-fpm" || svc != "php-fpm" || bin != "/usr/bin/php" {
		t.Errorf("appstream yolları değişmiş: %q %q %q %q", pool, sock, svc, bin)
	}
}

func TestFpmPaketAdi(t *testing.T) {
	if got := fpmPaketAdi(SurumMeta{Kod: "8.3", Kaynak: "sury"}); got != "php8.3-fpm" {
		t.Errorf("sury: %q", got)
	}
	if got := fpmPaketAdi(SurumMeta{Kod: "83", Kaynak: "remi"}); got != "php83-php-fpm" {
		t.Errorf("remi: %q", got)
	}
}

// 🔴 En riskli fonksiyon: yanlış desen, İSTENMEYEN paketleri kaldırabilir.
// apt özel karakterli argümanı REGEX sayar; "php8.3-*" içindeki nokta herhangi
// bir karaktere karşılık gelirdi. Nokta kaçırılmalı ve desen başa çapalanmalı.
func TestKaldirDeseni(t *testing.T) {
	sury := kaldirDeseni(SurumMeta{Kod: "8.3", Kaynak: "sury"})
	if sury != `^php8\.3-` {
		t.Fatalf("sury deseni: got %q want %q", sury, `^php8\.3-`)
	}
	if !strings.HasPrefix(sury, "^") {
		t.Error("desen başa çapalanmamış — başka paketleri eşleyebilir")
	}
	if strings.Contains(sury, "8.3") {
		t.Error("nokta kaçırılmamış — regex'te herhangi bir karaktere karşılık gelir")
	}

	// RHEL davranışı korunmalı (dnf glob'u shell tarzı yorumlar).
	if got := kaldirDeseni(SurumMeta{Kod: "83", Kaynak: "remi"}); got != "php83-*" {
		t.Errorf("remi deseni: %q", got)
	}
}

// sury paket adları "php<sürüm>-<eklenti>" kalıbında olmalı ve Debian'da
// bulunmayan adlar (mysqlnd, pdo) İSTENMEMELİ — biri istenirse apt tüm
// kurulumu "paket bulunamadı" ile reddeder.
func TestPaketAdlariSury(t *testing.T) {
	adlar := PaketAdlari(SurumMeta{Surum: "8.2", Kod: "8.2", Kaynak: "sury"})
	if len(adlar) == 0 {
		t.Fatal("boş liste")
	}
	for _, ad := range adlar {
		if !strings.HasPrefix(ad, "php8.2-") {
			t.Errorf("beklenmeyen paket adı: %q", ad)
		}
	}
	birlesik := strings.Join(adlar, " ")
	for _, yasak := range []string{"mysqlnd", "-pdo", "pecl"} {
		if strings.Contains(birlesik, yasak) {
			t.Errorf("Debian'da bulunmayan paket isteniyor (%q): %s", yasak, birlesik)
		}
	}
	// FPM ve CLI olmazsa olmaz.
	for _, gerekli := range []string{"php8.2-fpm", "php8.2-cli", "php8.2-mysql"} {
		if !strings.Contains(birlesik, gerekli) {
			t.Errorf("%q eksik: %s", gerekli, birlesik)
		}
	}
}

// Remi paket adları bozulmamalı.
func TestPaketAdlariRemiKorundu(t *testing.T) {
	adlar := PaketAdlari(SurumMeta{Surum: "8.2", Kod: "82", Kaynak: "remi"})
	birlesik := strings.Join(adlar, " ")
	if !strings.Contains(birlesik, "php82-php-fpm") {
		t.Errorf("remi adları değişmiş: %s", birlesik)
	}
}
