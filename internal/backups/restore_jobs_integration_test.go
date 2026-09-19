package backups

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/go-sql-driver/mysql"
)

// Bu test gerçek MariaDB, gerçek bir disposable sistem kullanıcısı, tar,
// runuser ve rsync kullanır. Normal test koşusunu ağırlaştırmamak için opt-in'dir:
//
//	SANALCP_RESTORE_IT=1 go test ./internal/backups -run RestoreJobCanli -v
//
// Başarılı arka plan geri yüklemesini ve panel yeniden başladıktan sonraki
// recovery-file rollback yolunu aynı fixture üzerinde uçtan uca doğrular.
func TestRestoreJobCanliBasariVeYenidenBaslatmaRollback(t *testing.T) {
	if os.Getenv("SANALCP_RESTORE_IT") != "1" || os.Geteuid() != 0 {
		t.Skip("SANALCP_RESTORE_IT=1 ve root gerekli")
	}
	for _, tool := range []string{"mysql", "mysqldump", "tar", "rsync", "runuser", "useradd", "userdel"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s aracı yok", tool)
		}
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(suffix) > 10 {
		suffix = suffix[len(suffix)-10:]
	}
	sk := "c_rit_" + suffix
	dbName := "zz_restore_it_" + suffix
	domain := "restore-" + suffix + ".test"
	home := filepath.Join("/home", sk)
	backupDir := filepath.Join(BackupRoot, sk)

	rootSQL := func(sqlText string) error {
		cmd := exec.Command("mysql", "-uroot")
		cmd.Stdin = strings.NewReader(sqlText)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mysql: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	if err := rootSQL("CREATE DATABASE `" + dbName + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rootSQL("DROP DATABASE IF EXISTS `" + dbName + "`"); err != nil {
			t.Errorf("test veritabanı temizliği: %v", err)
		}
	})

	db, err := sql.Open("mysql", "root@unix(/run/mysqld/mysqld.sock)/"+dbName+"?parseTime=true&multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(restoreIntegrationSchema); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command("useradd", "-m", "-U", "-d", home, "-s", "/usr/sbin/nologin", sk).CombinedOutput(); err != nil {
		t.Fatalf("useradd: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(backupDir)
		if out, err := exec.Command("userdel", "-r", sk).CombinedOutput(); err != nil {
			t.Errorf("userdel: %v: %s", err, out)
		}
	})

	uid, gid := restoreIntegrationIDs(t, sk)
	writeTenantFile := func(rel, value string) {
		t.Helper()
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(filepath.Dir(path), uid, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	readTenantFile := func(rel string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(home, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	if _, err := db.Exec(`INSERT INTO domains(id,alan_adi,sistem_kullanici,is_demo) VALUES(1,?,?,0)`, domain, sk); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Kullanıcının seçtiği yedek "yedek sürüm" içeriyor. Canlı home daha sonra
	// değiştirilir; iş başlarken oluşturulan recovery arşivi bu ikinci hâli tutar.
	writeTenantFile("public_html/index.txt", "yedek-surum")
	backupName := "restore-source.tar.gz"
	backupPath := filepath.Join(backupDir, backupName)
	size, err := createArchive(context.Background(), db, 1, domain, sk, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO backups(domain_id,tip,dosya,boyut_b,notlar) VALUES(1,'tam',?,?,?)`, backupName, size, "restore IT")
	if err != nil {
		t.Fatal(err)
	}
	backupID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	writeTenantFile("public_html/index.txt", "geri-donulecek-canli-surum")
	writeTenantFile("public_html/eski.txt", "rollback bunu geri getirmeli")

	h := &Handlers{DB: db}
	body, _ := json.Marshal(restoreRequest{Scope: "files"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	rctx.URLParams.Add("bid", strconv.FormatInt(backupID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Restore(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("restore kabul edilmedi: HTTP %d: %s", w.Code, w.Body.String())
	}
	var accepted struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil || accepted.JobID < 1 {
		t.Fatalf("job_id alınamadı: %v: %s", err, w.Body.String())
	}

	job := restoreIntegrationWaitJob(t, db, accepted.JobID, "success", 90*time.Second)
	if got := readTenantFile("public_html/index.txt"); got != "yedek-surum" {
		t.Fatalf("geri yüklenen içerik = %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, "public_html/eski.txt")); !os.IsNotExist(err) {
		t.Fatalf("yedekte olmayan dosya silinmedi: %v", err)
	}

	// Panel, dosyalar değiştirildikten sonra kapanmış gibi aynı işi running'e
	// döndür. RecoverRestoreJobs kayıtlı recovery arşivini uygulamalı.
	if job.recovery == "" {
		t.Fatal("iş recovery arşivi kaydetmedi")
	}
	if _, err := db.Exec(`UPDATE backup_restore_jobs SET status='running',progress=50,finished_at=NULL WHERE id=?`, accepted.JobID); err != nil {
		t.Fatal(err)
	}
	h.RecoverRestoreJobs()
	recovered := restoreIntegrationWaitJob(t, db, accepted.JobID, "rolled_back", 30*time.Second)
	if !strings.Contains(recovered.message, "yeniden başladığı") {
		t.Fatalf("beklenmeyen recovery mesajı: %q", recovered.message)
	}
	if got := readTenantFile("public_html/index.txt"); got != "geri-donulecek-canli-surum" {
		t.Fatalf("rollback içeriği = %q", got)
	}
	if got := readTenantFile("public_html/eski.txt"); got != "rollback bunu geri getirmeli" {
		t.Fatalf("rollback silinen dosyayı getirmedi: %q", got)
	}

	// İki worker slotunu geçici olarak doldurup yeni işi queued durumda tut.
	// DELETE ucu bu işi context üzerinden iptal etmeli; dosyalara dokunulmamalı.
	restoreSlots <- struct{}{}
	restoreSlots <- struct{}{}
	slotsReleased := false
	defer func() {
		if !slotsReleased {
			<-restoreSlots
			<-restoreSlots
		}
	}()
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	rctx.URLParams.Add("bid", strconv.FormatInt(backupID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w = httptest.NewRecorder()
	h.Restore(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("queued restore kabul edilmedi: HTTP %d: %s", w.Code, w.Body.String())
	}
	accepted = struct {
		JobID int64 `json:"job_id"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil || accepted.JobID < 1 {
		t.Fatalf("queued job_id alınamadı: %v: %s", err, w.Body.String())
	}
	activeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	activeRoute := chi.NewRouteContext()
	activeRoute.URLParams.Add("id", "1")
	activeReq = activeReq.WithContext(context.WithValue(activeReq.Context(), chi.RouteCtxKey, activeRoute))
	activeW := httptest.NewRecorder()
	h.ActiveRestoreJob(activeW, activeReq)
	if activeW.Code != http.StatusOK || !strings.Contains(activeW.Body.String(), `"active":true`) ||
		!strings.Contains(activeW.Body.String(), `"id":`+strconv.FormatInt(accepted.JobID, 10)) {
		t.Fatalf("aktif işe yeniden bağlanılamadı: HTTP %d: %s", activeW.Code, activeW.Body.String())
	}
	cancelReq := httptest.NewRequest(http.MethodDelete, "/", nil)
	cancelRoute := chi.NewRouteContext()
	cancelRoute.URLParams.Add("id", "1")
	cancelRoute.URLParams.Add("jid", strconv.FormatInt(accepted.JobID, 10))
	cancelReq = cancelReq.WithContext(context.WithValue(cancelReq.Context(), chi.RouteCtxKey, cancelRoute))
	cancelW := httptest.NewRecorder()
	h.CancelRestoreJob(cancelW, cancelReq)
	if cancelW.Code != http.StatusAccepted {
		t.Fatalf("queued restore iptal edilmedi: HTTP %d: %s", cancelW.Code, cancelW.Body.String())
	}
	restoreIntegrationWaitJob(t, db, accepted.JobID, "cancelled", 10*time.Second)
	if got := readTenantFile("public_html/index.txt"); got != "geri-donulecek-canli-surum" {
		t.Fatalf("queued iptal dosyayı değiştirdi: %q", got)
	}
	<-restoreSlots
	<-restoreSlots
	slotsReleased = true
}

type restoreIntegrationJob struct {
	status, message, recovery string
}

func restoreIntegrationWaitJob(t *testing.T, db *sql.DB, id int64, want string, timeout time.Duration) restoreIntegrationJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var job restoreIntegrationJob
		err := db.QueryRow(`SELECT status,message,recovery_file FROM backup_restore_jobs WHERE id=?`, id).
			Scan(&job.status, &job.message, &job.recovery)
		if err != nil {
			t.Fatal(err)
		}
		if job.status == want {
			return job
		}
		if job.status == "failed" || job.status == "cancelled" || job.status == "rolled_back" {
			t.Fatalf("iş %s yerine %s oldu: %s", want, job.status, job.message)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("iş %s durumuna geçmedi", want)
	return restoreIntegrationJob{}
}

func restoreIntegrationIDs(t *testing.T, sk string) (int, int) {
	t.Helper()
	out, err := exec.Command("id", "-u", sk).Output()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command("id", "-g", sk).Output()
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return uid, gid
}

const restoreIntegrationSchema = `
CREATE TABLE domains (
  id BIGINT UNSIGNED PRIMARY KEY,
  alan_adi VARCHAR(255) NOT NULL,
  sistem_kullanici VARCHAR(64) NOT NULL,
  is_demo TINYINT NOT NULL DEFAULT 0
);
CREATE TABLE backups (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  tip VARCHAR(20) NOT NULL,
  dosya VARCHAR(255) NOT NULL,
  boyut_b BIGINT NOT NULL DEFAULT 0,
  notlar VARCHAR(255) NOT NULL DEFAULT '',
  uzak_durum VARCHAR(20) NOT NULL DEFAULT '',
  dogrulama_durum VARCHAR(20) NOT NULL DEFAULT '',
  dogrulama_hata VARCHAR(1000) NOT NULL DEFAULT '',
  dogrulama_sha256 VARCHAR(64) NOT NULL DEFAULT '',
  dogrulama_zamani TIMESTAMP NULL DEFAULT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_restore_it_backup_domain FOREIGN KEY(domain_id) REFERENCES domains(id)
);
CREATE TABLE db_accounts (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  db_name VARCHAR(64) NOT NULL,
  db_user VARCHAR(64) NOT NULL,
  db_pass_plain TEXT NOT NULL
);
CREATE TABLE backup_restore_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  backup_id BIGINT UNSIGNED NOT NULL,
  scope ENUM('full','files','file','database','email') NOT NULL,
  target_path VARCHAR(1024) NOT NULL DEFAULT '',
  database_name VARCHAR(64) NOT NULL DEFAULT '',
  status ENUM('queued','running','success','failed','cancelled','rolled_back') NOT NULL DEFAULT 'queued',
  progress TINYINT UNSIGNED NOT NULL DEFAULT 0,
  message VARCHAR(2000) NOT NULL DEFAULT '',
  result VARCHAR(2000) NOT NULL DEFAULT '',
  recovery_file VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP NULL DEFAULT NULL,
  finished_at TIMESTAMP NULL DEFAULT NULL,
  CONSTRAINT fk_restore_it_job_domain FOREIGN KEY(domain_id) REFERENCES domains(id),
  CONSTRAINT fk_restore_it_job_backup FOREIGN KEY(backup_id) REFERENCES backups(id)
);`
