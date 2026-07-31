package backups

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestS3RequestURLPathStyle(t *testing.T) {
	d := &Destination{
		Tip: "b2", Endpoint: "https://s3.us-west-004.backblazeb2.com",
		Bucket: "firma-yedekleri", UzakDizin: "/sanalcp/example.com/", PathStyle: true,
	}
	u, err := s3RequestURL(d, "backup 01.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://s3.us-west-004.backblazeb2.com/firma-yedekleri/sanalcp/example.com/backup%2001.tar.gz"
	if u.String() != want {
		t.Fatalf("URL = %q, want %q", u.String(), want)
	}
}

func TestS3UploadDownloadDelete(t *testing.T) {
	var methods []string
	var stored []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Errorf("istek imzalanmamış")
		}
		switch r.Method {
		case http.MethodPut:
			stored, _ = io.ReadAll(r.Body)
		case http.MethodGet:
			_, _ = w.Write(stored)
		case http.MethodHead, http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	oldClient := s3HTTPClient
	s3HTTPClient = server.Client()
	defer func() { s3HTTPClient = oldClient }()

	d := &Destination{
		Tip: "s3", Endpoint: server.URL, Bucket: "test-bucket", Region: "test-1",
		Kullanici: "access", Parola: "secret", UzakDizin: "domain", PathStyle: true, Aktif: true,
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tar.gz")
	target := filepath.Join(dir, "target.tar.gz")
	if err := os.WriteFile(source, []byte("backup-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := testS3Connection(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := uploadS3Object(ctx, d, source, "one.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := downloadS3Object(ctx, d, "one.tar.gz", target); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "backup-data" {
		t.Fatalf("indirilen veri = %q", got)
	}
	if err := deleteS3Object(ctx, d, "one.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "HEAD,PUT,GET,DELETE" {
		t.Fatalf("metotlar = %v", methods)
	}
}

func TestS3RequestURLVirtualHost(t *testing.T) {
	d := &Destination{
		Tip: "s3", Region: "eu-central-1", Bucket: "firma-yedekleri",
		UzakDizin: "daily", PathStyle: false,
	}
	u, err := s3RequestURL(d, "backup.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://firma-yedekleri.s3.eu-central-1.amazonaws.com/daily/backup.tar.gz"
	if u.String() != want {
		t.Fatalf("URL = %q, want %q", u.String(), want)
	}
}

func TestS3EndpointRejectsInsecureAndRedirectProneValues(t *testing.T) {
	cases := []string{
		"http://s3.example.com",
		"https://user:pass@s3.example.com",
		"https://s3.example.com?target=other",
	}
	for _, endpoint := range cases {
		_, err := s3Endpoint(&Destination{Tip: "s3", Endpoint: endpoint})
		if err == nil {
			t.Errorf("endpoint %q kabul edildi", endpoint)
		}
	}
}

func TestSignS3RequestDeterministic(t *testing.T) {
	req, err := http.NewRequest(http.MethodHead,
		"https://example-bucket.s3.us-east-1.amazonaws.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	d := &Destination{
		Kullanici: "AKIDEXAMPLE",
		Parola:    "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		Region:    "us-east-1",
	}
	signS3Request(req, d, emptySHA256, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth,
		"AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20130524/us-east-1/s3/aws4_request, ") {
		t.Fatalf("beklenmeyen authorization: %s", auth)
	}
	if req.Header.Get("X-Amz-Date") != "20130524T000000Z" {
		t.Fatalf("x-amz-date = %q", req.Header.Get("X-Amz-Date"))
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("imzalı header listesi eksik: %s", auth)
	}
}
