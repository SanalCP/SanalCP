package backups

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
)

func TestIPYasakli(t *testing.T) {
	yasakli := []string{
		"127.0.0.1", "127.0.0.53", "::1",
		"10.0.0.5", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", // cloud metadata
		"169.254.0.1", "fe80::1",
		"0.0.0.0",
		"224.0.0.1", // multicast
	}
	for _, s := range yasakli {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test verisi geçersiz IP: %s", s)
		}
		if !ipYasakli(ip) {
			t.Errorf("ipYasakli(%s) = false, want true (yasaklı olmalı)", s)
		}
	}

	serbest := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34"}
	for _, s := range serbest {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test verisi geçersiz IP: %s", s)
		}
		if ipYasakli(ip) {
			t.Errorf("ipYasakli(%s) = true, want false (genel/public adres)", s)
		}
	}
}

func TestHostGuvenliMiIPLiteral(t *testing.T) {
	ctx := context.Background()
	for _, h := range []string{"127.0.0.1", "169.254.169.254", "10.1.2.3", "::1"} {
		if err := hostGuvenliMi(ctx, h); err == nil {
			t.Errorf("hostGuvenliMi(%s) = nil, want SSRF hatası", h)
		}
	}
	for _, h := range []string{"8.8.8.8", "1.1.1.1"} {
		if err := hostGuvenliMi(ctx, h); err != nil {
			t.Errorf("hostGuvenliMi(%s) = %v, want nil (genel adres)", h, err)
		}
	}
}

func TestFtpNetrcYaz(t *testing.T) {
	path, err := ftpNetrcYaz("ftp.example.com", `user"with\special`, `pass with space`)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("netrc dosya izni = %o, want 0600", perm)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	if !strings.Contains(got, "machine ftp.example.com") {
		t.Errorf("netrc içeriğinde machine satırı yok: %q", got)
	}
	if !strings.Contains(got, `login "user\"with\\special"`) {
		t.Errorf("netrc login escape'i beklenmedik: %q", got)
	}
	if !strings.Contains(got, `password "pass with space"`) {
		t.Errorf("netrc password satırı beklenmedik: %q", got)
	}
	// Parola argv'ye değil, sadece dosyaya gitmeli — bu test dosyanın
	// gerçekten yazıldığını doğrular; testConnection'daki --netrc-file
	// kullanımı parolayı process argümanından tamamen çıkarır.
}

func TestKnownHostsDosyaYaz(t *testing.T) {
	keys := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG8FaGZiTazxyRUYEgvX/Fi8OYSr5M+Hfn3JDN32K8B3"
	path, err := knownHostsDosyaYaz(keys)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("known_hosts dosya izni = %o, want 0600", perm)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != keys {
		t.Errorf("known_hosts içeriği = %q, want %q", content, keys)
	}
}
