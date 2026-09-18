package git

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"sanalcp/internal/jailpath"
)

// HealLegacyRepoCredentials removes the redundant cleartext URL credentials.
// The encrypted github_connections PAT remains the sole credential source.
// Every startup also retries file cleanup, including rows already migrated.
func HealLegacyRepoCredentials(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT g.id,g.repo_url,g.target_dir,d.sistem_kullanici FROM git_repos g JOIN domains d ON d.id=g.domain_id`)
	if err != nil {
		return errors.New("git kimlik göçü kayıtları okunamadı")
	}
	type record struct {
		id              int64
		raw, target, sk string
	}
	var records []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.raw, &r.target, &r.sk); err != nil {
			rows.Close()
			return errors.New("git kimlik göçü kaydı okunamadı")
		}
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.New("git kimlik göçü okunamadı")
	}
	var failed []string
	for _, r := range records {
		clean := cleanRepoURL(r.raw)
		if clean != r.raw {
			if _, err := db.ExecContext(ctx, `UPDATE git_repos SET repo_url=? WHERE id=? AND repo_url=?`, clean, r.id, r.raw); err != nil {
				failed = append(failed, fmt.Sprintf("DB:%d", r.id))
			}
		}
		home, err := jailpath.TenantHome(r.sk)
		if err == nil {
			err = scrubRepoConfig(home, r.target)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			failed = append(failed, fmt.Sprintf("dosya:%d", r.id))
		}
	}
	if len(failed) != 0 {
		return fmt.Errorf("git kimlik göçü tamamlanamadı; kayıtlar: %s", strings.Join(failed, ","))
	}
	return nil
}

func scrubRepoConfig(home, target string) error {
	if !gecerliTargetDir(target) {
		return errors.New("geçersiz Git hedef dizini")
	}
	root := filepath.Join(target, ".git")
	for _, name := range []string{"config", "FETCH_HEAD"} {
		if err := scrubGitFile(home, filepath.Join(root, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("Git kimlik dosyası temizlenemedi")
		}
	}
	if err := scrubGitLogs(home, filepath.Join(root, "logs"), 0); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("Git işlem geçmişi temizlenemedi")
	}
	return nil
}

func scrubGitLogs(home, rel string, depth int) error {
	if depth > 32 {
		return errors.New("Git geçmişi dizin sınırı aşıldı")
	}
	dir, err := jailpath.AcDizin(home, rel)
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(rel, entry.Name())
		if entry.IsDir() {
			err = scrubGitLogs(home, child, depth+1)
		} else {
			err = scrubGitFile(home, child)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Pin the parent, lock like Git itself, and atomically replace only this inode.
// Existing symlink/hardlink/special files are refused; no root Git command runs.
func scrubGitFile(home, rel string) error {
	dir, err := jailpath.AcDizin(home, filepath.Dir(rel))
	if err != nil {
		return err
	}
	defer dir.Close()
	dfd, name := int(dir.Fd()), filepath.Base(rel)
	lockName := name + ".lock"
	lockfd, err := unix.Openat(dfd, lockName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	lock := os.NewFile(uintptr(lockfd), lockName)
	defer lock.Close()
	defer unix.Unlinkat(dfd, lockName, 0)
	fd, err := unix.Openat(dfd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Size > 16<<20 {
		return errors.New("güvensiz veya çok büyük Git dosyası")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if err != nil || len(raw) > 16<<20 {
		return errors.New("Git dosyası okunamadı")
	}
	clean := credentialURL.ReplaceAllString(string(raw), "${1}")
	if clean == string(raw) {
		return nil
	}
	if _, err := lock.WriteString(clean); err != nil {
		return err
	}
	if err := lock.Chown(int(st.Uid), int(st.Gid)); err != nil {
		return err
	}
	if err := lock.Chmod(os.FileMode(st.Mode & 0777)); err != nil {
		return err
	}
	if err := lock.Sync(); err != nil {
		return err
	}
	return unix.Renameat(dfd, lockName, dfd, name)
}
