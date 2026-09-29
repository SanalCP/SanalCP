package appruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sanalcp/internal/jailpath"
)

func unitDirective(body []byte, key string) string {
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func unitNeedsRecovery(current, expected []byte, workDir string, pending bool) bool {
	return pending || unitDirective(current, "WorkingDirectory") != workDir || unitDirective(current, "ExecStart") != unitDirective(expected, "ExecStart")
}

// Reconcile releases left between the systemd switch and DB commit by a
// process crash. The committed DB release is authoritative; its unit is
// restored before further application mutations run.
func Reconcile(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT domain_id FROM app_runtimes WHERE enabled=1`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	h := &Handlers{DB: db}
	var failures []error
	for _, id := range ids {
		if err := reconcileOne(ctx, h, id); err != nil {
			failures = append(failures, fmt.Errorf("site %d: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

func reconcileOne(ctx context.Context, h *Handlers, id int64) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	s, err := h.lookup(ctx, id)
	if err != nil {
		return err
	}
	c, exists, err := h.read(ctx, id)
	if err != nil || !exists {
		return err
	}
	workDir, err := releasePath(s, c.ReleaseDir)
	if err != nil {
		return err
	}
	if c.ReleaseDir == "" {
		if err := jailpath.DizinDogrula(filepath.Join("/home", s.sk), "public_html"); err != nil {
			return err
		}
	}
	if !entryRE.MatchString(c.Entrypoint) || strings.Contains(c.Entrypoint, "..") || !healthRE.MatchString(healthPath(c.HealthPath)) {
		return errors.New("kayıtlı uygulama yolu geçersiz")
	}
	binary := selectedBinary(c.Runtime, c.Interpreter)
	if binary == "" {
		return errors.New("kayıtlı yorumlayıcı bulunamadı")
	}
	entry := filepath.Join("/home", s.sk, filepath.FromSlash(c.Entrypoint))
	expected := []byte(unit(s, c, binary, entry))
	current, err := os.ReadFile(unitPath(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	_, pendingErr := os.Lstat(pendingPath(id))
	if pendingErr != nil && !os.IsNotExist(pendingErr) {
		return pendingErr
	}
	pending := pendingErr == nil
	if err == nil && !unitNeedsRecovery(current, expected, workDir, pending) {
		return nil
	}
	if err := atomicWrite(unitPath(id), expected); err != nil {
		return err
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if s.suspended {
		if err := systemctl(ctx, "disable", "--now", unitName(id)); err != nil {
			return err
		}
		if pending {
			if err := os.Remove(pendingPath(id)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	if err := systemctl(ctx, "enable", unitName(id)); err != nil {
		return err
	}
	if err := systemctl(ctx, "restart", unitName(id)); err != nil {
		return err
	}
	if err := waitHTTP(ctx, id, s.domain, c.Port, c.HealthPath); err != nil {
		return err
	}
	if pending {
		if err := os.Remove(pendingPath(id)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
