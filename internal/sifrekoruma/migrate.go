package sifrekoruma

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type protectedEntry struct {
	id                        int64
	domainID                  int64
	sk, php, path, user, file string
}

// HealLegacyFiles separates the users of old shared password files before the
// API accepts mutations. Re-render every protected domain, including already
// migrated rows: a crash between the DB commit and nginx reload is retryable.
func HealLegacyFiles(ctx context.Context, db *sql.DB) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	entries, err := migrateFiles(ctx, db, htpasswdDir)
	if err != nil {
		return err
	}
	h := &Handlers{DB: db}
	seen := map[int64]bool{}
	for _, e := range entries {
		if seen[e.domainID] {
			continue
		}
		seen[e.domainID] = true
		if err := h.reRender(e.domainID, e.sk, e.php); err != nil {
			return fmt.Errorf("domain %d koruma yapılandırması: %w", e.domainID, err)
		}
	}
	return nil
}

func migrateFiles(ctx context.Context, db *sql.DB, dir string) ([]protectedEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT k.id, k.domain_id, d.sistem_kullanici, COALESCE(d.php_surum,'8.3'), k.yol, k.kullanici, k.htpasswd_dosya
 FROM korumali_dizinler k JOIN domains d ON d.id=k.domain_id ORDER BY k.domain_id, k.yol, k.kullanici`)
	if err != nil {
		return nil, err
	}
	var entries []protectedEntry
	for rows.Next() {
		var e protectedEntry
		if err := rows.Scan(&e.id, &e.domainID, &e.sk, &e.php, &e.path, &e.user, &e.file); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	groups := map[string][]protectedEntry{}
	for _, e := range entries {
		if e.file == passwordFile(dir, e.domainID, e.path) {
			continue
		}
		// Only panel-managed legacy files in the root-owned directory.
		if filepath.Dir(e.file) != dir || !strings.HasPrefix(filepath.Base(e.file), fmt.Sprintf("d%d_", e.domainID)) {
			return nil, fmt.Errorf("koruma kaydı %d: beklenmeyen parola dosyası", e.id)
		}
		groups[e.file] = append(groups[e.file], e)
	}
	for legacy, group := range groups {
		data, err := readPasswordFile(legacy)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		files, ambiguous := splitLegacyPasswords(dir, group, data)
		for name, body := range files {
			// A previous failed DB transaction may already have prepared this
			// file. Rebuilding from the unchanged legacy file is idempotent.
			if err := atomicPasswordFile(name, body); err != nil {
				return nil, err
			}
			if _, err := exec.LookPath("restorecon"); err == nil {
				c, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = exec.CommandContext(c, "restorecon", name).Run()
				cancel()
				if err != nil {
					return nil, err
				}
			}
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		for _, e := range group {
			if _, err = tx.ExecContext(ctx, `UPDATE korumali_dizinler SET htpasswd_dosya=? WHERE id=?`, passwordFile(dir, e.domainID, e.path), e.id); err != nil {
				break
			}
		}
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		// Old nginx workers still reference this file until reload. Deny
		// authentication there instead of leaving cross-directory access open.
		if err := atomicPasswordFile(legacy, nil); err != nil {
			return nil, err
		}
		if ambiguous > 0 {
			log.Printf("koruma göçü: %s içinde %d çakışan kullanıcı kaydı kilitlendi; dizin parolaları yeniden ayarlanmalı", filepath.Base(legacy), ambiguous)
		}
	}
	return entries, nil
}

func readPasswordFile(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("parola dosyası çok büyük")
	}
	return b, err
}

// A username reused in colliding paths has only one hash left in the legacy
// file. Its original password cannot be recovered safely; lock it until reset.
func splitLegacyPasswords(dir string, entries []protectedEntry, data []byte) (map[string][]byte, int) {
	hashes := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if user, hash, ok := strings.Cut(line, ":"); ok {
			hashes[user] = hash
		}
	}
	users := map[string]map[string]bool{}
	for _, e := range entries {
		if users[e.user] == nil {
			users[e.user] = map[string]bool{}
		}
		users[e.user][passwordFile(dir, e.domainID, e.path)] = true
	}
	lines := map[string][]string{}
	ambiguous := 0
	for _, e := range entries {
		hash := hashes[e.user]
		if hash == "" || len(users[e.user]) > 1 {
			hash = "!"
			ambiguous++
		}
		name := passwordFile(dir, e.domainID, e.path)
		lines[name] = append(lines[name], e.user+":"+hash+"\n")
	}
	files := map[string][]byte{}
	for name, l := range lines {
		sort.Strings(l)
		files[name] = []byte(strings.Join(l, ""))
	}
	return files, ambiguous
}

func atomicPasswordFile(name string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(name), ".htpasswd-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(0644); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}
