package osfam

import (
	"strings"
	"testing"
)

// dpkg-query, KALDIRILMIŞ ama yapılandırması duran paketleri de listeler.
// Bunlar kurulu sayılırsa panel "zaten kurulu" deyip kurulumu atlar ve eksik
// paketle çalışmaya devam eder — sessiz ve teşhisi zor bir arıza.
func TestKuruluListeAyristirKaldirilmislariEler(t *testing.T) {
	cikti := strings.Join([]string{
		"installed\tnginx",
		"config-files\teski-paket",
		"not-installed\thic-kurulmamis",
		"installed\tmariadb-server",
		"", // boş satır
		"bozuk-satir-sekmesiz",
	}, "\n")

	got := KuruluListeAyristir(cikti)
	want := []string{"nginx", "mariadb-server"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

// rpm çıktısı da aynı ayrıştırıcıdan geçer (durum sütunu sabit "installed").
func TestKuruluListeAyristirRPMBicimi(t *testing.T) {
	cikti := "installed\tnginx|1.26.0|High performance web server\ninstalled\tbind|9.18|DNS server\n"
	got := KuruluListeAyristir(cikti)
	if len(got) != 2 || !strings.HasPrefix(got[0], "nginx|") {
		t.Fatalf("beklenmeyen: %v", got)
	}
}

// Arama çıktısı iki paket yöneticisinde farklı biçimde gelir; ikisi de doğru
// ayrıştırılmalı, aksi halde Paket Yöneticisi ekranı bir ailede boş görünür.
func TestAramaSatiriAyristir(t *testing.T) {
	durumlar := []struct {
		ad           string
		satir        string
		wantAd       string
		wantAciklama string
		wantOK       bool
	}{
		{"dnf mimarili", "nginx.x86_64 : A high performance web server", "nginx", "A high performance web server", true},
		{"dnf noarch", "php-pear.noarch : PHP Extension and Application Repository", "php-pear", "PHP Extension and Application Repository", true},
		{"dnf aarch64", "curl.aarch64 : A utility for transferring files", "curl", "A utility for transferring files", true},
		{"apt biçimi", "nginx - small, powerful, scalable web/proxy server", "nginx", "small, powerful, scalable web/proxy server", true},
		{"apt tireli paket adı", "php8.3-fpm - server-side, HTML-embedded scripting language (FPM-CGI binary)", "php8.3-fpm", "server-side, HTML-embedded scripting language (FPM-CGI binary)", true},
		{"başlık satırı", "=== Name Matched ===", "", "", false},
		{"metadata satırı", "Last metadata expiration check: 0:10:00 ago", "", "", false},
		{"boş", "", "", "", false},
		{"ayraçsız", "rastgele metin", "", "", false},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			ad, aciklama, ok := AramaSatiriAyristir(d.satir)
			if ok != d.wantOK {
				t.Fatalf("ok: got %v want %v", ok, d.wantOK)
			}
			if !ok {
				return
			}
			if ad != d.wantAd {
				t.Errorf("ad: got %q want %q", ad, d.wantAd)
			}
			if aciklama != d.wantAciklama {
				t.Errorf("açıklama: got %q want %q", aciklama, d.wantAciklama)
			}
		})
	}
}

// Paket adında nokta olması (php8.3-fpm) mimari eki sanılmamalı.
func TestAramaSatiriAyristirSurumluAdiBozmaz(t *testing.T) {
	ad, _, ok := AramaSatiriAyristir("php8.3-cli - command-line interpreter")
	if !ok || ad != "php8.3-cli" {
		t.Fatalf("got %q ok=%v, want php8.3-cli", ad, ok)
	}
}

func TestYukseltArgs(t *testing.T) {
	rhel := Bilgi{Aile: RHEL}
	deb := Bilgi{Aile: Debian, ID: "debian"}

	if got := strings.Join(rhel.YukseltArgs(""), " "); got != "dnf upgrade -y" {
		t.Errorf("RHEL tümü: %q", got)
	}
	if got := strings.Join(rhel.YukseltArgs("nginx"), " "); got != "dnf upgrade -y nginx" {
		t.Errorf("RHEL tek: %q", got)
	}

	// Debian'da tüm sistem için dist-upgrade gerekir: düz `upgrade` yeni
	// bağımlılık gerektiren yükseltmeleri sessizce atlar.
	tumu := strings.Join(deb.YukseltArgs(""), " ")
	if !strings.Contains(tumu, "dist-upgrade") {
		t.Errorf("Debian tümü dist-upgrade değil: %q", tumu)
	}
	// Tek pakette --only-upgrade olmazsa, kurulu olmayan bir paket YENİDEN
	// KURULURDU; yükseltme ucu paket kurma ucuna dönüşürdü.
	tek := strings.Join(deb.YukseltArgs("nginx"), " ")
	if !strings.Contains(tek, "--only-upgrade") || !strings.HasSuffix(tek, "nginx") {
		t.Errorf("Debian tek: %q", tek)
	}
	for _, args := range [][]string{deb.YukseltArgs(""), deb.YukseltArgs("nginx")} {
		if !strings.Contains(strings.Join(args, " "), "force-confold") {
			t.Errorf("yapılandırma koruma seçeneği yok: %v", args)
		}
	}
}

func TestKuruluDetayArgsAileyeGoreDegisir(t *testing.T) {
	rhel := strings.Join((Bilgi{Aile: RHEL}).KuruluDetayArgs(), " ")
	deb := strings.Join((Bilgi{Aile: Debian}).KuruluDetayArgs(), " ")
	if !strings.HasPrefix(rhel, "rpm ") {
		t.Errorf("RHEL: %q", rhel)
	}
	if !strings.HasPrefix(deb, "dpkg-query ") {
		t.Errorf("Debian: %q", deb)
	}
	// İki biçim de aynı ayrıştırıcıya girdiği için durum sütunu ŞART.
	for _, s := range []string{rhel, deb} {
		if !strings.Contains(s, "installed") && !strings.Contains(s, "db:Status-Status") {
			t.Errorf("durum sütunu yok: %q", s)
		}
	}
}
