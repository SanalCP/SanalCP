package laravel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/backups"
	"sanalcp/internal/httpx"
)

type Job struct {
	ID           int64  `json:"id"`
	DomainID     int64  `json:"domain_id"`
	Dizin        string `json:"dizin"`
	Tur          string `json:"tur"`
	Komut        string `json:"komut"`
	Durum        string `json:"durum"`
	Ilerleme     int    `json:"ilerleme"`
	Mesaj        string `json:"mesaj"`
	Cikti        string `json:"cikti"`
	RecoveryFile string `json:"recovery_file"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
}

var jobMu sync.Mutex

const jobSelect = `SELECT id,domain_id,dizin,tur,komut,status,progress,COALESCE(message,''),COALESCE(output,''),COALESCE(recovery_file,''),DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),COALESCE(DATE_FORMAT(started_at,'%Y-%m-%d %H:%i:%s'),''),COALESCE(DATE_FORMAT(finished_at,'%Y-%m-%d %H:%i:%s'),'') FROM laravel_deploy_jobs`

func scanJob(s interface{ Scan(...any) error }) (Job, error) {
	var j Job
	e := s.Scan(&j.ID, &j.DomainID, &j.Dizin, &j.Tur, &j.Komut, &j.Durum, &j.Ilerleme, &j.Mesaj, &j.Cikti, &j.RecoveryFile, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, e
}
func (h *Handlers) jobYeni(ctx context.Context, d domain, p install, tur, komut string) (int64, error) {
	jobMu.Lock()
	defer jobMu.Unlock()
	var n int
	if e := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM laravel_deploy_jobs WHERE domain_id=? AND status IN ('queued','running')`, d.ID).Scan(&n); e != nil {
		return 0, e
	}
	if n > 0 {
		return 0, errors.New("bu domain için çalışan bir Laravel işi var")
	}
	res, e := h.DB.ExecContext(ctx, `INSERT INTO laravel_deploy_jobs(domain_id,dizin,tur,komut) VALUES(?,?,?,?)`, d.ID, p.Rel, tur, komut)
	if e != nil {
		return 0, e
	}
	return res.LastInsertId()
}
func (h *Handlers) jobDurum(id int64, status string, progress int, msg, out, recovery string) {
	_, _ = h.DB.Exec(`UPDATE laravel_deploy_jobs SET status=?,progress=?,message=?,output=IF(?='',output,?),recovery_file=IF(?='',recovery_file,?),started_at=IF(started_at IS NULL AND ?='running',NOW(),started_at),finished_at=IF(? IN ('success','failed','rolled_back'),NOW(),finished_at) WHERE id=?`, status, progress, msg, out, out, recovery, recovery, status, status, id)
}

func (h *Handlers) Jobs(w http.ResponseWriter, r *http.Request) {
	d, e := h.domain(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	rows, e := h.DB.QueryContext(r.Context(), jobSelect+` WHERE domain_id=? ORDER BY id DESC LIMIT 30`, d.ID)
	if e != nil {
		httpx.WriteError(w, 500, e.Error())
		return
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		if j, e := scanJob(rows); e == nil {
			out = append(out, j)
		}
	}
	httpx.WriteJSON(w, 200, out)
}
func (h *Handlers) Job(w http.ResponseWriter, r *http.Request) {
	d, e := h.domain(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	jid, _ := strconv.ParseInt(chi.URLParam(r, "jid"), 10, 64)
	j, e := scanJob(h.DB.QueryRowContext(r.Context(), jobSelect+` WHERE id=? AND domain_id=?`, jid, d.ID))
	if errors.Is(e, sql.ErrNoRows) {
		httpx.WriteError(w, 404, "iş bulunamadı")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, e.Error())
		return
	}
	httpx.WriteJSON(w, 200, j)
}

func (h *Handlers) Deploy(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Dizin   string `json:"dizin"`
		Migrate bool   `json:"migrate"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	d, p, e := h.kurulum(r, q.Dizin)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	if !dosyaVar(filepath.Join(p.Root, ".git", "HEAD")) {
		httpx.WriteError(w, 400, "Laravel dizini bir Git çalışma ağacı değil")
		return
	}
	if dosyaVar(filepath.Join(p.Root, "storage", "framework", "down")) {
		httpx.WriteError(w, 409, "deploy öncesinde bakım modunu kapatın; deploy bakım penceresini kendisi yönetecek")
		return
	}
	id, e := h.jobYeni(r.Context(), d, p, "deploy", map[bool]string{true: "deploy+migrate", false: "deploy"}[q.Migrate])
	if e != nil {
		httpx.WriteError(w, 409, e.Error())
		return
	}
	go h.deployCalistir(id, d, p, q.Migrate)
	httpx.WriteJSON(w, 202, map[string]any{"ok": true, "job_id": id})
}

func (h *Handlers) Composer(w http.ResponseWriter, r *http.Request) {
	var q struct{ Dizin, Komut string }
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	if q.Komut != "install" && q.Komut != "update" && q.Komut != "dump-autoload" {
		httpx.WriteError(w, 400, "izin verilmeyen Composer komutu")
		return
	}
	d, p, e := h.kurulum(r, q.Dizin)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	id, e := h.jobYeni(r.Context(), d, p, "composer", q.Komut)
	if e != nil {
		httpx.WriteError(w, 409, e.Error())
		return
	}
	go h.composerCalistir(id, d, p, q.Komut)
	httpx.WriteJSON(w, 202, map[string]any{"ok": true, "job_id": id})
}

func (h *Handlers) deployCalistir(id int64, d domain, p install, migrate bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	h.jobDurum(id, "running", 5, "Kurtarma noktası oluşturuluyor", "", "")
	name, _, e := backups.CreateRecoveryArchive(ctx, h.DB, d.ID, d.AlanAdi, d.SK, "Laravel deploy öncesi otomatik kurtarma noktası")
	if e != nil {
		h.jobDurum(id, "failed", 100, "Kurtarma noktası oluşturulamadı", e.Error(), "")
		return
	}
	h.jobDurum(id, "running", 20, "Kod güncelleniyor", "", name)
	var output strings.Builder
	if out, err := tenantExec(ctx, d, p, "/usr/bin/php", filepath.Join(p.Root, "artisan"), "down", "--retry=60", "--no-ansi"); err != nil {
		h.deployRollback(id, d, name, fmt.Errorf("bakım modu açılamadı: %w", err), out)
		return
	}
	steps := [][]string{{"git", "pull", "--ff-only"}, {"/usr/local/bin/composer", "install", "--no-interaction", "--no-ansi", "--no-dev", "--prefer-dist", "--optimize-autoloader", "--no-plugins"}, {"/usr/bin/php", filepath.Join(p.Root, "artisan"), "config:cache", "--no-ansi"}, {"/usr/bin/php", filepath.Join(p.Root, "artisan"), "route:cache", "--no-ansi"}, {"/usr/bin/php", filepath.Join(p.Root, "artisan"), "view:cache", "--no-ansi"}}
	if migrate {
		steps = append(steps, []string{"/usr/bin/php", filepath.Join(p.Root, "artisan"), "migrate", "--force", "--no-interaction", "--no-ansi"})
	}
	for i, a := range steps {
		out, err := tenantExec(ctx, d, p, a[0], a[1:]...)
		output.WriteString("$ " + strings.Join(a, " ") + "\n" + out + "\n")
		h.jobDurum(id, "running", 25+(i+1)*60/len(steps), "Deploy adımları çalışıyor", son(output.String(), 40000), name)
		if err != nil {
			h.deployRollback(id, d, name, fmt.Errorf("%s: %w", a[0], err), output.String())
			return
		}
	}
	if out, err := tenantExec(ctx, d, p, "/usr/bin/php", filepath.Join(p.Root, "artisan"), "up", "--no-ansi"); err != nil {
		output.WriteString("$ artisan up\n" + out + "\n")
		h.deployRollback(id, d, name, fmt.Errorf("bakım modu kapatılamadı: %w", err), output.String())
		return
	}
	if code := localHTTP(ctx, d.AlanAdi); code == 0 || code >= 500 {
		h.deployRollback(id, d, name, fmt.Errorf("hedef HTTP sağlık kontrolü başarısız: %d", code), output.String())
		return
	}
	h.jobDurum(id, "success", 100, "Deploy ve sağlık kontrolü tamamlandı", son(output.String(), 40000), name)
}

func (h *Handlers) composerCalistir(id int64, d domain, p install, op string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	h.jobDurum(id, "running", 10, "Kurtarma noktası oluşturuluyor", "", "")
	name, _, e := backups.CreateRecoveryArchive(ctx, h.DB, d.ID, d.AlanAdi, d.SK, "Laravel Composer işlemi öncesi kurtarma noktası")
	if e != nil {
		h.jobDurum(id, "failed", 100, "Kurtarma noktası oluşturulamadı", e.Error(), "")
		return
	}
	args := []string{op, "--no-interaction", "--no-ansi", "--no-scripts", "--no-plugins"}
	if op == "install" {
		args = append(args, "--prefer-dist")
	}
	out, e := tenantExec(ctx, d, p, "/usr/local/bin/composer", args...)
	if e != nil {
		h.deployRollback(id, d, name, e, out)
		return
	}
	h.jobDurum(id, "success", 100, "Composer işlemi tamamlandı", son(out, 40000), name)
}

func (h *Handlers) deployRollback(id int64, d domain, name string, cause error, out string) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	e := backups.RestoreRecoveryArchive(ctx, h.DB, d.ID, d.SK, filepath.Join(backups.BackupRoot, d.SK, name), "files", "")
	rows, _ := h.DB.QueryContext(ctx, `SELECT DISTINCT db_name FROM db_accounts WHERE domain_id=?`, d.ID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var dbn string
			if rows.Scan(&dbn) == nil {
				if x := backups.RestoreRecoveryArchive(ctx, h.DB, d.ID, d.SK, filepath.Join(backups.BackupRoot, d.SK, name), "database", dbn); x != nil && e == nil {
					e = x
				}
			}
		}
	}
	if e != nil {
		h.jobDurum(id, "failed", 100, "Deploy başarısız; otomatik geri alma da başarısız: "+e.Error(), son(out+"\n"+cause.Error(), 40000), name)
	} else {
		h.jobDurum(id, "rolled_back", 100, "Deploy başarısız; kurtarma noktası geri yüklendi", son(out+"\n"+cause.Error(), 40000), name)
	}
}

func tenantExec(ctx context.Context, d domain, p install, bin string, args ...string) (string, error) {
	argv := []string{"-u", d.SK, "--", bin}
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, "runuser", argv...)
	cmd.Dir = p.Root
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + d.Home, "COMPOSER_HOME=" + filepath.Join(d.Home, ".composer"), "COMPOSER_ALLOW_SUPERUSER=0", "APP_ENV=production"}
	b, e := cmd.CombinedOutput()
	return string(b), e
}
func localHTTP(ctx context.Context, domain string) int {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/", nil)
	req.Host = domain
	c := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := c.Do(req)
	if e != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (h *Handlers) RecoverJobs() {
	rows, e := h.DB.Query(`SELECT j.id,j.domain_id,j.recovery_file,d.alan_adi,d.sistem_kullanici FROM laravel_deploy_jobs j JOIN domains d ON d.id=j.domain_id WHERE j.status IN ('queued','running')`)
	if e != nil {
		return
	}
	defer rows.Close()
	type x struct {
		id, did         int64
		rec, domain, sk string
	}
	all := []x{}
	for rows.Next() {
		var a x
		if rows.Scan(&a.id, &a.did, &a.rec, &a.domain, &a.sk) == nil {
			all = append(all, a)
		}
	}
	for _, a := range all {
		if a.rec == "" {
			h.jobDurum(a.id, "failed", 100, "Panel yeniden başladığı için iş kesildi; hedef değiştirilmemişti", "", "")
			continue
		}
		h.deployRollback(a.id, domain{ID: a.did, AlanAdi: a.domain, SK: a.sk}, a.rec, errors.New("panel yeniden başladı"), "")
	}
}
