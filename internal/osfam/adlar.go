package osfam

// Paket ve servis adı çözümlemesi.
//
// Kod içinde MANTIKSAL adlar kullanılır ("dns", "cache", "ftp"); gerçek paket ve
// systemd unit adları burada çözülür. Böylece yeni bir dağıtım eklemek, çağrı
// yerlerine dokunmadan bu tabloları genişletmek demektir.
//
// ÇÖZÜMLEME AİLEYE + SÜRÜME BAKAR. Somut örnek: valkey-server Debian 13 ve tüm
// hedef Ubuntu sürümlerinde var, ama Debian 12'de YOK — orada redis-server'a
// düşülür. Bu tür durumlar `ozelPaket` içinde, gerekçesiyle birlikte toplanır.

import "strings"

// Mantıksal ad sabitleri — yazım hatası derleme zamanında yakalansın diye.
const (
	PaketWeb       = "web"
	PaketWebEkstra = "web-ekstra" // htpasswd vb. araçlar
	PaketDB        = "db"
	PaketDNS       = "dns"
	PaketDNSArac   = "dns-araclari"
	PaketFTP       = "ftp"
	PaketCache     = "cache"
	PaketAntivirus = "antivirus"
	PaketAVGuncel  = "antivirus-guncelleme"
	PaketCron      = "cron"
	PaketKotaXFS   = "kota-xfs" // XFS kota araçları (xfs_quota)
	PaketKotaExt   = "kota-ext" // ext2/3/4 kota araçları (setquota/repquota/quotaon)
	PaketApache    = "apache"
	PaketApacheAra = "apache-araclari"
	PaketACL       = "acl"
	PaketSSH       = "ssh"
	PaketBsdtar    = "bsdtar"
)

// paketAdlari: mantıksal ad -> aileye göre gerçek paket adı.
var paketAdlari = map[string]map[Aile]string{
	PaketWeb:       {RHEL: "nginx", Debian: "nginx"},
	PaketDB:        {RHEL: "mariadb-server", Debian: "mariadb-server"},
	PaketDNS:       {RHEL: "bind", Debian: "bind9"},
	PaketDNSArac:   {RHEL: "bind-utils", Debian: "bind9-utils"},
	PaketFTP:       {RHEL: "pure-ftpd", Debian: "pure-ftpd-mysql"},
	PaketCache:     {RHEL: "valkey", Debian: "valkey-server"}, // bkz. ozelPaket
	PaketAntivirus: {RHEL: "clamav", Debian: "clamav-daemon"},
	PaketAVGuncel:  {RHEL: "clamav-update", Debian: "clamav-freshclam"},
	PaketCron:      {RHEL: "cronie", Debian: "cron"},
	// Kota backend'i AİLEYE DEĞİL, kök dosya sisteminin tipine göre seçilir
	// (bkz. internal/kaynaklimit/kota.go) — AlmaLinux ext4 köke de kurulabilir,
	// Debian XFS köke de. Bu yüzden her iki araç seti de her iki ailede kurulur;
	// paket adları zaten iki ailede de aynıdır.
	PaketKotaXFS:   {RHEL: "xfsprogs", Debian: "xfsprogs"},
	PaketKotaExt:   {RHEL: "quota", Debian: "quota"},
	PaketApache:    {RHEL: "httpd", Debian: "apache2"},
	PaketApacheAra: {RHEL: "httpd-tools", Debian: "apache2-utils"},
	PaketWebEkstra: {RHEL: "httpd-tools", Debian: "apache2-utils"},
	PaketACL:       {RHEL: "acl", Debian: "acl"},
	PaketSSH:       {RHEL: "openssh-server", Debian: "openssh-server"},
	// bsdtar RHEL'de kendi adıyla, Debian'da libarchive-tools içinde gelir.
	PaketBsdtar: {RHEL: "bsdtar", Debian: "libarchive-tools"},
}

// servisAdlari: mantıksal ad -> aileye göre systemd unit adı.
//
// Not: named-checkconf / named-checkzone İKİLİLERİ her iki ailede de aynı adı
// taşır (bind9 paketi onları da kurar); yalnız SERVİS adı farklıdır.
var servisAdlari = map[string]map[Aile]string{
	PaketWeb:       {RHEL: "nginx", Debian: "nginx"},
	PaketDB:        {RHEL: "mariadb", Debian: "mariadb"},
	PaketDNS:       {RHEL: "named", Debian: "bind9"},
	PaketFTP:       {RHEL: "pure-ftpd", Debian: "pure-ftpd-mysql"},
	PaketCache:     {RHEL: "valkey", Debian: "valkey-server"}, // bkz. ozelServis
	PaketAntivirus: {RHEL: "clamd@scan", Debian: "clamav-daemon"},
	PaketAVGuncel:  {RHEL: "clamav-freshclam", Debian: "clamav-freshclam"},
	PaketCron:      {RHEL: "crond", Debian: "cron"},
	PaketApache:    {RHEL: "httpd", Debian: "apache2"},
	// 🔴 SSH birimi Debian'da "ssh.service"tir. "sshd.service" yalnız bir
	// ALIAS'tır ve journald kayıtları gerçek unit adıyla tutulduğu için
	// `journalctl -u sshd.service` Debian'da BOŞ döner.
	PaketSSH: {RHEL: "sshd", Debian: "ssh"},
}

// ozelPaket: aile tablosunun yetmediği, dağıtım SÜRÜMÜNE bağlı durumlar.
// Boş string dönerse tablo değeri kullanılır.
func ozelPaket(b Bilgi, mantiksal string) string {
	// Valkey, Redis'ten 2024'te çatallandı; Debian 12 (bookworm) 2023'te
	// yayınlandığı için valkey-server bookworm'da YOKTUR. Doğrulandı:
	// packages.debian.org/bookworm/valkey-server -> "Package not available".
	// Debian 13, Ubuntu 24.04/25.10/26.04'te mevcut.
	if mantiksal == PaketCache && b.Aile == Debian && bookwormVeyaOncesi(b) {
		return "redis-server"
	}
	return ""
}

// ozelServis: ozelPaket'in servis karşılığı.
func ozelServis(b Bilgi, mantiksal string) string {
	if mantiksal == PaketCache && b.Aile == Debian && bookwormVeyaOncesi(b) {
		return "redis-server"
	}
	return ""
}

// bookwormVeyaOncesi: Debian 12 ve öncesi mi? Ubuntu için daima false —
// hedeflenen tüm Ubuntu sürümlerinde valkey mevcut.
//
// KodAdi güvenilir olduğunda ona bakılır; boşsa VERSION_ID'ye düşülür (bazı
// minimal imajlarda VERSION_CODENAME bulunmayabilir).
func bookwormVeyaOncesi(b Bilgi) bool {
	if b.ID == "ubuntu" {
		return false
	}
	switch b.KodAdi {
	case "bookworm", "bullseye", "buster", "stretch":
		return true
	case "trixie", "forky", "sid":
		return false
	}
	// KodAdi yok: VERSION_ID sayısal karşılaştırması ("12", "12.5" -> 12)
	ana := b.Surum
	if i := strings.IndexByte(ana, '.'); i > 0 {
		ana = ana[:i]
	}
	switch ana {
	case "9", "10", "11", "12":
		return true
	}
	return false
}

// Paket: mantıksal ada karşılık gelen gerçek paket adı. Bilinmeyen mantıksal ad
// olduğu gibi döner (çağıran zaten gerçek bir ad vermiş olabilir).
func Paket(mantiksal string) string { return Mevcut().Paket(mantiksal) }

func (b Bilgi) Paket(mantiksal string) string {
	if ozel := ozelPaket(b, mantiksal); ozel != "" {
		return ozel
	}
	if m, ok := paketAdlari[mantiksal]; ok {
		if ad, ok := m[b.Aile]; ok {
			return ad
		}
	}
	return mantiksal
}

// Servis: mantıksal ada karşılık gelen systemd unit adı.
func Servis(mantiksal string) string { return Mevcut().Servis(mantiksal) }

func (b Bilgi) Servis(mantiksal string) string {
	if ozel := ozelServis(b, mantiksal); ozel != "" {
		return ozel
	}
	if m, ok := servisAdlari[mantiksal]; ok {
		if ad, ok := m[b.Aile]; ok {
			return ad
		}
	}
	return mantiksal
}

// WebKullanici: nginx'in çalıştığı sistem kullanıcısı.
//
// PHP-FPM havuzlarındaki listen.owner/listen.group bu değeri kullanır — yanlış
// olursa nginx FPM soketine bağlanamaz ve TÜM siteler 502 döner.
func WebKullanici() string { return Mevcut().WebKullanici() }

func (b Bilgi) WebKullanici() string {
	if b.DebianMi() {
		return "www-data"
	}
	return "nginx"
}

// ApacheBackendDestekli: panelin opsiyonel Apache backend'i bu sistemde
// kullanılabilir mi?
//
// v1'de Debian'da KAPALI: Apache yapılandırma düzeni (sites-available +
// a2ensite, farklı modül adları) RHEL'dekinden tamamen farklı ve kullanım oranı
// düşük. Yarım çalışan bir backend yerine dürüstçe kapatılıyor.
func ApacheBackendDestekli() bool { return Mevcut().RHELMi() }

// GuvenlikGuncellemeDestekli: CVE / güvenlik güncellemesi ekranı çalışır mı?
//
// v1'de Debian'da KAPALI: RHEL'in `dnf updateinfo --security` çıktısının
// Debian'da doğrudan karşılığı yok (apt tarafında ayrı bir veri kaynağı ve
// ayrıştırma gerekir). Yanlış "0 açık" göstermektense ekran kapatılıyor.
func GuvenlikGuncellemeDestekli() bool { return Mevcut().RHELMi() }
