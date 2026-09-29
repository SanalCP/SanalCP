package appruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

type StagedRelease struct {
	Commit     string
	ReleaseDir string // tenant-home-relative, under .sanalcp/releases/<domain-id>/
	Log        string
}

type GitStage func(context.Context) (StagedRelease, error)
type GitDiscard func(context.Context, string) error

func pendingPath(id int64) string { return unitPath(id) + ".pending" }

// DeployGitAtomic builds in a detached Git worktree. The running release and
// its dependencies are untouched until the new service definition is switched.
// On failure the old unit is restored; its release remains available.
func DeployGitAtomic(ctx context.Context, db *sql.DB, id int64, sk, targetDir string, legacyUpdate GitUpdate, stage GitStage, discard GitDiscard) (DeployResult, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	var result DeployResult
	h := &Handlers{DB: db}
	c, exists, err := h.read(ctx, id)
	if err != nil {
		return result, err
	}
	if !exists {
		result.Commit, result.Log, err = legacyUpdate(ctx)
		return result, err
	}
	result.Managed = true
	if _, err := os.Lstat(pendingPath(id)); err == nil {
		return result, errors.New("önceki dağıtımın açılış onarımı tamamlanmadı")
	} else if !os.IsNotExist(err) {
		return result, err
	}
	s, err := h.lookup(ctx, id)
	if err != nil {
		return result, err
	}
	if s.sk != sk || s.backend != "reverse-proxy" || s.demo || s.suspended || targetDir != "public_html" {
		return result, errors.New("yönetilen uygulamada Git hedefi public_html olmalı ve site etkin olmalı")
	}
	if !c.Enabled || !active(ctx, id) {
		return result, errors.New("dağıtım öncesinde uygulama çalışıyor olmalı")
	}
	if _, err := releasePath(s, c.ReleaseDir); err != nil {
		return result, err
	}
	if err := probeHTTP(ctx, s.domain, c.Port, c.HealthPath); err != nil {
		return result, fmt.Errorf("mevcut uygulama sağlıklı değil: %w", err)
	}
	staged, err := stage(ctx)
	result.Log = staged.Log
	if err != nil {
		return result, err
	}
	if !commitRE.MatchString(staged.Commit) {
		err = errors.New("yeni commit geçersiz")
	}
	if err == nil {
		_, err = releasePath(s, staged.ReleaseDir)
	}
	if err != nil {
		if staged.ReleaseDir != "" {
			_ = discard(context.Background(), staged.ReleaseDir)
		}
		return result, err
	}
	oldBody, err := os.ReadFile(unitPath(id))
	if err != nil {
		_ = discard(context.Background(), staged.ReleaseDir)
		return result, err
	}
	newConfig := c
	newConfig.ReleaseDir = staged.ReleaseDir
	binary := selectedBinary(c.Runtime, c.Interpreter)
	if binary == "" {
		_ = discard(context.Background(), staged.ReleaseDir)
		return result, errors.New("seçili yorumlayıcı bulunamadı")
	}
	newBody := []byte(unit(s, newConfig, binary, ""))
	result, err = runReleaseTransition(ctx, staged, c.ReleaseDir, releaseHooks{
		prepare: func(stepCtx context.Context) (string, error) {
			return prepareManagedAt(stepCtx, s, c, staged.ReleaseDir)
		},
		switchTo: func(stepCtx context.Context) error {
			if err := atomicWrite(pendingPath(id), []byte(staged.ReleaseDir+"\n")); err != nil {
				return err
			}
			if err := atomicWrite(unitPath(id), newBody); err != nil {
				return err
			}
			if err := systemctl(stepCtx, "daemon-reload"); err != nil {
				return err
			}
			return systemctl(stepCtx, "restart", unitName(id))
		},
		health: func(stepCtx context.Context) error { return waitHTTP(stepCtx, id, s.domain, c.Port, c.HealthPath) },
		persist: func(stepCtx context.Context) error {
			res, err := db.ExecContext(stepCtx, `UPDATE app_runtimes SET release_dir=?, interpreter=IF(interpreter='',?,interpreter) WHERE domain_id=?`, staged.ReleaseDir, binary, id)
			if err != nil {
				return err
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rows != 1 {
				return errors.New("uygulama kaydı bulunamadı")
			}
			return nil
		},
		restore: func(stepCtx context.Context) error {
			if err := atomicWrite(unitPath(id), oldBody); err != nil {
				return err
			}
			if err := systemctl(stepCtx, "daemon-reload"); err != nil {
				return err
			}
			if err := systemctl(stepCtx, "restart", unitName(id)); err != nil {
				return err
			}
			return waitHTTP(stepCtx, id, s.domain, c.Port, c.HealthPath)
		},
		discard: discard,
	})
	if err == nil || result.RolledBack {
		if removeErr := os.Remove(pendingPath(id)); removeErr != nil && !os.IsNotExist(removeErr) {
			result.Log += "\nAçılış onarımı işareti temizlenemedi: " + removeErr.Error()
		}
	}
	return result, err
}

type releaseHooks struct {
	prepare  func(context.Context) (string, error)
	switchTo func(context.Context) error
	health   func(context.Context) error
	persist  func(context.Context) error
	restore  func(context.Context) error
	discard  GitDiscard
}

func runReleaseTransition(ctx context.Context, staged StagedRelease, previousRel string, hooks releaseHooks) (DeployResult, error) {
	result := DeployResult{Managed: true, Log: staged.Log}
	cleanup := func(rel string) error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return hooks.discard(cleanupCtx, rel)
	}
	buildLog, err := hooks.prepare(ctx)
	result.Log += buildLog
	if err != nil {
		if cleanupErr := cleanup(staged.ReleaseDir); cleanupErr != nil {
			result.Log += "\nGeçici sürüm temizlenemedi: " + cleanupErr.Error()
		}
		result.RolledBack = true
		return result, fmt.Errorf("yeni sürüm hazırlanamadı; çalışan sürüm korundu: %w", err)
	}
	rollback := func(cause error) (DeployResult, error) {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if restoreErr := hooks.restore(restoreCtx); restoreErr != nil {
			return result, fmt.Errorf("dağıtım: %w; eski sürüme dönüş başarısız: %v", cause, restoreErr)
		}
		result.RolledBack = true
		if cleanupErr := cleanup(staged.ReleaseDir); cleanupErr != nil {
			result.Log += "\nGeçici sürüm temizlenemedi: " + cleanupErr.Error()
		}
		return result, fmt.Errorf("dağıtım: %w; çalışan sürüm geri yüklendi", cause)
	}
	if err := hooks.switchTo(ctx); err != nil {
		return rollback(err)
	}
	if err := hooks.health(ctx); err != nil {
		return rollback(err)
	}
	if err := hooks.persist(ctx); err != nil {
		return rollback(err)
	}
	result.Commit = staged.Commit
	if previousRel != "" && previousRel != staged.ReleaseDir {
		if cleanupErr := cleanup(previousRel); cleanupErr != nil {
			result.Log += "\nÖnceki sürüm temizlenemedi: " + cleanupErr.Error()
		}
	}
	return result, nil
}
