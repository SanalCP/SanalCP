package httpx

import (
	"net"
	"net/http/httptest"
	"testing"
)

func loopbackGuvenilir(t *testing.T) {
	t.Helper()
	_, v4, _ := net.ParseCIDR("127.0.0.1/32")
	_, v6, _ := net.ParseCIDR("::1/128")
	SetTrustedProxies([]*net.IPNet{v4, v6})
	t.Cleanup(func() {
		SetTrustedProxies([]*net.IPNet{})
		SetProxyAnahtari("")
		SetLoopbackAnahtarsizGuven(false)
	})
}

// Bulgu (2026-10-02): tenant süreci 127.0.0.1:8080'e doğrudan bağlanıp sahte
// X-Forwarded-For ile panel IP izin listesini ve giriş sınırını atlatabiliyordu.
func TestClientIPLoopbackAnahtarsizXFFReddedilir(t *testing.T) {
	loopbackGuvenilir(t)
	SetProxyAnahtari("dogru-anahtar")
	for _, uzak := range []string{"127.0.0.1:5555", "[::1]:5555"} {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = uzak
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		r.Header.Set("X-Real-IP", "203.0.113.7")
		if got := ClientIP(r); got == "203.0.113.7" {
			t.Fatalf("%s: anahtarsız loopback isteğinde sahte XFF kabul edildi", uzak)
		}
		r.Header.Set("X-SanalCP-Proxy", "yanlis-anahtar")
		if got := ClientIP(r); got == "203.0.113.7" {
			t.Fatalf("%s: yanlış anahtarla sahte XFF kabul edildi", uzak)
		}
	}
}

func TestClientIPLoopbackDogruAnahtarlaXFFKabulEdilir(t *testing.T) {
	loopbackGuvenilir(t)
	SetProxyAnahtari("dogru-anahtar")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("X-SanalCP-Proxy", "dogru-anahtar")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("nginx anahtarıyla gelen istemci IP'si = %q, beklenen 203.0.113.7", got)
	}
}

// Anahtar hiç yüklenemediyse (boş) loopback başlıkları hiç kabul edilmez.
func TestClientIPAnahtarYoksaFailClosed(t *testing.T) {
	loopbackGuvenilir(t)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("X-SanalCP-Proxy", "")
	if got := ClientIP(r); got != "127.0.0.1" {
		t.Fatalf("anahtar yokken ClientIP = %q, beklenen 127.0.0.1", got)
	}
}

// Ayrı makinedeki, TRUSTED_PROXY_CIDRS ile açıkça tanımlanmış yük dengeleyici
// anahtar göndermez; eski davranış korunur.
func TestClientIPLoopbackOlmayanGuvenilirVekil(t *testing.T) {
	_, lb, _ := net.ParseCIDR("10.0.0.0/8")
	SetTrustedProxies([]*net.IPNet{lb})
	t.Cleanup(func() { SetTrustedProxies([]*net.IPNet{}) })
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:443"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.1.2.3")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, beklenen 203.0.113.7", got)
	}
}

func TestClientIPGecisModu(t *testing.T) {
	loopbackGuvenilir(t)
	SetProxyAnahtari("dogru-anahtar")
	SetLoopbackAnahtarsizGuven(true)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("geçiş modunda ClientIP = %q, beklenen 203.0.113.7", got)
	}
}
