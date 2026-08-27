package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func TestVhostReverseProxyWebSocket(t *testing.T) {
	o := VhostOpts{AlanAdi: "app.example.com", WebRoot: "/home/app/public_html", Backend: "reverse-proxy",
		ProxyScheme: "http", ProxyHost: "127.0.0.1", ProxyPort: 3000, ProxyWebSocket: true}
	var b bytes.Buffer
	if err := vhostTmpl.Execute(&b, o); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, want := range []string{"proxy_pass http://127.0.0.1:3000;", "proxy_http_version 1.1;", "proxy_set_header Upgrade $http_upgrade;", "X-Forwarded-For $remote_addr;"} {
		if !strings.Contains(s, want) {
			t.Errorf("vhost eksik: %s", want)
		}
	}
	if strings.Contains(s, "fastcgi_pass") {
		t.Error("reverse proxy vhost PHP-FPM içermemeli")
	}
}
