package backups

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/httpx"
)

type RestoreJob struct {
	ID         int64  `json:"id"`
	DomainID   int64  `json:"domain_id"`
	BackupID   int64  `json:"backup_id"`
	Scope      string `json:"scope"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	Message    string `json:"message"`
	Result     string `json:"result"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

var restoreCreateMu sync.Mutex
var restoreSlots = make(chan struct{}, 2)
var restoreQueue = make(chan struct{}, 16)

func reserveRestoreQueue() bool {
	select {
	case restoreQueue <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseRestoreQueue() { <-restoreQueue }

func (h *Handlers) initRestoreCancels() {
	h.restoreMu.Lock()
	if h.restoreCancels == nil {
		h.restoreCancels = make(map[int64]context.CancelFunc)
	}
	h.restoreMu.Unlock()
}

func (h *Handlers) Restore(w http.ResponseWriter, r *http.Request) {
	domainID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	backupID, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	if domainID < 1 || backupID < 1 {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz yedek")
		return
	}
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT d.is_demo FROM backups b JOIN domains d ON d.id=b.domain_id WHERE b.id=? AND b.domain_id=?`, backupID, domainID).Scan(&isDemo); errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "yedek bulunamadı")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "yedek okunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe geri yükleme yapılamaz")
		return
	}
	req := restoreRequest{Scope: "full"}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz geri yükleme isteği")
			return
		}
	}
	if req.Scope == "" {
		req.Scope = "full"
	}
	if req.Scope != "full" && req.Scope != "files" && req.Scope != "file" && req.Scope != "database" && req.Scope != "email" {
		httpx.WriteError(w, http.StatusBadRequest, "scope: full|files|file|database|email")
		return
	}
	if req.Scope == "file" {
		if _, err := safeRestoreRelativePath(req.Path); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Scope == "database" && !mysqlNameRE.MatchString(req.Database) {
		httpx.WriteError(w, http.StatusBadRequest, "geçerli bir veritabanı seçin")
		return
	}

	restoreCreateMu.Lock()
	defer restoreCreateMu.Unlock()
	var active int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM backup_restore_jobs WHERE domain_id=? AND status IN ('queued','running')`, domainID).Scan(&active); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi denetlenemedi")
		return
	}
	if active != 0 {
		httpx.WriteError(w, http.StatusConflict, "bu domain için devam eden bir geri yükleme var")
		return
	}
	if !reserveRestoreQueue() {
		httpx.WriteError(w, http.StatusServiceUnavailable, "geri yükleme kuyruğu dolu; devam eden işlerden biri tamamlanınca yeniden deneyin")
		return
	}
	queued := true
	defer func() {
		if queued {
			releaseRestoreQueue()
		}
	}()
	res, err := h.DB.ExecContext(r.Context(), `INSERT INTO backup_restore_jobs(domain_id,backup_id,scope,target_path,database_name) VALUES(?,?,?,?,?)`, domainID, backupID, req.Scope, req.Path, req.Database)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi oluşturulamadı")
		return
	}
	jobID, err := res.LastInsertId()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi kimliği alınamadı")
		return
	}
	h.initRestoreCancels()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	h.restoreMu.Lock()
	h.restoreCancels[jobID] = cancel
	h.restoreMu.Unlock()
	h.restoreWG.Add(1)
	go func() {
		defer h.restoreWG.Done()
		defer releaseRestoreQueue()
		h.runRestoreJob(ctx, jobID, domainID, backupID, req)
	}()
	queued = false
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true, "job_id": jobID, "status": "queued"})
}

func (h *Handlers) runRestoreJob(ctx context.Context, jobID, domainID, backupID int64, req restoreRequest) {
	defer func() {
		h.restoreMu.Lock()
		if cancel := h.restoreCancels[jobID]; cancel != nil {
			cancel()
		}
		delete(h.restoreCancels, jobID)
		h.restoreMu.Unlock()
	}()
	select {
	case restoreSlots <- struct{}{}:
		defer func() { <-restoreSlots }()
	case <-ctx.Done():
		h.restoreJobFinish(jobID, "cancelled", "geri yükleme başlamadan iptal edildi", "")
		return
	}
	h.restoreJobUpdate(jobID, "running", 5, "geri yükleme öncesi kurtarma noktası oluşturuluyor")
	var domain, sk string
	if err := h.DB.QueryRowContext(ctx, `SELECT alan_adi,sistem_kullanici FROM domains WHERE id=?`, domainID).Scan(&domain, &sk); err != nil {
		h.restoreJobFinish(jobID, "failed", "domain bilgisi okunamadı", "")
		return
	}
	recovery, _, err := CreateRecoveryArchive(ctx, h.DB, domainID, domain, sk, "Yedek geri yükleme öncesi otomatik kurtarma noktası")
	if err != nil {
		status, msg := "failed", "kurtarma noktası oluşturulamadı: "+err.Error()
		if ctx.Err() != nil {
			status, msg = "cancelled", "geri yükleme başlamadan iptal edildi"
		}
		h.restoreJobFinish(jobID, status, truncateRestoreMessage(msg), "")
		return
	}
	h.restoreJobRecovery(jobID, recovery)
	h.restoreJobUpdate(jobID, "running", 20, "yedek doğrulanıyor ve geri yükleniyor")
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)).WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(domainID, 10))
	rctx.URLParams.Add("bid", strconv.FormatInt(backupID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.restoreNow(w, r)
	if ctx.Err() != nil {
		h.rollbackRestoreJob(jobID, domainID, sk, recovery, req, "geri yükleme iptal edildi")
		return
	}
	if w.Code != http.StatusOK {
		var e httpx.ErrorBody
		_ = json.Unmarshal(w.Body.Bytes(), &e)
		msg := strings.TrimSpace(e.Hata)
		if msg == "" {
			msg = "geri yükleme başarısız"
		}
		h.rollbackRestoreJob(jobID, domainID, sk, recovery, req, msg)
		return
	}
	var result struct {
		Sonuc string `json:"sonuc"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	h.restoreJobFinish(jobID, "success", "geri yükleme tamamlandı", truncateRestoreMessage(result.Sonuc))
}

func truncateRestoreMessage(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

func (h *Handlers) restoreJobUpdate(id int64, status string, progress int, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = h.DB.ExecContext(ctx, `UPDATE backup_restore_jobs SET status=?,progress=?,message=?,started_at=COALESCE(started_at,NOW()) WHERE id=?`, status, progress, message, id)
}

func (h *Handlers) restoreJobFinish(id int64, status, message, result string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = h.DB.ExecContext(ctx, `UPDATE backup_restore_jobs SET status=?,progress=100,message=?,result=?,finished_at=NOW() WHERE id=?`, status, message, result, id)
}

func (h *Handlers) restoreJobRecovery(id int64, recovery string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = h.DB.ExecContext(ctx, `UPDATE backup_restore_jobs SET recovery_file=? WHERE id=?`, recovery, id)
}

func (h *Handlers) rollbackRestoreJob(jobID, domainID int64, sk, recovery string, req restoreRequest, cause string) {
	h.restoreJobUpdate(jobID, "running", 90, "işlem geri alınıyor")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	archive := filepath.Join(BackupRoot, sk, recovery)
	var rollbackErr error
	switch req.Scope {
	case "database":
		rollbackErr = RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "database", req.Database)
	case "files":
		rollbackErr = RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "files", "")
	case "email":
		rollbackErr = RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "email", "")
	case "file":
		rollbackErr = RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "file", req.Path)
	default:
		rollbackErr = RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "home", "")
	}
	if rollbackErr == nil && req.Scope == "full" {
		rows, err := h.DB.QueryContext(ctx, `SELECT DISTINCT db_name FROM db_accounts WHERE domain_id=?`, domainID)
		if err != nil {
			rollbackErr = err
		} else {
			for rows.Next() {
				var database string
				if err := rows.Scan(&database); err != nil {
					rollbackErr = err
					break
				}
				if err := RestoreRecoveryArchive(ctx, h.DB, domainID, sk, archive, "database", database); err != nil {
					rollbackErr = err
					break
				}
			}
			if err := rows.Err(); err != nil && rollbackErr == nil {
				rollbackErr = err
			}
			rows.Close()
		}
	}
	if rollbackErr != nil {
		h.restoreJobFinish(jobID, "failed", truncateRestoreMessage(cause+"; otomatik geri alma başarısız: "+rollbackErr.Error()), "")
		return
	}
	h.restoreJobFinish(jobID, "rolled_back", truncateRestoreMessage(cause+"; değişiklikler otomatik geri alındı"), "")
}

const restoreJobSelect = `SELECT id,domain_id,backup_id,scope,status,progress,message,result,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),COALESCE(DATE_FORMAT(started_at,'%Y-%m-%d %H:%i:%s'),''),COALESCE(DATE_FORMAT(finished_at,'%Y-%m-%d %H:%i:%s'),'') FROM backup_restore_jobs`

func scanRestoreJob(row interface{ Scan(...any) error }) (RestoreJob, error) {
	var j RestoreJob
	err := row.Scan(&j.ID, &j.DomainID, &j.BackupID, &j.Scope, &j.Status, &j.Progress, &j.Message, &j.Result, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, err
}

func (h *Handlers) RestoreJob(w http.ResponseWriter, r *http.Request) {
	domainID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	jobID, _ := strconv.ParseInt(chi.URLParam(r, "jid"), 10, 64)
	j, err := scanRestoreJob(h.DB.QueryRowContext(r.Context(), restoreJobSelect+` WHERE id=? AND domain_id=?`, jobID, domainID))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "geri yükleme işi bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}

// ActiveRestoreJob lets a browser reconnect after a refresh, navigation or a
// temporary polling failure. The response shape is stable even when there is
// no active job, avoiding 404/error toasts during normal page load.
func (h *Handlers) ActiveRestoreJob(w http.ResponseWriter, r *http.Request) {
	domainID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	j, err := scanRestoreJob(h.DB.QueryRowContext(r.Context(), restoreJobSelect+` WHERE domain_id=? AND status IN ('queued','running') ORDER BY id DESC LIMIT 1`, domainID))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"active": true, "job": j})
}

func (h *Handlers) CancelRestoreJob(w http.ResponseWriter, r *http.Request) {
	domainID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	jobID, _ := strconv.ParseInt(chi.URLParam(r, "jid"), 10, 64)
	var status string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT status FROM backup_restore_jobs WHERE id=? AND domain_id=?`, jobID, domainID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "geri yükleme işi bulunamadı")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geri yükleme işi okunamadı")
		return
	}
	if status != "queued" && status != "running" {
		httpx.WriteError(w, http.StatusConflict, "geri yükleme işi artık iptal edilemez")
		return
	}
	h.restoreMu.Lock()
	cancel := h.restoreCancels[jobID]
	h.restoreMu.Unlock()
	if cancel == nil {
		httpx.WriteError(w, http.StatusConflict, "iş başka bir panel sürecinde veya yeniden başlatma sonrasında; durumunu yenileyin")
		return
	}
	cancel()
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true, "status": "cancelling"})
}

func (h *Handlers) RecoverRestoreJobs() {
	rows, err := h.DB.Query(`SELECT j.id,j.domain_id,j.scope,j.target_path,j.database_name,j.recovery_file,d.sistem_kullanici FROM backup_restore_jobs j JOIN domains d ON d.id=j.domain_id WHERE j.status IN ('queued','running')`)
	if err != nil {
		return
	}
	type interrupted struct {
		id, domainID                          int64
		scope, target, database, recovery, sk string
	}
	var jobs []interrupted
	for rows.Next() {
		var j interrupted
		if rows.Scan(&j.id, &j.domainID, &j.scope, &j.target, &j.database, &j.recovery, &j.sk) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		if j.recovery == "" {
			h.restoreJobFinish(j.id, "failed", "Panel yeniden başladığı için iş kesildi; hedef değiştirilmeden önce durmuştu", "")
			continue
		}
		h.rollbackRestoreJob(j.id, j.domainID, j.sk, j.recovery, restoreRequest{Scope: j.scope, Path: j.target, Database: j.database}, "panel yeniden başladığı için geri yükleme kesildi")
	}
}

// CancelRestoreJobs stops child processes and waits for their final status to
// be persisted during a controlled panel shutdown.
func (h *Handlers) CancelRestoreJobs(ctx context.Context) error {
	h.restoreMu.Lock()
	for _, cancel := range h.restoreCancels {
		cancel()
	}
	h.restoreMu.Unlock()
	done := make(chan struct{})
	go func() {
		h.restoreWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
