package dns

import "sanalcp/internal/osfam"

// BIND dosya yolları, çalışma kullanıcısı ve servis adı — dağıtım ailesine göre.
//
// NEDEN BURADA: `osfam` yalnız "işletim sistemi gerçeklerini" bilir (paket ve
// systemd unit adları). BIND'in hangi dizine zone yazdığı UYGULAMA bilgisidir;
// osfam'ın paket açıklamasındaki tasarım ilkesi gereği onu bilen paket tutar.
//
// 🔴 Debian yolları keyfi seçilmedi. bind9 paketi bir AppArmor profiliyle gelir
// (/etc/apparmor.d/usr.sbin.named) ve named'in YAZABİLECEĞİ dizinler o profille
// sınırlıdır: /var/lib/bind ve /var/cache/bind yazılabilir tanımlıdır,
// /var/named DEĞİLDİR. RHEL yollarını Debian'da kullanmak, zone dosyası diske
// yazılsa bile named'in onu okumasının/yazmasının AppArmor tarafından sessizce
// reddedilmesi demekti. Aynı sebeple include dosyası /etc/bind altındadır
// (profil /etc/bind/** okumasına izin verir).
//
// DNSSEC anahtar dizini her iki ailede de zone dizininin altındadır: RHEL'de
// /var/named/dynamic zaten named_cache_t etiketli (ekstra SELinux ayarı
// gerektirmez), Debian'da /var/lib/bind/dynamic AppArmor'ın yazılabilir
// ağacının içinde kalır.
var (
	ZoneDir          = "/var/named"
	NamedConfInclude = "/etc/named/sanalcp-zones.conf"
	DNSSECKeyDir     = "/var/named/dynamic"

	// dnsKullanici: zone dosyalarının sahibi (named'in okuyabilmesi için).
	dnsKullanici = "named"
	// dnsServis: `systemctl reload/restart` için systemd unit adı.
	dnsServis = "named"
)

// Yollar: bir dağıtım için BIND yerleşimi. Saf veri — test edilebilsin diye
// ayrı tip.
type Yollar struct {
	ZoneDir      string
	Include      string
	DNSSECKeyDir string
	Kullanici    string
	Servis       string
}

// YollarSec: aileye göre BIND yerleşimi. SAF FONKSİYON — dosya sistemi
// gerektirmez, birim test edilebilir.
func YollarSec(b osfam.Bilgi) Yollar {
	if b.DebianMi() {
		return Yollar{
			ZoneDir:      "/var/lib/bind",
			Include:      "/etc/bind/sanalcp-zones.conf",
			DNSSECKeyDir: "/var/lib/bind/dynamic",
			Kullanici:    "bind",
			Servis:       b.Servis(osfam.PaketDNS), // "bind9"
		}
	}
	return Yollar{
		ZoneDir:      "/var/named",
		Include:      "/etc/named/sanalcp-zones.conf",
		DNSSECKeyDir: "/var/named/dynamic",
		Kullanici:    "named",
		Servis:       b.Servis(osfam.PaketDNS), // "named"
	}
}

func init() {
	y := YollarSec(osfam.Mevcut())
	ZoneDir, NamedConfInclude, DNSSECKeyDir = y.ZoneDir, y.Include, y.DNSSECKeyDir
	dnsKullanici, dnsServis = y.Kullanici, y.Servis
}
