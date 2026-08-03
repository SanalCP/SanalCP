package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func TestSunucuAdlariWWWYonlendir(t *testing.T) {
	o := VhostOpts{AlanAdi: "example.com"}
	if got := o.SunucuAdlari(); got != "example.com www.example.com" {
		t.Errorf("WWWYonlendir kapalıyken beklenmeyen server_name: %q", got)
	}
	o.WWWYonlendir = true
	if got := o.SunucuAdlari(); got != "www.example.com" {
		t.Errorf("WWWYonlendir açıkken beklenmeyen server_name: %q", got)
	}
}

func TestWWWRedirectTmplSSLli(t *testing.T) {
	o := VhostOpts{
		AlanAdi:      "example.com",
		WWWYonlendir: true,
		CertPath:     "/home/c_example/ssl/example.com.crt",
		KeyPath:      "/home/c_example/ssl/example.com.key",
	}
	var buf bytes.Buffer
	if err := wwwRedirectTmpl.Execute(&buf, o); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "server_name example.com;") {
		t.Errorf("apex server_name eksik:\n%s", out)
	}
	if !strings.Contains(out, "return 301 https://www.example.com$request_uri;") {
		t.Errorf("301 yönlendirme satırı eksik:\n%s", out)
	}
	if !strings.Contains(out, "location /.well-known/acme-challenge/") {
		t.Errorf("acme-challenge location'ı eksik (LE yenilemesi kırılır):\n%s", out)
	}
	if !strings.Contains(out, "listen 443 ssl;") {
		t.Errorf("SSL aktifken 443 bloğu eksik:\n%s", out)
	}
}

func TestWWWRedirectTmplSSLsiz(t *testing.T) {
	o := VhostOpts{AlanAdi: "example.com", WWWYonlendir: true}
	var buf bytes.Buffer
	if err := wwwRedirectTmpl.Execute(&buf, o); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "listen 443") {
		t.Errorf("SSL yokken 443 bloğu render edilmemeli:\n%s", out)
	}
	// SSL yokken sertifikasız bir https:// hedefine yönlendirmek TARAYICI HATASI
	// üretir — hedef http:// olmalı.
	if !strings.Contains(out, "return 301 http://www.example.com$request_uri;") {
		t.Errorf("SSL yokken hedef http:// olmalı:\n%s", out)
	}
	if strings.Contains(out, "https://www.example.com") {
		t.Errorf("SSL yokken sertifikasız https hedefi üretilmemeli:\n%s", out)
	}
}

func TestVhostTmplWWWYonlendirApexIcermez(t *testing.T) {
	o := VhostOpts{
		AlanAdi:      "example.com",
		WebRoot:      "/home/c_example/public_html",
		PHPSocket:    "/run/php-fpm/c_example.sock",
		WWWYonlendir: true,
	}
	var buf bytes.Buffer
	if err := vhostTmpl.Execute(&buf, o); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "server_name www.example.com;") {
		t.Errorf("ana vhost www-only server_name içermeli:\n%s", out)
	}
	if strings.Contains(out, "server_name www.example.com example.com") ||
		strings.Contains(out, "server_name example.com www.example.com") {
		t.Errorf("ana vhost apex'i de içermemeli (ayrı bloktan yönlendirilir):\n%s", out)
	}
}
