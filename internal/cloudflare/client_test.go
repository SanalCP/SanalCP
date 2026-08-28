package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return &Client{BaseURL: s.URL, HTTP: s.Client()}
}

func TestVerifyBearerVeAktifToken(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.URL.Path != "/user/tokens/verify" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"status":"active"}}`))
	})
	if err := c.Verify(context.Background(), "secret-token"); err != nil {
		t.Fatal(err)
	}
}

func TestFindZoneTamAdEslesmesi(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "example.com" {
			t.Fatalf("name query = %q", r.URL.Query().Get("name"))
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"zone12345678","name":"example.com","status":"active"}]}`))
	})
	z, err := c.FindZone(context.Background(), "token", "example.com")
	if err != nil || z.ID != "zone12345678" {
		t.Fatalf("zone=%+v err=%v", z, err)
	}
}

func TestAPIErrorsTokenSizdirmiyor(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"result":null}`))
	})
	err := c.Verify(context.Background(), "cok-gizli-token")
	if err == nil || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "cok-gizli-token") {
		t.Fatal("token hata mesajına sızdı")
	}
}

func TestRecordIslemleriZoneIleSinirli(t *testing.T) {
	var paths []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"record12345678","type":"A","name":"www.example.com","content":"192.0.2.1","ttl":1,"proxied":true,"proxiable":true}}`))
	})
	_, err := c.CreateRecord(context.Background(), "token", "zone12345678", RecordInput{Type: "A", Name: "www.example.com", Content: "192.0.2.1", TTL: 1, Proxied: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "POST /zones/zone12345678/dns_records" {
		t.Fatalf("paths=%v", paths)
	}
}
