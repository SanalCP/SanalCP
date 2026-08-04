package provisioner

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWwwHostlar(t *testing.T) {
	if got := wwwHostlar("ornek.com"); len(got) != 2 || got[0] != "ornek.com" || got[1] != "www.ornek.com" {
		t.Errorf("apex için [domain, www.domain] bekleniyordu: %v", got)
	}
	if got := wwwHostlar("www.ornek.com"); len(got) != 1 || got[0] != "www.ornek.com" {
		t.Errorf("zaten www olan host çoğaltılmamalı: %v", got)
	}
}

func TestOrtakAdresVar(t *testing.T) {
	ip := func(s string) net.IPAddr { return net.IPAddr{IP: net.ParseIP(s)} }
	apex := []net.IPAddr{ip("1.2.3.4"), ip("2001:db8::1")}

	if !ortakAdresVar(apex, []net.IPAddr{ip("9.9.9.9"), ip("1.2.3.4")}) {
		t.Error("ortak IPv4 bulunmalıydı")
	}
	if !ortakAdresVar(apex, []net.IPAddr{ip("2001:db8::1")}) {
		t.Error("ortak IPv6 bulunmalıydı")
	}
	if ortakAdresVar(apex, []net.IPAddr{ip("5.6.7.8")}) {
		t.Error("ortak adres yokken true döndü")
	}
	if ortakAdresVar(apex, nil) {
		t.Error("boş liste ile true döndü")
	}
}

// Hiç çözülmeyen bir apex için ACME'ye HİÇ gidilmemeli: leHostlari net bir hata
// döndürmeli (rate-limit yakılmaz, kullanıcı sebebi görür).
func TestLeHostlariCozulmeyenApexHataVerir(t *testing.T) {
	// RFC 2606: .invalid TLD'si asla çözülmez.
	_, _, err := leHostlari(context.Background(), "kesinlikle-yok-12345.invalid")
	if err == nil {
		t.Fatal("çözülmeyen apex için hata bekleniyordu")
	}
	if !strings.Contains(err.Error(), "DNS") {
		t.Errorf("hata mesajı sebebi açıklamalı: %q", err)
	}
}

// Asıl regresyon: apex çözülüyor ama "www." çözülmüyorsa, www sertifikaya
// EKLENMEMELİ ve işlem hata vermemeli. Bunun tersi (0.4.1 ve öncesi davranışı)
// tüm ACME siparişini düşürüyor, panel self-signed'a geri düşüyor ve tarayıcı
// "güvenli değil" diyordu.
func TestLeHostlariCozulmeyenWwwAtlanir(t *testing.T) {
	// example.com çözülür; www.example.com da çözülür — bu yüzden gerçek DNS'e
	// bağlı bir senaryo yerine seçim mantığını doğrudan doğrularız.
	ip := func(s string) net.IPAddr { return net.IPAddr{IP: net.ParseIP(s)} }
	apexAdres := []net.IPAddr{ip("178.162.242.174")}

	// www hiç çözülmüyor (adres listesi boş) → eklenmemeli.
	if ortakAdresVar(apexAdres, nil) {
		t.Error("çözülmeyen www eklenebilir görünüyor")
	}
	// www başka sunucuya bakıyor → eklenmemeli.
	if ortakAdresVar(apexAdres, []net.IPAddr{ip("203.0.113.10")}) {
		t.Error("farklı sunucuya bakan www eklenebilir görünüyor")
	}
	// www aynı sunucuya bakıyor → eklenmeli.
	if !ortakAdresVar(apexAdres, []net.IPAddr{ip("178.162.242.174")}) {
		t.Error("aynı sunucuya bakan www eklenmiyor")
	}
}

// Regresyon: apex-only alınmış GEÇERLİ bir Let's Encrypt sertifikası,
// "www kapsanmıyor" diye reddedilmemeli.
//
// Reddedilseydi zincirleme bozulma olurdu: enIyiCertBul cert'i bulamaz →
// EnableLetsEncrypt reuse'a girmez → her çağrıda yeniden çekim (rate-limit) →
// sslFailSafe de bulamaz → çalışan LE cert'inin üstüne self-signed yazılır.
func TestCertGecerliMiApexOnlySertifikayiKabulEder(t *testing.T) {
	dizin := t.TempDir()
	certYol, keyYol := filepath.Join(dizin, "t.crt"), filepath.Join(dizin, "t.key")
	// SAN: yalnız apex.
	if out, err := exec.Command("openssl", "req", "-x509", "-nodes",
		"-newkey", "rsa:2048", "-keyout", keyYol, "-out", certYol,
		"-days", "90", "-subj", "/CN=ornek.test",
		"-addext", "subjectAltName=DNS:ornek.test").CombinedOutput(); err != nil {
		t.Skipf("openssl yok/çalışmadı: %s", out)
	}

	if !certGecerliMi(certYol, keyYol, 30, "ornek.test") {
		t.Error("apex-only sertifika apex sorgusunda geçerli sayılmalıydı")
	}
	// Eski davranış (www da şart) bu sertifikayı reddederdi — belgeliyoruz.
	if certGecerliMi(certYol, keyYol, 30, "ornek.test", "www.ornek.test") {
		t.Error("www SAN'ı olmayan sertifika www sorulunca geçerli sayılmamalı")
	}
}
