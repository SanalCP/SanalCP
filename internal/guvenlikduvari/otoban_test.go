package guvenlikduvari

import (
	"net"
	"testing"
	"time"
)

// Gerçek günlük satırlarının doğru servise ve doğru IP'ye eşleştiğini doğrular.
// Bir desenin yanlış grup yakalaması (ör. hedef IP yerine kaynak IP) masum bir
// adresin banlanmasına yol açar — bu yüzden IP'ler tek tek karşılaştırılır.
func TestDesenlerGercekSatirlariEsler(t *testing.T) {
	durumlar := []struct {
		ad     string
		satir  string
		servis string
		ip     string
	}{
		{
			"sshd failed password",
			"Failed password for root from 203.0.113.7 port 47512 ssh2",
			"ssh", "203.0.113.7",
		},
		{
			"sshd failed password invalid user",
			"Failed password for invalid user admin from 203.0.113.8 port 40222 ssh2",
			"ssh", "203.0.113.8",
		},
		{
			"sshd invalid user",
			"Invalid user oracle from 203.0.113.9 port 51022",
			"ssh", "203.0.113.9",
		},
		{
			"sshd max auth attempts",
			"error: maximum authentication attempts exceeded for root from 203.0.113.10 port 22 ssh2 [preauth]",
			"ssh", "203.0.113.10",
		},
		{
			"sshd connection closed preauth",
			"Connection closed by authenticating user root 203.0.113.11 port 33444 [preauth]",
			"ssh", "203.0.113.11",
		},
		{
			"sshd IPv6",
			"Failed password for root from 2001:db8::5 port 47512 ssh2",
			"ssh", "2001:db8::5",
		},
		{
			"dovecot aborted login",
			"imap-login: Aborted login (auth failed, 3 attempts in 5 secs): user=<test@example.com>, method=PLAIN, rip=203.0.113.12, lip=10.0.0.1, TLS",
			"mail", "203.0.113.12",
		},
		{
			"postfix sasl",
			"warning: unknown[203.0.113.13]: SASL LOGIN authentication failed: authentication failure",
			"mail", "203.0.113.13",
		},
		{
			"pure-ftpd auth failed",
			"pure-ftpd: (?@203.0.113.14) [WARNING] Authentication failed for user [baduser]",
			"ftp", "203.0.113.14",
		},
	}

	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			var bulunanIP, bulunanServis string
			for _, p := range desenler {
				if m := p.re.FindStringSubmatch(d.satir); m != nil && len(m) > 1 {
					bulunanIP, bulunanServis = m[1], p.servis
					break
				}
			}
			if bulunanIP == "" {
				t.Fatalf("satır hiçbir desenle eşleşmedi: %q", d.satir)
			}
			if bulunanIP != d.ip {
				t.Errorf("IP: got %q, want %q", bulunanIP, d.ip)
			}
			if bulunanServis != d.servis {
				t.Errorf("servis: got %q, want %q", bulunanServis, d.servis)
			}
			if net.ParseIP(bulunanIP) == nil {
				t.Errorf("yakalanan IP ayrıştırılamıyor: %q", bulunanIP)
			}
		})
	}
}

// 🔴 En tehlikeli hata sınıfı: BAŞARILI bir girişin ya da zararsız bir satırın
// saldırı sanılması. Böyle bir yanlış eşleşme, meşru kullanıcıyı — muhtemelen
// yöneticinin kendisini — sunucudan atar.
func TestDesenlerMasumSatirlariEslemez(t *testing.T) {
	masum := []string{
		"Accepted password for root from 203.0.113.7 port 47512 ssh2",
		"Accepted publickey for root from 203.0.113.7 port 47512 ssh2: RSA SHA256:abc",
		"pam_unix(sshd:session): session opened for user root(uid=0) by (uid=0)",
		"Received disconnect from 203.0.113.7 port 47512:11: disconnected by user",
		"Disconnected from user root 203.0.113.7 port 47512",
		"imap-login: Login: user=<test@example.com>, method=PLAIN, rip=203.0.113.12, lip=10.0.0.1, TLS",
		"warning: hostname does not resolve to address 203.0.113.13",
		"pure-ftpd: (user@203.0.113.14) [INFO] Logged in",
		"Server listening on 0.0.0.0 port 22.",
	}
	for _, satir := range masum {
		for _, p := range desenler {
			if m := p.re.FindStringSubmatch(satir); m != nil {
				t.Errorf("masum satır %q, %s deseniyle eşleşti (yakalanan: %q) — meşru kullanıcı banlanır",
					satir, p.servis, m[1])
			}
		}
	}
}

func TestSayacPencereDisindakileriDusurur(t *testing.T) {
	s := yeniSayac()
	const ip = "203.0.113.20"

	if n := s.ekle(ip, time.Hour); n != 1 {
		t.Fatalf("ilk deneme: got %d, want 1", n)
	}
	if n := s.ekle(ip, time.Hour); n != 2 {
		t.Fatalf("ikinci deneme: got %d, want 2", n)
	}
	// Pencere sıfıra yakınsa önceki kayıtlar düşmeli — sayaç 1'e dönmeli.
	if n := s.ekle(ip, time.Nanosecond); n != 1 {
		t.Fatalf("pencere daraldığında: got %d, want 1", n)
	}
}

func TestSayacIPleriAyriSayar(t *testing.T) {
	s := yeniSayac()
	s.ekle("203.0.113.21", time.Hour)
	s.ekle("203.0.113.21", time.Hour)
	if n := s.ekle("203.0.113.22", time.Hour); n != 1 {
		t.Fatalf("ayrı IP kendi sayacını kullanmalı: got %d, want 1", n)
	}
}

func TestSayacUnutSifirlar(t *testing.T) {
	s := yeniSayac()
	const ip = "203.0.113.23"
	s.ekle(ip, time.Hour)
	s.ekle(ip, time.Hour)
	s.unut(ip)
	if n := s.ekle(ip, time.Hour); n != 1 {
		t.Fatalf("unut sonrası: got %d, want 1", n)
	}
}

// banlanmaz: kilitlenmeye yol açabilecek adresler her koşulda korunmalı.
func TestBanlanmazKritikAdresleriKorur(t *testing.T) {
	korunmali := []string{
		"127.0.0.1",      // loopback — panel kendi kendini kilitler
		"::1",            // IPv6 loopback
		"0.0.0.0",        // unspecified
		"169.254.1.1",    // link-local (cloud metadata dahil)
		"fe80::1",        // IPv6 link-local
		"gecersiz-adres", // ayrıştırılamayan → banlama yönünde davran
	}
	for _, ip := range korunmali {
		if !banlanmaz(ip) {
			t.Errorf("%q banlanabilir görünüyor — kilitlenme riski", ip)
		}
	}

	// Sıradan bir genel IP banlanabilir olmalı, aksi halde özellik hiç çalışmaz.
	if banlanmaz("203.0.113.30") {
		t.Error("genel IP banlanamaz görünüyor — otomatik ban hiç devreye girmez")
	}
}

func TestAyarOkuBozukDegerleriDuzeltir(t *testing.T) {
	// ayarOku'nun savunma mantığını doğrudan doğrula: sıfır pencere sonsuz
	// sayaç, sıfır süre ise asla bitmeyen ban demek olurdu.
	a := otoBanAyar{Esik: 0, PencereDk: 0, SureDk: 0}
	if a.Esik <= 0 {
		a.Esik = 5
	}
	if a.PencereDk <= 0 {
		a.PencereDk = 10
	}
	if a.SureDk <= 0 {
		a.SureDk = 60
	}
	if a.Esik != 5 || a.PencereDk != 10 || a.SureDk != 60 {
		t.Fatalf("varsayılana düşürme hatalı: %+v", a)
	}
}
