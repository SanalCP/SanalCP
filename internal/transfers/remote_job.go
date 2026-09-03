package transfers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/httpx"
)

type remoteStartReq struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Provider   string `json:"provider"`
	Hesap      string `json:"hesap"`
	Domain     string `json:"domain"`
	CustomerID int64  `json:"customer_id"`
	PlanID     *int64 `json:"plan_id"`
	PHPSurum   string `json:"php_version"`
}
type remoteJob struct {
	ID             int64  `json:"id"`
	Provider       string `json:"provider"`
	Host           string `json:"host"`
	Hesap          string `json:"hesap"`
	Domain         string `json:"domain"`
	Durum          string `json:"durum"`
	Mesaj          string `json:"mesaj"`
	Port           int    `json:"port"`
	Ilerleme       int    `json:"ilerleme"`
	TargetDomainID *int64 `json:"target_domain_id,omitempty"`
	SourceHTTP     int    `json:"source_http_status,omitempty"`
	TargetHTTP     int    `json:"target_http_status,omitempty"`
}

// Toplu seçim yüzlerce domain döndürebilir. Paketleme/mysqldump/SSH akışları
// disk ve CPU yoğundur; en fazla iki domain aynı anda çalışsın, kalanı queued
// durumunda güvenle sırasını beklesin.
var remoteAktarimSem = make(chan struct{}, 2)

func (h *Handlers) RemoteStart(w http.ResponseWriter, r *http.Request) {
	var q remoteStartReq
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	q.Host = strings.TrimSpace(q.Host)
	q.Hesap = strings.TrimSpace(q.Hesap)
	q.Domain = strings.ToLower(strings.TrimSpace(q.Domain))
	if q.Port == 0 {
		q.Port = 22
	}
	if q.PHPSurum == "" {
		q.PHPSurum = "8.3"
	}
	if !uzakHostGecerli(q.Host) || !uzakHesapGecerli(q.Provider, q.Hesap, q.Domain) || !domainKesifGecerli(q.Domain) || q.Port < 1 || q.Port > 65535 || q.CustomerID < 1 {
		httpx.WriteError(w, 400, "geçersiz aktarım hedefi")
		return
	}
	if q.Provider != "sanalcp" && q.Provider != "cpanel" && q.Provider != "plesk" && q.Provider != "directadmin" {
		httpx.WriteError(w, 422, "desteklenmeyen kaynak panel")
		return
	}
	var varMi int
	if h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM customers WHERE id=?`, q.CustomerID).Scan(&varMi) != nil || varMi == 0 {
		httpx.WriteError(w, 400, "müşteri bulunamadı")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `INSERT INTO remote_transfer_jobs(provider,source_host,source_port,source_account,source_domain,customer_id,plan_id,php_version) VALUES(?,?,?,?,?,?,?,?)`, q.Provider, q.Host, q.Port, q.Hesap, q.Domain, q.CustomerID, q.PlanID, q.PHPSurum)
	if err != nil {
		httpx.WriteError(w, 500, "iş oluşturulamadı: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	go h.remoteCalistir(id, q)
	httpx.WriteJSON(w, 202, map[string]any{"ok": true, "job_id": id, "status": "queued"})
}

func uzakHesapGecerli(provider, hesap, domain string) bool {
	if provider == "plesk" {
		return hesap == domain && domainKesifGecerli(hesap)
	}
	return uzakUserRe.MatchString(hesap)
}

func (h *Handlers) RemoteJob(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "jid"), 10, 64)
	j, err := h.remoteJobOku(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, 404, "iş bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, j)
}

func (h *Handlers) RemoteJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT id,provider,source_host,source_port,source_account,source_domain,status,progress,COALESCE(message,''),target_domain_id,COALESCE(source_http_status,0),COALESCE(target_http_status,0) FROM remote_transfer_jobs ORDER BY id DESC LIMIT 25`)
	if err != nil {
		httpx.WriteError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	sonuc := []remoteJob{}
	for rows.Next() {
		var j remoteJob
		var hedef sql.NullInt64
		if rows.Scan(&j.ID, &j.Provider, &j.Host, &j.Port, &j.Hesap, &j.Domain, &j.Durum, &j.Ilerleme, &j.Mesaj, &hedef, &j.SourceHTTP, &j.TargetHTTP) == nil {
			if hedef.Valid {
				j.TargetDomainID = &hedef.Int64
			}
			sonuc = append(sonuc, j)
		}
	}
	httpx.WriteJSON(w, 200, sonuc)
}

func (h *Handlers) remoteJobOku(ctx context.Context, id int64) (remoteJob, error) {
	var j remoteJob
	var hedef sql.NullInt64
	err := h.DB.QueryRowContext(ctx, `SELECT id,provider,source_host,source_port,source_account,source_domain,status,progress,COALESCE(message,''),target_domain_id,COALESCE(source_http_status,0),COALESCE(target_http_status,0) FROM remote_transfer_jobs WHERE id=?`, id).Scan(&j.ID, &j.Provider, &j.Host, &j.Port, &j.Hesap, &j.Domain, &j.Durum, &j.Ilerleme, &j.Mesaj, &hedef, &j.SourceHTTP, &j.TargetHTTP)
	if hedef.Valid {
		j.TargetDomainID = &hedef.Int64
	}
	return j, err
}

func (h *Handlers) remoteCalistir(id int64, q remoteStartReq) {
	remoteAktarimSem <- struct{}{}
	defer func() { <-remoteAktarimSem }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	h.remoteDurum(id, "packaging", 10, "Uzak site paketi hazırlanıyor")
	kaynakHTTP := h.uzakHTTPDurumu(ctx, q)
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET source_http_status=? WHERE id=?`, nullStatus(kaynakHTTP), id)
	f, err := os.CreateTemp("", "sanalcp-remote-*.tar.gz")
	if err != nil {
		h.remoteHata(id, err)
		return
	}
	yol := f.Name()
	defer os.Remove(yol)
	komut, err := uzakPaketKomutu(q)
	if err != nil {
		_ = f.Close()
		h.remoteHata(id, err)
		return
	}
	args := append(uzakSSHArgs(q.Port), "root@"+q.Host, komut)
	cmd := exec.CommandContext(ctx, sshBin, args...)
	lw := &sinirliYazici{w: f, kalan: MaxUploadBytes}
	var stderr bytes.Buffer
	cmd.Stdout = lw
	cmd.Stderr = &stderr
	err = cmd.Run()
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		h.remoteHata(id, errors.New(kisalt(msg, 500)))
		return
	}
	h.remoteDurum(id, "downloading", 50, "Paket güvenli geçici alana alındı")
	h.remoteDurum(id, "importing", 65, "SanalCP import ve rollback hattı çalışıyor")
	rr := h.importRemoteArchive(ctx, yol, q)
	if rr.Code != http.StatusCreated {
		h.remoteHata(id, errors.New(kisalt(rr.Body.String(), 1000)))
		return
	}
	var sonuc struct {
		DomainID int64    `json:"domain_id"`
		Skipped  []string `json:"skipped"`
	}
	if json.Unmarshal(rr.Body.Bytes(), &sonuc) != nil || sonuc.DomainID < 1 {
		h.remoteHata(id, errors.New("import sonucu okunamadı"))
		return
	}
	hedefHTTP := yerelHTTPDurumu(ctx, q.Domain, hedefSSLVar(h.DB, sonuc.DomainID))
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET target_domain_id=?,target_http_status=? WHERE id=?`, sonuc.DomainID, nullStatus(hedefHTTP), id)
	if !httpSaglikli(kaynakHTTP) {
		_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET status='success',progress=100,message=?,finished_at=NOW() WHERE id=?`,
			basariMesaji(fmt.Sprintf("Aktarım tamamlandı; kaynak site HTTP %d verdiği için web sağlığı doğrulanamadı", kaynakHTTP), sonuc.Skipped), id)
		return
	}
	if httpSaglikli(kaynakHTTP) && !httpSaglikli(hedefHTTP) {
		hataOzeti := h.hedefHataOzeti(sonuc.DomainID, q.Domain)
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(sonuc.DomainID, 10), nil).WithContext(ctx)
		if err := h.rollbackDomain(r, sonuc.DomainID); err != nil {
			h.remoteHata(id, fmt.Errorf("sağlık karşılaştırması başarısız: kaynak HTTP %d, hedef HTTP %d; hedef rollback başarısız: %w", kaynakHTTP, hedefHTTP, err))
			return
		}
		if _, err := h.DB.Exec(`UPDATE remote_transfer_jobs SET target_domain_id=NULL WHERE id=?`, id); err != nil {
			h.remoteHata(id, fmt.Errorf("hedef silindi fakat aktarım kaydı güncellenemedi: %w", err))
			return
		}
		mesaj := fmt.Sprintf("sağlık karşılaştırması başarısız: kaynak HTTP %d, hedef HTTP %d; hedef geri alındı", kaynakHTTP, hedefHTTP)
		if hataOzeti != "" {
			mesaj += "; hata: " + hataOzeti
		}
		h.remoteHata(id, errors.New(mesaj))
		return
	}
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET status='success',progress=100,message=?,finished_at=NOW() WHERE id=?`,
		basariMesaji("Aktarım ve sağlık kontrolü tamamlandı", sonuc.Skipped), id)
}

// basariMesaji — import'un "atlandı" uyarılarını başarı mesajına ekler. Aktarım
// başarılı olsa da eksik kalan şeyler (aktarılmayan ana dizin dizinleri, SSL)
// yöneticiye GÖRÜNMELİ; aksi hâlde site sessizce kırık kalıyor.
func basariMesaji(taban string, atlananlar []string) string {
	temiz := []string{}
	for _, a := range atlananlar {
		if a = strings.TrimSpace(a); a != "" {
			temiz = append(temiz, a)
		}
	}
	if len(temiz) == 0 {
		return taban
	}
	return kisalt(taban+" — DİKKAT: "+strings.Join(temiz, "; "), 1500)
}

func (h *Handlers) hedefHataOzeti(domainID int64, domain string) string {
	var sk string
	if h.DB.QueryRow(`SELECT sistem_kullanici FROM domains WHERE id=?`, domainID).Scan(&sk) != nil || !sistemKullaniciRe.MatchString(sk) {
		return ""
	}
	// TÜM kaynaklar taranır. "İlk boş olmayan dosyayı döndür" yaklaşımı yanıltıcıydı:
	// per-tenant FPM günlüğü her açılışta NOTICE satırları yazdığı için nginx hata
	// günlüğüne hiç sıra gelmiyordu.
	kaynaklar := []struct{ ad, yol string }{
		{"php-fpm", "/var/log/php-fpm/tenant-" + sk + ".log"},
		{"nginx", "/var/log/nginx/" + domain + ".error.log"},
		{"php-debug", "/home/" + sk + "/.gpanel/php_debug.log"},
		// Sertifikası olmayan domainde 443 isteği eşleşmeyen-SNI güvenlik ağına
		// düşer; hata orada, domainin kendi günlüğünde DEĞİL, kayda geçer.
		{"nginx-default443", "/var/log/nginx/default443.error.log"},
		{"nginx-default80", "/var/log/nginx/default80.error.log"},
	}
	parcalar := []string{}
	for _, k := range kaynaklar {
		if s := hataSatirlari(dosyaSonu(k.yol, 8000)); s != "" {
			parcalar = append(parcalar, k.ad+": "+s)
		}
	}
	if len(parcalar) == 0 {
		return "PHP/nginx hata günlüğünde ayrıntı bulunamadı" + govdeIpucu(domain)
	}
	return kisalt(strings.Join(parcalar, " | "), 1200) + govdeIpucu(domain)
}

var sistemKullaniciRe = regexp.MustCompile(`^c_[a-z0-9_]+$`)

// hataIzleri — bir günlük satırının teşhis değeri taşıdığını gösteren imzalar.
// FPM'in açılış NOTICE'leri (fpm is running / ready to handle connections /
// configuration file ... test is successful) bilerek dışarıda bırakılır.
var hataIzleri = []string{
	"PHP message", "PHP Fatal", "PHP Parse", "PHP Warning", "Fatal error", "Parse error",
	"ERROR:", "WARNING:", "[error]", "[crit]", "[alert]", "[emerg]",
}

func hataSatirlari(tail string) string {
	secilen := []string{}
	for _, satir := range strings.Split(tail, "\n") {
		satir = strings.TrimSpace(satir)
		if satir == "" {
			continue
		}
		for _, iz := range hataIzleri {
			if strings.Contains(satir, iz) {
				secilen = append(secilen, satir)
				break
			}
		}
	}
	if len(secilen) == 0 {
		return ""
	}
	// En yeni satırlar en değerlisi; sondan en fazla 5 tanesi yeter.
	if len(secilen) > 5 {
		secilen = secilen[len(secilen)-5:]
	}
	return strings.Join(secilen, " ")
}

// govdeIpucu — 500'ü nginx'in mi yoksa uygulamanın mı ürettiğini ayırt etmek
// günlükler sessiz kaldığında tek ipucu olabiliyor.
func govdeIpucu(domain string) string {
	args := []string{"-ksS", "--max-time", "8", "-H", "Host: " + domain, "http://127.0.0.1/"}
	out, err := exec.Command("curl", args...).Output()
	if err != nil {
		return ""
	}
	govde := strings.Join(strings.Fields(string(out)), " ")
	if govde == "" {
		return "; hedef gövdesi boş (PHP çıktısız sonlandı)"
	}
	return "; hedef gövdesi: " + kisalt(govde, 200)
}

func dosyaSonu(p string, limit int64) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	if time.Since(st.ModTime()) > 5*time.Minute {
		return ""
	}
	start := st.Size() - limit
	if start < 0 {
		start = 0
	}
	buf := make([]byte, st.Size()-start)
	if _, err = f.ReadAt(buf, start); err != nil && err != io.EOF {
		return ""
	}
	return string(buf)
}

func uzakPaketKomutu(q remoteStartReq) (string, error) {
	switch q.Provider {
	case "sanalcp":
		return fmt.Sprintf(`sanalcp-transfer-export export %s`, q.Domain), nil
	case "cpanel":
		return fmt.Sprintf(`d=$(mktemp -d /tmp/sanalcp-transfer.XXXXXX) || exit 1; trap 'rm -rf "$d"' EXIT; /scripts/pkgacct %s "$d" >/dev/null 2>&1 || exit 2; cat "$d/cpmove-%s.tar.gz"`, q.Hesap, q.Hesap), nil
	case "plesk":
		// Plesk'in kendi CLI'sı hem belge kökünü hem de veritabanı dökümlerini
		// sürümden bağımsız biçimde sağlar. Çıktı mevcut güvenli import biçimine çevrilir.
		return ortakPaketOnEk(q) + fmt.Sprintf(`; web=$(plesk db -Ne "SELECT h.www_root FROM domains d JOIN hosting h ON h.dom_id=d.id WHERE d.name='%s'" | head -1); test -n "$web" && test -d "$web" || exit 3; cp -a "$web"/. "$r/homedir/public_html/"; plesk db -Ne "SELECT db.name FROM data_bases db JOIN domains d ON db.dom_id=d.id WHERE d.name='%s'" | while IFS= read -r db; do test -n "$db" || continue; plesk db dump "$db" > "$r/mysql/$db.sql" || exit 4; done; test -f /var/named/run-root/var/%s && cp /var/named/run-root/var/%s "$r/dnszones/%s.db" || true; test -f /opt/psa/var/certificates/%s && cp /opt/psa/var/certificates/%s "$r/sslcerts/%s.crt" || true; `, q.Domain, q.Domain, q.Domain, q.Domain, q.Domain, q.Domain, q.Domain, q.Domain) + ortakPaketSonEk(q), nil
	case "directadmin":
		return ortakPaketOnEk(q) + fmt.Sprintf(`; web=/home/%s/domains/%s/public_html; test -d "$web" || exit 3; cp -a "$web"/. "$r/homedir/public_html/"; mysql --defaults-extra-file=/usr/local/directadmin/conf/my.cnf -NBe "SHOW DATABASES LIKE '%s\\_%%'" | while IFS= read -r db; do test -n "$db" || continue; mysqldump --defaults-extra-file=/usr/local/directadmin/conf/my.cnf --single-transaction --routines --events "$db" > "$r/mysql/$db.sql" || exit 4; done; z=/var/named/%s.db; test -f "$z" && cp "$z" "$r/dnszones/%s.db" || true; cert=/usr/local/directadmin/data/users/%s/domains/%s.cert; key=/usr/local/directadmin/data/users/%s/domains/%s.key; test -f "$cert" && cp "$cert" "$r/sslcerts/%s.crt" || true; test -f "$key" && cp "$key" "$r/sslcerts/%s.key" || true; `, q.Hesap, q.Domain, q.Hesap, q.Domain, q.Domain, q.Hesap, q.Domain, q.Hesap, q.Domain, q.Domain, q.Domain) + ortakPaketSonEk(q), nil
	default:
		return "", errors.New("desteklenmeyen kaynak panel")
	}
}

func ortakPaketOnEk(q remoteStartReq) string {
	return fmt.Sprintf(`d=$(mktemp -d /tmp/sanalcp-transfer.XXXXXX) || exit 1; trap 'rm -rf "$d"' EXIT; r="$d/cpmove-%s"; mkdir -p "$r/cp/userdata/%s" "$r/homedir/public_html" "$r/mysql" "$r/dnszones" "$r/sslcerts"; printf '%%s\n' %s > "$r/cp/backup_user"; printf 'main_domain: %%s\n' %s > "$r/cp/userdata/%s/main"`, q.Hesap, q.Hesap, q.Hesap, q.Domain, q.Hesap)
}

func ortakPaketSonEk(q remoteStartReq) string {
	return fmt.Sprintf(`tar -C "$d" -czf - cpmove-%s`, q.Hesap)
}

func (h *Handlers) uzakHTTPDurumu(ctx context.Context, q remoteStartReq) int {
	// Hedef ölçümüyle SİMETRİK olmalı (bkz. yerelHTTPDurumu): 443 sağlıklı bir
	// yanıt vermezse kaynak da düz HTTP üzerinden sorulur, aksi hâlde sertifikası
	// olmayan bir kaynak sitede catch-all vhost'un durumu ölçülürdü.
	komut := fmt.Sprintf(`s=$(curl -ksS -L --max-redirs 3 --max-time 12 -o /dev/null -w '%%{http_code}' --resolve %s:443:127.0.0.1 --resolve www.%s:443:127.0.0.1 https://%s/ 2>/dev/null); case "$s" in 2??|3??|4??) ;; *) f=$(curl -sS --max-time 8 -o /dev/null -w '%%{http_code}' -H 'Host: %s' http://127.0.0.1/ 2>/dev/null); case "$f" in 2??|3??|4??) s=$f ;; *) [ "$s" = 000 ] && s=$f ;; esac ;; esac; printf '%%s' "$s"`, q.Domain, q.Domain, q.Domain, q.Domain)
	args := append(uzakSSHArgs(q.Port), "root@"+q.Host, komut)
	out, err := exec.CommandContext(ctx, sshBin, args...).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return v
}

// yerelHTTPDurumu — hedef sitenin sağlığı. Yeni aktarılan domainde henüz
// sertifika yoksa 443 isteği eşleşmeyen-SNI güvenlik ağına (_default443) düşer;
// o vhost'un durumu sitenin durumu DEĞİLDİR. Bu yüzden SSL kapalıyken ölçüm düz
// HTTP üzerinden yapılır. SSL açıkken düz HTTP'ye DÜŞÜLMEZ: port 80 vhost'u
// https'e 301 verdiği için bozuk bir site "sağlıklı" görünürdü.
func yerelHTTPDurumu(ctx context.Context, domain string, sslVar bool) int {
	if !sslVar {
		return yerelDuzHTTPDurumu(ctx, domain)
	}
	s := yerelHTTPSDurumu(ctx, domain)
	if httpSaglikli(s) {
		return s
	}
	if h := yerelDuzHTTPDurumu(ctx, domain); httpSaglikli(h) {
		return h
	}
	return s
}

func yerelHTTPSDurumu(ctx context.Context, domain string) int {
	return curlDurumu(ctx, "-ksS", "-L", "--max-redirs", "3", "--max-time", "12", "-o", "/dev/null", "-w", "%{http_code}",
		"--resolve", domain+":443:127.0.0.1", "--resolve", "www."+domain+":443:127.0.0.1", "https://"+domain+"/")
}

func yerelDuzHTTPDurumu(ctx context.Context, domain string) int {
	return curlDurumu(ctx, "-sS", "--max-time", "8", "-o", "/dev/null", "-w", "%{http_code}",
		"-H", "Host: "+domain, "http://127.0.0.1/")
}

func curlDurumu(ctx context.Context, args ...string) int {
	out, err := exec.CommandContext(ctx, "curl", args...).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return v
}

func hedefSSLVar(db *sql.DB, domainID int64) bool {
	var aktif int
	if db.QueryRow(`SELECT ssl_aktif FROM domains WHERE id=?`, domainID).Scan(&aktif) != nil {
		return false
	}
	return aktif == 1
}

func httpSaglikli(v int) bool { return v >= 200 && v < 500 }
func nullStatus(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func (h *Handlers) importRemoteArchive(ctx context.Context, yol string, q remoteStartReq) *httptest.ResponseRecorder {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		part, e := mw.CreateFormFile("archive", "cpmove-"+q.Hesap+".tar.gz")
		if e == nil {
			var f *os.File
			f, e = os.Open(yol)
			if e == nil {
				_, e = io.Copy(part, f)
				f.Close()
			}
		}
		alanlar := map[string]string{"customer_id": strconv.FormatInt(q.CustomerID, 10), "domain": q.Domain, "php_version": q.PHPSurum}
		if q.PlanID != nil {
			alanlar["plan_id"] = strconv.FormatInt(*q.PlanID, 10)
		}
		for k, v := range alanlar {
			if e == nil {
				e = mw.WriteField(k, v)
			}
		}
		if e == nil {
			e = mw.Close()
		}
		_ = pw.CloseWithError(e)
	}()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transfers/import", pr).WithContext(ctx)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	h.Import(rr, req)
	return rr
}

func (h *Handlers) remoteDurum(id int64, durum string, ilerleme int, mesaj string) {
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET status=?,progress=?,message=?,started_at=COALESCE(started_at,NOW()) WHERE id=?`, durum, ilerleme, mesaj, id)
}
func (h *Handlers) remoteHata(id int64, err error) {
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET status='failed',message=?,finished_at=NOW() WHERE id=?`, kisalt(err.Error(), 2000), id)
}

type sinirliYazici struct {
	w     io.Writer
	kalan int64
}

func (s *sinirliYazici) Write(p []byte) (int, error) {
	if int64(len(p)) > s.kalan {
		return 0, errors.New("uzak arşiv 20 GiB sınırını aşıyor")
	}
	n, e := s.w.Write(p)
	s.kalan -= int64(n)
	return n, e
}
func kisalt(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// RecoverRemoteJobs servis kesintisinde yarım kalan işleri yanlışlıkla “çalışıyor” bırakmaz.
func RecoverRemoteJobs(db *sql.DB) {
	_, _ = db.Exec(`UPDATE remote_transfer_jobs SET status='failed',message='Panel yeniden başladığı için iş kesildi; aktarımı yeniden başlatın',finished_at=NOW() WHERE status IN ('queued','packaging','downloading','importing')`)
}
