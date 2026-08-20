package osfam

import (
	"context"
	"strings"
	"testing"
)

// Gerçek dağıtımların /etc/os-release içerikleri. Tespit hatası yanlış paket
// yöneticisini çağırmak demektir — bu yüzden hedeflenen her dağıtım burada.
const (
	osAlma10 = `NAME="AlmaLinux"
VERSION="10.0 (Purple Lion)"
ID="almalinux"
ID_LIKE="rhel centos fedora"
VERSION_ID="10.0"
PLATFORM_ID="platform:el10"`

	osAlma9 = `NAME="AlmaLinux"
VERSION="9.8 (Sapphire Caracal)"
ID="almalinux"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.8"`

	osRocky9 = `NAME="Rocky Linux"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.5"`

	osDebian13 = `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
VERSION_CODENAME=trixie
ID=debian`

	osDebian12 = `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION_CODENAME=bookworm
ID=debian`

	osUbuntu2604 = `PRETTY_NAME="Ubuntu 26.04 LTS"
NAME="Ubuntu"
VERSION_ID="26.04"
VERSION="26.04 LTS (Resolute Raccoon)"
VERSION_CODENAME=resolute
ID=ubuntu
ID_LIKE=debian
UBUNTU_CODENAME=resolute`

	osUbuntu2404 = `PRETTY_NAME="Ubuntu 24.04.1 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION_CODENAME=noble
ID=ubuntu
ID_LIKE=debian`
)

func TestAyristirAileTespiti(t *testing.T) {
	durumlar := []struct {
		ad     string
		icerik string
		aile   Aile
		id     string
		surum  string
		kodAdi string
	}{
		{"AlmaLinux 10", osAlma10, RHEL, "almalinux", "10.0", ""},
		{"AlmaLinux 9", osAlma9, RHEL, "almalinux", "9.8", ""},
		{"Rocky 9", osRocky9, RHEL, "rocky", "9.5", ""},
		{"Debian 13", osDebian13, Debian, "debian", "13", "trixie"},
		{"Debian 12", osDebian12, Debian, "debian", "12", "bookworm"},
		{"Ubuntu 26.04", osUbuntu2604, Debian, "ubuntu", "26.04", "resolute"},
		{"Ubuntu 24.04", osUbuntu2404, Debian, "ubuntu", "24.04", "noble"},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			b := Ayristir(d.icerik)
			if b.Aile != d.aile {
				t.Errorf("Aile: got %q, want %q", b.Aile, d.aile)
			}
			if b.ID != d.id {
				t.Errorf("ID: got %q, want %q", b.ID, d.id)
			}
			if b.Surum != d.surum {
				t.Errorf("Surum: got %q, want %q", b.Surum, d.surum)
			}
			if b.KodAdi != d.kodAdi {
				t.Errorf("KodAdi: got %q, want %q", b.KodAdi, d.kodAdi)
			}
		})
	}
}

// Tanınmayan bir sistemde Bilinmez dönmeli — RHEL'e DÜŞMEMELİ. Yanlış tahmin,
// olmayan bir paket yöneticisini çağırmak demektir.
func TestAyristirBilinmeyenSistemVarsaymaz(t *testing.T) {
	for _, icerik := range []string{
		"",
		"ID=arch\n",
		"NAME=\"Some Distro\"\nID=exotic\nID_LIKE=suse\n",
	} {
		if b := Ayristir(icerik); b.Aile != Bilinmez {
			t.Errorf("içerik %q -> Aile %q, Bilinmez bekleniyordu", icerik, b.Aile)
		}
	}
}

// ID bilinmiyor ama ID_LIKE tanınıyorsa aile ondan çözülmeli (türev dağıtımlar).
func TestAyristirIDLikeIleCozer(t *testing.T) {
	b := Ayristir("ID=linuxmint\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"22\"\n")
	if b.Aile != Debian {
		t.Fatalf("ID_LIKE'tan Debian çözülmedi: %q", b.Aile)
	}
}

func TestPaketYoneticiKomutlari(t *testing.T) {
	deb := Bilgi{Aile: Debian, ID: "debian", Surum: "13", KodAdi: "trixie"}
	rhel := Bilgi{Aile: RHEL, ID: "almalinux", Surum: "10.0"}

	if got := strings.Join(rhel.KurArgs("nginx"), " "); got != "dnf install -y nginx" {
		t.Errorf("RHEL kur: %q", got)
	}
	debKur := strings.Join(deb.KurArgs("nginx"), " ")
	if !strings.HasPrefix(debKur, "apt-get install -y") || !strings.HasSuffix(debKur, "nginx") {
		t.Errorf("Debian kur: %q", debKur)
	}
	// force-confold OLMADAN apt, panelin yazdığı yapılandırmaları ezme sorusu
	// sorabilir ve arka planda çalışan komut kilitlenir.
	if !strings.Contains(debKur, "force-confold") {
		t.Errorf("Debian kur komutu force-confold taşımıyor: %q", debKur)
	}

	if got := strings.Join(deb.DepoTazeleArgs(), " "); got != "apt-get update" {
		t.Errorf("Debian tazele: %q", got)
	}
	if got := strings.Join(deb.SilArgs("x"), " "); got != "apt-get remove -y x" {
		t.Errorf("Debian sil: %q", got)
	}
}

// Komut(): Debian'da apt'nin etkileşimsiz ortamı ZORUNLU — yoksa postfix gibi
// paketler soru sorup süreci sonsuza kadar bekletir.
func TestKomutDebianEtkilesimsizOrtamEkler(t *testing.T) {
	deb := Bilgi{Aile: Debian, ID: "debian"}
	cmd := deb.Komut(context.Background(), deb.KurArgs("postfix"))
	var bulundu bool
	for _, e := range cmd.Env {
		if e == "DEBIAN_FRONTEND=noninteractive" {
			bulundu = true
		}
	}
	if !bulundu {
		t.Error("DEBIAN_FRONTEND=noninteractive eklenmedi — kurulum kilitlenebilir")
	}

	rhel := Bilgi{Aile: RHEL}
	if c := rhel.Komut(context.Background(), rhel.KurArgs("nginx")); c.Env != nil {
		t.Error("RHEL komutuna gereksiz ortam eklendi")
	}
}

func TestPaketVeServisAdlari(t *testing.T) {
	deb13 := Bilgi{Aile: Debian, ID: "debian", Surum: "13", KodAdi: "trixie"}
	rhel := Bilgi{Aile: RHEL, ID: "almalinux", Surum: "10.0"}

	durumlar := []struct {
		mantiksal       string
		rhelPkt, debPkt string
		rhelSvc, debSvc string
	}{
		{PaketDNS, "bind", "bind9", "named", "bind9"},
		{PaketFTP, "pure-ftpd", "pure-ftpd-mysql", "pure-ftpd", "pure-ftpd-mysql"},
		{PaketCron, "cronie", "cron", "crond", "cron"},
		{PaketApache, "httpd", "apache2", "httpd", "apache2"},
	}
	for _, d := range durumlar {
		if got := rhel.Paket(d.mantiksal); got != d.rhelPkt {
			t.Errorf("%s RHEL paket: got %q want %q", d.mantiksal, got, d.rhelPkt)
		}
		if got := deb13.Paket(d.mantiksal); got != d.debPkt {
			t.Errorf("%s Debian paket: got %q want %q", d.mantiksal, got, d.debPkt)
		}
		if got := rhel.Servis(d.mantiksal); got != d.rhelSvc {
			t.Errorf("%s RHEL servis: got %q want %q", d.mantiksal, got, d.rhelSvc)
		}
		if got := deb13.Servis(d.mantiksal); got != d.debSvc {
			t.Errorf("%s Debian servis: got %q want %q", d.mantiksal, got, d.debSvc)
		}
	}

	// Bilinmeyen mantıksal ad olduğu gibi dönmeli.
	if got := deb13.Paket("curl"); got != "curl" {
		t.Errorf("bilinmeyen ad değişti: %q", got)
	}
}

// 🔴 Sürüm bazlı çözümlemenin asıl sınavı: valkey-server Debian 12'de YOK.
// Aileye bakıp valkey demek, Debian 12 kurulumunu kırar.
func TestCacheValkeyRedisSurumeGoreCozulur(t *testing.T) {
	durumlar := []struct {
		ad     string
		b      Bilgi
		paket  string
		servis string
	}{
		{"Debian 13", Bilgi{Aile: Debian, ID: "debian", Surum: "13", KodAdi: "trixie"}, "valkey-server", "valkey-server"},
		{"Debian 12", Bilgi{Aile: Debian, ID: "debian", Surum: "12", KodAdi: "bookworm"}, "redis-server", "redis-server"},
		{"Ubuntu 26.04", Bilgi{Aile: Debian, ID: "ubuntu", Surum: "26.04", KodAdi: "resolute"}, "valkey-server", "valkey-server"},
		{"Ubuntu 24.10", Bilgi{Aile: Debian, ID: "ubuntu", Surum: "24.10", KodAdi: "oracular"}, "valkey-server", "valkey-server"},
		// 🔴 24.04 (noble) arşivinde valkey-server YOK. Bu satır önce
		// "valkey-server" bekliyordu; yani test, hatalı varsayımı SABİTLİYORDU
		// ve bu yüzden hiçbir şey uyarmadı. Faz 5c statik ön-uçuşunda
		// archive.ubuntu.com Packages indeksine bakılarak düzeltildi.
		{"Ubuntu 24.04", Bilgi{Aile: Debian, ID: "ubuntu", Surum: "24.04", KodAdi: "noble"}, "redis-server", "redis-server"},
		{"Ubuntu 22.04", Bilgi{Aile: Debian, ID: "ubuntu", Surum: "22.04", KodAdi: "jammy"}, "redis-server", "redis-server"},
		{"AlmaLinux", Bilgi{Aile: RHEL, ID: "almalinux", Surum: "10.0"}, "valkey", "valkey"},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			if got := d.b.Paket(PaketCache); got != d.paket {
				t.Errorf("paket: got %q want %q", got, d.paket)
			}
			if got := d.b.Servis(PaketCache); got != d.servis {
				t.Errorf("servis: got %q want %q", got, d.servis)
			}
		})
	}
}

// KodAdi yoksa (minimal imaj) VERSION_ID'den karar verilebilmeli.
// Ubuntu'da sürüm YIL.AY biçiminde olduğu için sayısal karşılaştırma tek
// başına yetmez: 24.04 valkey'siz, 24.10 valkey'li — aynı yıl, farklı sonuç.
func TestCacheUbuntuKodAdiYokkaAyaBakar(t *testing.T) {
	durumlar := []struct {
		surum string
		paket string
	}{
		{"22.04", "redis-server"},
		{"24.04", "redis-server"},
		{"24.10", "valkey-server"},
		{"26.04", "valkey-server"},
	}
	for _, d := range durumlar {
		b := Bilgi{Aile: Debian, ID: "ubuntu", Surum: d.surum}
		if got := b.Paket(PaketCache); got != d.paket {
			t.Errorf("Ubuntu %s: got %q want %q", d.surum, got, d.paket)
		}
	}
}

func TestCacheKodAdiYokkaSurumdenCozer(t *testing.T) {
	b12 := Bilgi{Aile: Debian, ID: "debian", Surum: "12"}
	if got := b12.Paket(PaketCache); got != "redis-server" {
		t.Errorf("Debian 12 (kod adı yok): got %q want redis-server", got)
	}
	b13 := Bilgi{Aile: Debian, ID: "debian", Surum: "13"}
	if got := b13.Paket(PaketCache); got != "valkey-server" {
		t.Errorf("Debian 13 (kod adı yok): got %q want valkey-server", got)
	}
}

// nginx kullanıcısı yanlışsa FPM soketinin sahibi yanlış olur ve TÜM siteler
// 502 döner — bu yüzden ayrı test.
func TestWebKullanici(t *testing.T) {
	if got := (Bilgi{Aile: Debian}).WebKullanici(); got != "www-data" {
		t.Errorf("Debian: got %q want www-data", got)
	}
	if got := (Bilgi{Aile: RHEL}).WebKullanici(); got != "nginx" {
		t.Errorf("RHEL: got %q want nginx", got)
	}
}
