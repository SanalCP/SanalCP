package appruntime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHealthProbeRequires2xxAndUsesTenantHost(t *testing.T) {
	for _, tc := range []struct {
		status  int
		healthy bool
	}{{200, true}, {204, true}, {302, false}, {500, false}} {
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Host != "example.com" || r.URL.Host != "127.0.0.1:3000" || r.URL.Path != "/health" {
				t.Errorf("unexpected health request: host=%q url=%q", r.Host, r.URL.String())
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})}
		err := probeHTTPClient(context.Background(), client, "example.com", 3000, "/health")
		if (err == nil) != tc.healthy {
			t.Errorf("status=%d, err=%v", tc.status, err)
		}
	}
}

func TestEntryPathAllowlist(t *testing.T) {
	for _, path := range []string{"public_html/server.js", "public_html/app/main.mjs", "public_html/app.py"} {
		if !entryRE.MatchString(path) || strings.Contains(path, "..") {
			t.Errorf("geçerli yol reddedildi: %q", path)
		}
	}
	for _, path := range []string{"/etc/passwd", "public_html/../server.js", "public_html/link/../../etc/passwd", "public_html/app.py\nExecStart=/bin/sh", "public_html/server.sh"} {
		if entryRE.MatchString(path) && !strings.Contains(path, "..") {
			t.Errorf("güvensiz yol kabul edildi: %q", path)
		}
	}
}

func TestUnitTenantIdentityAndLimits(t *testing.T) {
	s := site{id: 7, domain: "example.com", sk: "c_example_com", backend: "reverse-proxy"}
	c := Config{Runtime: "node", Entrypoint: "public_html/server.js", Port: 3000, Enabled: true}
	body := unit(s, c, "/usr/bin/node", "/home/c_example_com/public_html/server.js")
	for _, want := range []string{
		"User=c_example_com", "Group=c_example_com", "Slice=sanal-c_example_com.slice",
		"ExecStart=/usr/bin/node /home/c_example_com/public_html/server.js", "Environment=PORT=3000",
		"Environment=HOST=127.0.0.1", "NoNewPrivileges=true", "ProtectSystem=strict",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("servis dosyasında %q eksik", want)
		}
	}
}

func TestSuspendedAndHTTPSProxyCannotStart(t *testing.T) {
	c := Config{Runtime: "node", Entrypoint: "public_html/server.js", Port: 3000, Enabled: true}
	for _, s := range []site{
		{backend: "reverse-proxy", proxyScheme: "http", proxyPort: 3000, suspended: true},
		{backend: "reverse-proxy", proxyScheme: "https", proxyPort: 3000},
	} {
		if _, _, err := validate(s, c); err == nil {
			t.Errorf("site uygulama başlatmayı reddetmeli: %+v", s)
		}
	}
}
