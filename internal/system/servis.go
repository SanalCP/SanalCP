package system

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/osfam"
)

// Servis yönetimi: Genel Ayarlar'dan izin verilen servisleri restart/reload etme.
// Güvenlik: SADECE allowlist'teki birimler; keyfi systemctl çalıştırılamaz.

type servisTanim struct {
	Birim  string `json:"birim"`  // systemd unit adı
	Etiket string `json:"etiket"` // UI etiketi
	Grup   string `json:"grup"`   // UI kategorisi
	Reload bool   `json:"reload"` // reload destekliyor mu
}

// servisAllow: açılışta bir kez, çalışan dağıtımın unit adlarıyla kurulur.
//
// 🔴 Unit adları aileye göre değişir (named/bind9, crond/cron, httpd/apache2,
// php-fpm/php8.3-fpm). Sabit RHEL adlarıyla bırakılırsa Debian'da her satır
// "absent" görünür ve UI'dan hiçbir servis yeniden başlatılamaz.
var servisAllow = servisAllowSec(osfam.Mevcut())

func servisAllowSec(b osfam.Bilgi) []servisTanim {
	l := []servisTanim{
		{b.Servis(osfam.PaketWeb), "Nginx", "Web Sunucusu", true},
	}
	// Apache backend Debian'da v1'de kapalı (osfam.ApacheBackendDestekli) —
	// yeniden başlatılabilir servisler listesinde de görünmemeli.
	if b.RHELMi() {
		l = append(l, servisTanim{b.Servis(osfam.PaketApache), "Apache (Backend)", "Web Sunucusu", true})
	}
	l = append(l,
		servisTanim{b.Servis(osfam.PaketDB), "MariaDB", "Veritabanı & Önbellek", false},
		servisTanim{b.Servis(osfam.PaketCache), "Valkey (Redis)", "Veritabanı & Önbellek", false},
		servisTanim{b.Servis(osfam.PaketDNS), "BIND", "DNS", true},
	)
	// PHP-FPM birimleri: sury'de "php8.3-fpm", Remi/AppStream'de "php-fpm" +
	// "php82-php-fpm" kalıbı (bkz. provisioner.phpMap).
	if b.DebianMi() {
		l = append(l,
			servisTanim{"php8.3-fpm", "PHP-FPM 8.3", "PHP-FPM", true},
			servisTanim{"php8.2-fpm", "PHP-FPM 8.2", "PHP-FPM", true},
			servisTanim{"php7.4-fpm", "PHP-FPM 7.4", "PHP-FPM", true},
		)
	} else {
		l = append(l,
			servisTanim{"php-fpm", "PHP-FPM 8.3", "PHP-FPM", true},
			servisTanim{"php82-php-fpm", "PHP-FPM 8.2", "PHP-FPM", true},
			servisTanim{"php74-php-fpm", "PHP-FPM 7.4", "PHP-FPM", true},
		)
	}
	return append(l,
		servisTanim{b.Servis(osfam.PaketFTP), "Pure-FTPd (FTP)", "Diğer", false},
		servisTanim{b.Servis(osfam.PaketCron), "Cron (Zamanlayıcı)", "Diğer", false},
	)
}

func tanimBul(birim string) (servisTanim, bool) {
	for _, s := range servisAllow {
		if s.Birim == birim {
			return s, true
		}
		// valkey/redis aynı allowlist girdisini paylaşır: AlmaLinux 10 AppStream
		// valkey'i, AlmaLinux 9 (ve öncesi) hâlâ redis paketini kurar — ikisi de
		// protokol uyumlu, tek fark birim adı (bkz. birimVarMi).
		if s.Birim == "valkey" && birim == "redis" {
			s.Birim = "redis"
			return s, true
		}
	}
	return servisTanim{}, false
}

// ServisDurumlar: GET — izin verilen servislerin listesi + durumları (active/inactive/absent).
func ServisDurumlar(w http.ResponseWriter, r *http.Request) {
	type satir struct {
		servisTanim
		Durum string `json:"durum"`
	}
	out := make([]satir, 0, len(servisAllow))
	for _, s := range servisAllow {
		if s.Birim == "valkey" && !birimVarMi("valkey") && birimVarMi("redis") {
			s.Birim = "redis" // EL9: paket adı hâlâ redis
		}
		st := strings.TrimSpace(runOut("systemctl", "is-active", s.Birim))
		// is-active: active / inactive / failed / unknown; birim yoksa "inactive"/boş döner
		if st == "" {
			st = "absent"
		}
		out = append(out, satir{servisTanim: s, Durum: st})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// birimVarMi: systemd unit dosyası yüklü mü (servis kurulu mu)? "valkey"
// birimi EL9'da hiç yoktur — bunu "kurulu ama durmuş" ile karıştırmamak için.
func birimVarMi(birim string) bool {
	return strings.Contains(runOut("systemctl", "list-unit-files", "--no-legend", birim+".service"), birim+".service")
}

// ServisIslem: POST {birim, aksiyon:"restart"|"reload"} — allowlist'li servisi yeniden başlat.
func ServisIslem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Birim   string `json:"birim"`
		Aksiyon string `json:"aksiyon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	tanim, ok := tanimBul(req.Birim)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "bu servis için işlem izni yok")
		return
	}
	aksiyon := req.Aksiyon
	if aksiyon != "restart" && aksiyon != "reload" {
		aksiyon = "restart"
	}
	if aksiyon == "reload" && !tanim.Reload {
		aksiyon = "restart" // reload desteklemeyen serviste restart'a düş
	}
	out := runOut("systemctl", aksiyon, req.Birim)
	durum := strings.TrimSpace(runOut("systemctl", "is-active", req.Birim))
	if durum != "active" {
		httpx.WriteError(w, http.StatusInternalServerError,
			tanim.Etiket+" "+aksiyon+" başarısız (durum: "+durum+") "+strings.TrimSpace(out))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "birim": req.Birim, "aksiyon": aksiyon, "durum": durum,
	})
}

func runOut(name string, args ...string) string {
	b, _ := exec.Command(name, args...).CombinedOutput()
	return string(b)
}
