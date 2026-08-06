// Package subdomain: alt alan adı (subdomain) yönetimi — Plesk modeli.
// Subdomain, parent domain'in kullanıcısı/PHP havuzu altında; ayrı docroot + nginx server bloğu + DNS A kaydı.
package subdomain

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sanalcp/internal/dns"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
	"sanalcp/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

var reAlt = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Sub struct {
	ID       int64  `json:"id"`
	AltAd    string `json:"alt_ad"`
	TamAd    string `json:"tam_ad"`
	PHPSurum string `json:"php_surum"`
	DocRoot  string `json:"docroot"`
	// PHPSabit: true ise PHP sürümü bu alt alan adı için değiştirilemez
	// (tenant per-tenant FPM kullanıyor, sürüm ana domaine bağlı).
	PHPSabit  bool   `json:"php_sabit"`
	CreatedAt string `json:"created_at"`
}

func (h *Handlers) parent(r *http.Request) (id int64, sk, alanAdi, phpSurum string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, alan_adi, COALESCE(php_surum,'8.3'), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&sk, &alanAdi, &phpSurum, &isDemo); err != nil {
		return id, "", "", "", false, false
	}
	return id, sk, alanAdi, phpSurum, isDemo == 1, true
}

func docrootOf(sk, tamAd string) string { return "/home/" + sk + "/subdomains/" + tamAd }
func confPath(sk, altAd string) string  { return "/etc/nginx/conf.d/sub_" + sk + "_" + altAd + ".conf" }

// GET /domains/{id}/subdomain
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	id, sk, _, _, _, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, alt_ad, tam_ad, php_surum, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM subdomanlar WHERE domain_id=? ORDER BY alt_ad`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	// phpSabit: per-tenant FPM'de alt alan adının sürümü ana domaine bağlıdır
	// ve değiştirilemez. Arayüz seçiciyi buna göre kapatır — kullanıcının
	// deneyip 409 alması yerine nedenini baştan görmesi daha iyi.
	phpSabit := provisioner.TenantFPMActive(sk)
	out := []Sub{}
	for rows.Next() {
		var s Sub
		if err := rows.Scan(&s.ID, &s.AltAd, &s.TamAd, &s.PHPSurum, &s.CreatedAt); err == nil {
			s.DocRoot = docrootOf(sk, s.TamAd)
			s.PHPSabit = phpSabit
			out = append(out, s)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/subdomain  {alt_ad, php_surum?}
func (h *Handlers) Olustur(w http.ResponseWriter, r *http.Request) {
	id, sk, alanAdi, parentPHP, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !strings.HasPrefix(sk, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	var req struct {
		AltAd    string `json:"alt_ad"`
		PHPSurum string `json:"php_surum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	altAd := strings.ToLower(strings.TrimSpace(req.AltAd))
	if !reAlt.MatchString(altAd) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz alt alan (küçük harf/rakam/-)")
		return
	}
	phpSurum := strings.TrimSpace(req.PHPSurum)
	if phpSurum == "" {
		phpSurum = parentPHP
	}
	tamAd := altAd + "." + alanAdi
	if err := provisioner.ValidateDomain(tamAd); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz alan adı")
		return
	}
	// çakışma kontrolü
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM subdomanlar WHERE tam_ad=?`, tamAd).Scan(&n)
	if n == 0 {
		_ = h.DB.QueryRow(`SELECT COUNT(*) FROM domains WHERE alan_adi=?`, tamAd).Scan(&n)
	}
	if n > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu alan adı zaten kullanımda")
		return
	}
	// 🔴 Per-tenant FPM'de tenant TEK bir php-fpm master çalıştırır ve tüm
	// alan adları aynı sokete gider — istenen sürüm fiilen uygulanamaz.
	// İstenen değeri DB'ye yazmak paneli yalancı yapardı ("PHP 7.4" gösterip
	// 8.1 sunmak); onun yerine GERÇEKTE sunulan sürüm (ana domainin sürümü)
	// kaydedilir. Farklı sürüm isteniyorsa ana domainin sürümü değiştirilmeli.
	if provisioner.TenantFPMActive(sk) && phpSurum != parentPHP {
		phpSurum = parentPHP
	}
	socket, err := provisioner.PHPSocketFor(sk, phpSurum)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "PHP sürümü sunucuda kurulu değil: "+phpSurum)
		return
	}
	// 🔴 GÜVENLİK: docroot tenant'ın yazabildiği ~/subdomains altındadır; yol
	// tabanlı os.MkdirAll/os.WriteFile bir symlink'i izleyip root olarak jail
	// DIŞINA dizin açıp dosya yazardı. jailpath tüm bileşenleri
	// openat2(RESOLVE_BENEATH|NO_SYMLINKS) ile symlink-siz çözer.
	home, err := jailpath.TenantHome(sk)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	docrootRel := "subdomains/" + tamAd
	if err := jailpath.DizinOlustur(home, docrootRel, sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "docroot güvenli değil (symlink?): "+err.Error())
		return
	}
	docroot := docrootOf(sk, tamAd)
	// başlangıç sayfası
	if _, e := os.Stat(filepath.Join(docroot, "index.html")); e != nil {
		_ = jailpath.DosyaYaz(home, docrootRel+"/index.html", sk,
			[]byte("<!doctype html><meta charset=utf-8><title>"+tamAd+"</title>"+
				"<body style='font-family:sans-serif;text-align:center;padding:60px'>"+
				"<h1>"+tamAd+"</h1><p>Subdomain hazır. Dosyalarınızı bu dizine yükleyin.</p></body>"), 0o644)
	}
	_ = exec.Command("chown", "-R", sk+":"+sk, "/home/"+sk+"/subdomains").Run()
	_ = exec.Command("chcon", "-R", "-t", "httpd_sys_content_t", docroot).Run()

	// nginx server bloğu
	conf := confPath(sk, altAd)
	if err := os.WriteFile(conf, []byte(vhost(tamAd, docroot, socket)), 0o644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost yazılamadı")
		return
	}
	_ = exec.Command("restorecon", conf).Run()
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		_ = os.Remove(conf) // rollback: bozuk conf'u kaldır (çalışan nginx etkilenmez)
		_ = exec.Command("nginx", "-t").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "nginx doğrulanamadı: "+strings.TrimSpace(string(out)))
		return
	}
	_ = exec.Command("systemctl", "reload", "nginx").Run()

	if _, err := h.DB.Exec(`INSERT INTO subdomanlar (domain_id, alt_ad, tam_ad, php_surum) VALUES (?,?,?,?)`,
		id, altAd, tamAd, phpSurum); err != nil {
		_ = os.Remove(conf)
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "kayıt eklenemedi")
		return
	}
	// DNS A kaydı (parent zone'a) + zone yaz
	if h.IPv4 != "" {
		_, _ = h.DB.Exec(`INSERT INTO dns_records (domain_id, ad, tip, deger, ttl, oncelik, aktif) VALUES (?,?,?,?,?,?,1)`,
			id, altAd, "A", h.IPv4, 3600, 0)
		_ = dns.WriteZone(r.Context(), h.DB, id)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "tam_ad": tamAd, "docroot": docroot})
}

// PUT /domains/{id}/subdomain/{sid}  {php_surum}
//
// Subdomain'in PHP sürümünü değiştirir. Eskiden sürüm YALNIZ oluşturma anında
// belirlenebiliyordu; eski bir uygulama (ör. PHP 7.4 isteyen bir script) alt
// alan adına konduğunda tek çare subdomain'i silip yeniden oluşturmaktı —
// dosyalar ve sertifika da gidiyordu.
//
// SSL DURUMU KORUNUR: sertifika duruyorsa vhost SSL'li varyantla yeniden
// yazılır. Bunu atlamak, PHP sürümü değiştiren kullanıcının sitesini sessizce
// HTTP'ye düşürürdü.
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	id, sk, _, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	altAd, tamAd, mevcutPHP, ok := h.subInfo(r, id)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "subdomain bulunamadı")
		return
	}
	var req struct {
		PHPSurum string `json:"php_surum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	yeni := strings.TrimSpace(req.PHPSurum)
	if yeni == "" || yeni == mevcutPHP {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "php_surum": mevcutPHP})
		return
	}
	// 🔴 PER-TENANT FPM'DE SÜRÜM DEĞİŞTİRİLEMEZ — ve bunu SESSİZCE geçmek
	// yasaktır. Tenant per-tenant FPM'e (Seçenek A) geçmişse tek bir php-fpm
	// master çalışır ve TEK bir soket sunar; PHPSocketFor bu durumda istenen
	// sürümü YOK SAYIP tenant soketini döner. Yani vhost'u yeniden yazsak da
	// istekler eski sürüme gitmeye devam ederdi: panel "PHP 7.4" gösterir,
	// sunucu 8.1 çalıştırırdı. Kullanıcı bunu ancak script'i patlayınca anlardı.
	//
	// Doğru kaldıraç ana domainin PHP sürümüdür: değiştirildiğinde
	// EnableTenantFPM unit'i yeni sürümle yeniden kurar (bkz. internal/php)
	// ve alt alan adları da onu izler.
	if provisioner.TenantFPMActive(sk) {
		httpx.WriteError(w, http.StatusConflict,
			"Bu hesap kendine ait bir PHP-FPM servisi kullanıyor; alt alan adları ana domainle "+
				"AYNI PHP sürümünü paylaşır. Farklı bir sürüm için ana domainin PHP sürümünü "+
				"değiştirin (alt alan adları da onu izler).")
		return
	}
	socket, err := provisioner.PHPSocketFor(sk, yeni)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "PHP sürümü sunucuda kurulu değil: "+yeni)
		return
	}

	// Mevcut conf'u sakla: nginx doğrulaması başarısız olursa BİREBİR geri
	// yazılır. Yeniden üretmek yerine kopyalamak, conf'a elle eklenmiş
	// satırların kaybolmamasını da sağlar.
	conf := confPath(sk, altAd)
	eskiConf, okumaHatasi := os.ReadFile(conf)

	docroot := docrootOf(sk, tamAd)
	crt, key := certYolu(sk, tamAd)
	yeniIcerik := vhost(tamAd, docroot, socket)
	if dosyaVar(crt) && dosyaVar(key) {
		yeniIcerik = vhostSSL(tamAd, docroot, socket, crt, key)
	}
	if err := os.WriteFile(conf, []byte(yeniIcerik), 0o644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost yazılamadı")
		return
	}
	_ = exec.Command("restorecon", conf).Run()
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		if okumaHatasi == nil {
			_ = os.WriteFile(conf, eskiConf, 0o644)
		} else {
			_ = os.Remove(conf)
		}
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "nginx doğrulanamadı: "+strings.TrimSpace(string(out)))
		return
	}
	_ = exec.Command("systemctl", "reload", "nginx").Run()

	// DB en SON güncellenir: nginx doğrulamasından geçmeyen bir sürümü kayda
	// yazmak, panelin gerçekte çalışmayan bir sürümü göstermesine yol açardı.
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE subdomanlar SET php_surum=? WHERE domain_id=? AND tam_ad=?`, yeni, id, tamAd); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kayıt güncellenemedi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "tam_ad": tamAd, "php_surum": yeni})
}

// DELETE /domains/{id}/subdomain/{sid}
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	id, sk, _, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !strings.HasPrefix(sk, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	sid, _ := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var altAd, tamAd string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT alt_ad, tam_ad FROM subdomanlar WHERE id=? AND domain_id=?`, sid, id).Scan(&altAd, &tamAd); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "subdomain bulunamadı")
		return
	}
	_ = os.Remove(confPath(sk, altAd))
	_ = exec.Command("systemctl", "reload", "nginx").Run()
	// docroot sil — symlink-GÜVENLİ.
	//
	// 🔴 Eskiden os.RemoveAll(docroot) idi ve tek koruma bir STRING öneki
	// kontrolüydü. Panel root çalışır ve ~/subdomains tenant'ın yazabildiği bir
	// dizindir: tenant onu /etc'ye bakan bir symlink'e çevirirse yol çözümlemesi
	// jail DIŞINA gider ve silme orada gerçekleşirdi. String kontrolü bunu
	// göremez, çünkü yolun kendisi hâlâ /home/<sk>/subdomains/... görünür.
	//
	// jailpath.Sil fd-göreli ilerler ve hiçbir bileşende symlink takip etmez.
	if home, herr := jailpath.TenantHome(sk); herr == nil {
		if err := jailpath.Sil(home, "subdomains/"+tamAd); err != nil && !os.IsNotExist(err) {
			log.Printf("subdomain docroot silinemedi (%s): %v", tamAd, err)
		}
	}
	// SSL sertifikası da gitmeli: docroot ve vhost silindikten sonra
	// ~/ssl/<tam_ad>.crt|.key yetim kalırdı. Aynı ad tekrar oluşturulursa eski
	// (artık alan adına ait olmayan) sertifikanın yeniden kullanılması da
	// önlenir. Yol sabit ve tenant home'unun ALTINDA olduğu için symlink-güvenli
	// silme kullanılır — bkz. yukarıdaki docroot notu.
	if home, herr := jailpath.TenantHome(sk); herr == nil {
		for _, uzanti := range []string{".crt", ".key"} {
			if err := jailpath.Sil(home, "ssl/"+tamAd+uzanti); err != nil && !os.IsNotExist(err) {
				log.Printf("subdomain sertifikası silinemedi (%s%s): %v", tamAd, uzanti, err)
			}
		}
	}
	_, _ = h.DB.Exec(`DELETE FROM subdomanlar WHERE id=? AND domain_id=?`, sid, id)
	_, _ = h.DB.Exec(`DELETE FROM dns_records WHERE domain_id=? AND ad=? AND tip='A'`, id, altAd)
	_ = dns.WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func vhost(tamAd, docroot, socket string) string {
	return `server {
    listen 80;
    listen [::]:80;
    server_name ` + tamAd + `;

    root ` + docroot + `;
    index index.php index.html index.htm;

    access_log /var/log/nginx/` + tamAd + `.access.log;
    error_log  /var/log/nginx/` + tamAd + `.error.log warn;

    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    location /.well-known/acme-challenge/ {
        root /var/www/_acme;
        try_files $uri =404;
    }

    location / { try_files $uri $uri/ /index.php?$query_string; }

    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass unix:` + socket + `;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_read_timeout 60s;
    }

    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|pdf|zip|gz)$ {
        expires 30d;
        access_log off;
    }

    location ~ /\.(?!well-known) { deny all; }

    # SanalCP subdomain — ` + tamAd + `
}
`
}
