package domains

import "testing"

func TestProxyHedefDogrula(t *testing.T) {
	ws := true
	r := createReq{SiteTipi: "reverse_proxy", ProxyScheme: "http", ProxyHost: "localhost", ProxyPort: 3000, ProxyWebSocket: &ws}
	if err := proxyHedefDogrula(&r); err != nil {
		t.Fatalf("geçerli hedef reddedildi: %v", err)
	}
	if r.ProxyHost != "127.0.0.1" {
		t.Fatalf("localhost kanonikleşmedi: %q", r.ProxyHost)
	}
	for _, tc := range []createReq{
		{SiteTipi: "reverse_proxy", ProxyScheme: "ftp", ProxyHost: "127.0.0.1", ProxyPort: 3000},
		{SiteTipi: "reverse_proxy", ProxyScheme: "http", ProxyHost: "10.0.0.1", ProxyPort: 3000},
		{SiteTipi: "reverse_proxy", ProxyScheme: "http", ProxyHost: "127.0.0.1", ProxyPort: 8080},
	} {
		if err := proxyHedefDogrula(&tc); err == nil {
			t.Fatalf("tehlikeli hedef kabul edildi: %+v", tc)
		}
	}
}
