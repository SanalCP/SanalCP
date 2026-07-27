// Package domainek: addon/parked domain yönetimi ("ek alan adı").
//
// Bir "ek alan adı", hedef domain'in Linux kullanıcısı (sk) ve PHP-FPM havuzunu
// PAYLAŞIR — kendi sistem kullanıcısı/DB/FTP hesabı YOKTUR (subdomain.go ile aynı
// felsefe, tek fark: alt alan adı değil, bağımsız bir domain). domains tablosuna
// normal bir satır olarak eklenir (ana_domain_id dolu) — böylece mevcut plan kotası
// (max_domain), Dosyalar/Loglar/Yedekler gibi domain_id'ye göre çalışan sayfalar ve
// DNS/mail alt sistemleri ekstra değişiklik gerekmeden çalışır.
//
// parked=1 ise hedef domain'in docroot'unu birebir paylaşır (klasik "parked domain",
// ayrı dizin yok); parked=0 ise kendi docroot'u vardır (klasik "addon domain").
//
// GÜVENLİK: bu satırlar sk'yi PAYLAŞTIĞI için internal/domains Delete() özel olarak
// bu satırları (ana_domain_id dolu) provisioner.Deprovision/kaynaklimit.SystemdSliceSil/
// redis.KapatDomain gibi sk-genelindeki yıkıcı işlemlerden MUAF tutmalı — aksi halde
// bir addon domain silinirken ana hesabın TÜM Linux kullanıcısı/PHP havuzu silinir.
// Aynı sebeple provisioner.go'daki "sistem_kullanici=?" tabanlı askida/vhost_ozel
// sorgularına "AND ana_domain_id IS NULL" guard'ı eklendi (bkz. 0045 migration notu).
//
// SSL: ek alan adı kendi sertifikasını alabilir. Vhost'u ana alan adınınkinden
// AYRI bir dosyada durur (ek_<sk>_<alan>.conf) ve SSL'li hâlini de aynı dosyaya
// provisioner.EkVhostYaz yazar — normal SSL yolu dosya adını yalnız sk'den
// türettiği için ek alan adında çağrılırsa ANA alan adının vhost'unu ezer
// (canlıda yaşandı, bkz. internal/provisioner/ek_vhost.go başlığı).
package domainek

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"sanalpanel/internal/dns"
	"sanalpanel/internal/hesaplar"
	"sanalpanel/internal/httpx"
	"sanalpanel/internal/kota"
	"sanalpanel/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

type EkDomain struct {
	ID        int64  `json:"id"`
	AlanAdi   string `json:"alan_adi"`
	Parked    bool   `json:"parked"`
	DocRoot   string `json:"docroot"`
	PHPSurum  string `json:"php_surum"`
	SSLAktif  bool   `json:"ssl_aktif"`
	CreatedAt string `json:"created_at"`
}

type hedefBilgi struct {
	sk         string
	alanAdi    string
	phpSurum   string
	webRoot    string
	customerID *int64
	planID     *int64
	isAddon    bool
	isDemo     bool
}

func (h *Handlers) hedef(r *http.Request) (id int64, hb hedefBilgi, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var demo int
	var anaID sql.NullInt64
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, alan_adi, php_surum, web_root, customer_id, plan_id, ana_domain_id, COALESCE(is_demo,0)
		 FROM domains WHERE id=?`, id).
		Scan(&hb.sk, &hb.alanAdi, &hb.phpSurum, &hb.webRoot, &hb.customerID, &hb.planID, &anaID, &demo)
	if err != nil {
		return id, hb, false
	}
	hb.isAddon = anaID.Valid
	hb.isDemo = demo == 1
	return id, hb, true
}

// confPath: tek kaynak provisioner.EkConfPath — SSL yolu da aynı dosyayı yazar,
// iki yerde ayrı türetmek dosyaların ayrışmasına yol açardı.
func confPath(sk, alanAdi string) string  { return provisioner.EkConfPath(sk, alanAdi) }
func docrootOf(sk, alanAdi string) string { return "/home/" + sk + "/domains/" + alanAdi }

// GET /domains/{id}/ek
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.hedef(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, alan_adi, parked, php_surum, web_root, ssl_aktif, DATE_FORMAT(olusturulma,'%Y-%m-%d %H:%i')
		 FROM domains WHERE ana_domain_id=? ORDER BY alan_adi`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []EkDomain{}
	for rows.Next() {
		var e EkDomain
		var parked, ssl int
		if err := rows.Scan(&e.ID, &e.AlanAdi, &parked, &e.PHPSurum, &e.DocRoot, &ssl, &e.CreatedAt); err == nil {
			e.Parked = parked == 1
			e.SSLAktif = ssl == 1
			out = append(out, e)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/ek  {alan_adi, parked?, php_surum?}
func (h *Handlers) Olustur(w http.ResponseWriter, r *http.Request) {
	id, hb, ok := h.hedef(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if hb.isDemo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if hb.isAddon {
		httpx.WriteError(w, http.StatusBadRequest, "bir ek alan adının altına başka ek alan adı eklenemez")
		return
	}
	var req struct {
		AlanAdi  string `json:"alan_adi"`
		Parked   bool   `json:"parked"`
		PHPSurum string `json:"php_surum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	alanAdi := strings.ToLower(strings.TrimSpace(req.AlanAdi))
	if err := provisioner.ValidateDomain(alanAdi); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	phpSurum := strings.TrimSpace(req.PHPSurum)
	if phpSurum == "" {
		phpSurum = hb.phpSurum
	}

	var n int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM domains WHERE alan_adi=?`, alanAdi).Scan(&n)
	if n == 0 {
		_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM subdomanlar WHERE tam_ad=?`, alanAdi).Scan(&n)
	}
	if n > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu alan adı zaten kullanımda")
		return
	}
	if err := kota.CheckDomainEklenebilir(r.Context(), h.DB, hb.customerID); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	socket, err := provisioner.PHPSocketFor(hb.sk, phpSurum)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "PHP sürümü sunucuda kurulu değil: "+phpSurum)
		return
	}

	docroot := hb.webRoot
	if !req.Parked {
		docroot = docrootOf(hb.sk, alanAdi)
		if err := os.MkdirAll(docroot, 0o755); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "docroot oluşturulamadı")
			return
		}
		if _, e := os.Stat(filepath.Join(docroot, "index.html")); e != nil {
			_ = os.WriteFile(filepath.Join(docroot, "index.html"),
				[]byte("<!doctype html><meta charset=utf-8><title>"+alanAdi+"</title>"+
					"<body style='font-family:sans-serif;text-align:center;padding:60px'>"+
					"<h1>"+alanAdi+"</h1><p>Ek alan adı hazır. Dosyalarınızı bu dizine yükleyin.</p></body>"), 0o644)
		}
		_ = exec.Command("chown", "-R", hb.sk+":"+hb.sk, "/home/"+hb.sk+"/domains").Run()
		_ = exec.Command("chcon", "-R", "-t", "httpd_sys_content_t", docroot).Run()
	}

	conf := confPath(hb.sk, alanAdi)
	if err := os.WriteFile(conf, []byte(provisioner.EkVhostIcerik(alanAdi, docroot, socket, "", "")), 0o644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost yazılamadı")
		return
	}
	_ = exec.Command("restorecon", conf).Run()
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		_ = os.Remove(conf)
		_ = exec.Command("nginx", "-t").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "nginx doğrulanamadı: "+strings.TrimSpace(string(out)))
		return
	}
	_ = exec.Command("systemctl", "reload", "nginx").Run()

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domains(alan_adi, sistem_kullanici, php_surum, ssl_aktif, durum, ipv4,
		   db_host, web_root, is_demo, customer_id, plan_id, ana_domain_id, parked)
		 VALUES(?,?,?,0,'aktif',?, 'localhost',?, 0, ?, ?, ?, ?)`,
		alanAdi, hb.sk, phpSurum, h.IPv4, docroot, hb.customerID, hb.planID, id, req.Parked)
	if err != nil {
		_ = os.Remove(conf)
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		if !req.Parked {
			_ = os.RemoveAll(docroot)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "kayıt eklenemedi: "+err.Error())
		return
	}
	ekID, _ := res.LastInsertId()

	// Bağımsız bir domain olduğu için kendi DNS zone'unu alır (subdomain'in aksine —
	// subdomain parent zone'a A kaydı ekler, ek alan adı kendi zone'unu ister).
	if h.IPv4 != "" {
		if _, err := dns.SeedDefaults(r.Context(), h.DB, ekID, alanAdi, h.IPv4); err != nil {
			_ = err // best-effort, log yok (paket henüz log importu kullanmıyor)
		}
		_ = dns.WriteZone(r.Context(), h.DB, ekID)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": ekID, "alan_adi": alanAdi, "docroot": docroot, "parked": req.Parked})
}

// DELETE /domains/{id}/ek/{ekid}
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.hedef(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	ekid, _ := strconv.ParseInt(chi.URLParam(r, "ekid"), 10, 64)
	if err := DeleteEkDomain(r.Context(), h.DB, ekid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DeleteEkDomain: bir ek alan adını (addon/parked) tamamen kaldırır — nginx conf,
// (parked değilse) docroot, kendi DNS zone'u, kendi DB hesapları (domain_id ile
// izole, hesaplar.MySQLDropAllForDomain güvenle çağrılabilir) ve domains satırı.
//
// KASITLI OLARAK ÇAĞIRMAZ: provisioner.Deprovision / kaynaklimit.SystemdSliceSil /
// redis.KapatDomain — bunların hepsi paylaşılan sk üzerinde çalışır ve ana hesabın
// TÜM Linux kullanıcısını/PHP havuzunu/redis ACL'ini silerdi. cp_domain_redis satırı
// (varsa) sk'ye dokunmadan doğrudan domain_id ile temizlenir.
//
// parentID: çağıranın (Sil handler veya internal/domains Delete()) beklediği ana
// domain id'si — ownership doğrulaması için WHERE'e dahil edilir.
func DeleteEkDomain(ctx context.Context, db *sql.DB, ekID, parentID int64) error {
	var sk, alanAdi string
	var parked int
	var webRoot string
	err := db.QueryRowContext(ctx,
		`SELECT sistem_kullanici, alan_adi, parked, web_root FROM domains WHERE id=? AND ana_domain_id=?`,
		ekID, parentID).Scan(&sk, &alanAdi, &parked, &webRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("ek alan adı bulunamadı")
	}
	if err != nil {
		return err
	}

	_ = os.Remove(confPath(sk, alanAdi))
	_ = exec.Command("systemctl", "reload", "nginx").Run()

	if parked == 0 {
		docroot := docrootOf(sk, alanAdi)
		base := "/home/" + sk + "/domains/"
		if strings.HasPrefix(docroot, base) && filepath.Clean(docroot) != filepath.Clean(base) {
			_ = os.RemoveAll(docroot)
		}
	}

	_ = hesaplar.MySQLDropAllForDomain(db, ekID)
	_, _ = db.ExecContext(ctx, `DELETE FROM cp_domain_redis WHERE domain_id=?`, ekID)
	_, _ = db.ExecContext(ctx, `DELETE FROM domain_trafik WHERE domain_id=?`, ekID)
	_, _ = db.ExecContext(ctx, `DELETE FROM domain_trafik_imlec WHERE domain_id=?`, ekID)
	_, _ = db.ExecContext(ctx, `DELETE FROM wp_bakim WHERE domain_id=?`, ekID)

	if _, err := db.ExecContext(ctx, `DELETE FROM domains WHERE id=?`, ekID); err != nil {
		return err
	}
	// DNS zone temizliği DELETE'ten SONRA (bkz. internal/domains Delete() aynı sıralama notu).
	_ = dns.DeleteZone(ctx, db, alanAdi)
	return nil
}
