package osfam

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Zaman aşımında süreç SIGKILL ile öldürülürse kendi geçici dosyalarını
// silemez. dnf bu durumda /var/cache/dnf/*/tmpdir.* artıklarını bırakıyor ve
// 10 dakikalık bir döngüde 3 haftada 43 GB'lık diski doldurdu. Komut, iptalde
// önce SIGTERM göndermeli ki süreç temizliğini yapabilsin.
func TestKomutIptaldeSIGTERMGonderir(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	betik := `trap 'echo temizlendi; exit 0' TERM; sleep 5 >/dev/null 2>&1 & wait`
	out, _ := (Bilgi{Aile: RHEL}).Komut(ctx, []string{"sh", "-c", betik}).Output()
	if !strings.Contains(string(out), "temizlendi") {
		t.Fatalf("süreç SIGTERM almadı (SIGKILL ile öldürüldü), çıktı: %q", out)
	}
}

// Arka plan sorguları metadata indirmemeli: RHEL'de dnf -C (yalnız önbellek).
// Debian'daki dpkg-query / apt-cache zaten yerel çalıştığı için değişmez.
func TestOnbellektenArgs(t *testing.T) {
	rhel := Bilgi{Aile: RHEL}
	got := rhel.Onbellekten(rhel.MevcutMuArgs("php85-php-fpm"))
	want := []string{"dnf", "-C", "-q", "list", "--available", "php85-php-fpm"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RHEL: got %v, want %v", got, want)
	}
	deb := Bilgi{Aile: Debian}
	if got, want := deb.Onbellekten(deb.MevcutMuArgs("php8.4-fpm")), deb.MevcutMuArgs("php8.4-fpm"); !reflect.DeepEqual(got, want) {
		t.Errorf("Debian: got %v, want %v", got, want)
	}
}
