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
		DomainID int64 `json:"domain_id"`
	}
	if json.Unmarshal(rr.Body.Bytes(), &sonuc) != nil || sonuc.DomainID < 1 {
		h.remoteHata(id, errors.New("import sonucu okunamadı"))
		return
	}
	hedefHTTP := yerelHTTPDurumu(ctx, q.Domain)
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET target_domain_id=?,target_http_status=? WHERE id=?`, sonuc.DomainID, nullStatus(hedefHTTP), id)
	if httpSaglikli(kaynakHTTP) && !httpSaglikli(hedefHTTP) {
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(sonuc.DomainID, 10), nil).WithContext(ctx)
		if err := h.rollbackDomain(r, sonuc.DomainID); err != nil {
			h.remoteHata(id, fmt.Errorf("sağlık karşılaştırması başarısız: kaynak HTTP %d, hedef HTTP %d; hedef rollback başarısız: %w", kaynakHTTP, hedefHTTP, err))
			return
		}
		if _, err := h.DB.Exec(`UPDATE remote_transfer_jobs SET target_domain_id=NULL WHERE id=?`, id); err != nil {
			h.remoteHata(id, fmt.Errorf("hedef silindi fakat aktarım kaydı güncellenemedi: %w", err))
			return
		}
		h.remoteHata(id, fmt.Errorf("sağlık karşılaştırması başarısız: kaynak HTTP %d, hedef HTTP %d; hedef geri alındı", kaynakHTTP, hedefHTTP))
		return
	}
	_, _ = h.DB.Exec(`UPDATE remote_transfer_jobs SET status='success',progress=100,message='Aktarım ve sağlık kontrolü tamamlandı',finished_at=NOW() WHERE id=?`, id)
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
	komut := fmt.Sprintf(`s=$(curl -ksS --max-time 8 -o /dev/null -w '%%{http_code}' --resolve %s:443:127.0.0.1 https://%s/ 2>/dev/null); test "$s" != 000 || s=$(curl -sS --max-time 8 -o /dev/null -w '%%{http_code}' -H 'Host: %s' http://127.0.0.1/ 2>/dev/null); printf '%%s' "$s"`, q.Domain, q.Domain, q.Domain)
	args := append(uzakSSHArgs(q.Port), "root@"+q.Host, komut)
	out, err := exec.CommandContext(ctx, sshBin, args...).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return v
}

func yerelHTTPDurumu(ctx context.Context, domain string) int {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/", nil)
	req.Host = domain
	c := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
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
