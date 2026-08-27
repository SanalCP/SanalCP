package nginxset

import "testing"

func TestProxyAyarDogrula(t *testing.T) {
	p := ProxyAyar{Scheme: "HTTP", Host: "localhost", Port: 3000, WebSocket: true}
	if err := proxyAyarDogrula(&p); err != nil {
		t.Fatalf("geçerli hedef reddedildi: %v", err)
	}
	if p.Scheme != "http" || p.Host != "127.0.0.1" {
		t.Fatalf("kanonik hedef yanlış: %+v", p)
	}
	for _, x := range []ProxyAyar{
		{Scheme: "file", Host: "127.0.0.1", Port: 3000},
		{Scheme: "http", Host: "192.168.1.1", Port: 3000},
		{Scheme: "http", Host: "127.0.0.1", Port: 8080},
	} {
		if err := proxyAyarDogrula(&x); err == nil {
			t.Fatalf("geçersiz hedef kabul edildi: %+v", x)
		}
	}
}
