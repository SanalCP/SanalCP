package backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var mysqlNameRE = regexp.MustCompile(`^[A-Za-z0-9_$]+$`)

type backupManifest struct {
	Version   int      `json:"version"`
	Domain    string   `json:"domain"`
	User      string   `json:"system_user"`
	CreatedAt string   `json:"created_at"`
	Databases []string `json:"databases"`
	HasMail   bool     `json:"has_mail"`
}

// createArchive builds one consistent archive for manual and scheduled backups.
// Layout remains backwards compatible (top-level c_* home), while databases/
// and manifest.json make selective restore deterministic.
func createArchive(ctx context.Context, db *sql.DB, domainID int64, domain, sk, filePath string) (int64, error) {
	if !strings.HasPrefix(sk, "c_") {
		return 0, fmt.Errorf("güvensiz sistem kullanıcısı: %s", sk)
	}
	stage, err := os.MkdirTemp("", "sanal-backup-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)
	dbDir := filepath.Join(stage, "databases")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return 0, err
	}

	databases, err := domainDatabases(ctx, db, domainID, sk)
	if err != nil {
		return 0, err
	}
	for _, name := range databases {
		dumpPath := filepath.Join(dbDir, name+".sql")
		out, err := os.Create(dumpPath)
		if err != nil {
			return 0, err
		}
		cmd := exec.CommandContext(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", name)
		cmd.Stdout = out
		var stderr strings.Builder
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		closeErr := out.Close()
		if runErr != nil {
			return 0, fmt.Errorf("mysqldump %s: %s: %w", name, strings.TrimSpace(stderr.String()), runErr)
		}
		if closeErr != nil {
			return 0, closeErr
		}
	}

	_, mailErr := os.Stat(filepath.Join("/home", sk, "mail"))
	manifest := backupManifest{
		Version: 2, Domain: domain, User: sk, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Databases: databases, HasMail: mailErr == nil,
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), raw, 0o600); err != nil {
		return 0, err
	}

	args := []string{"czf", filePath, "-C", "/home", sk, "-C", stage, "manifest.json", "databases"}
	if out, err := exec.CommandContext(ctx, "tar", args...).CombinedOutput(); err != nil {
		_ = os.Remove(filePath)
		return 0, fmt.Errorf("tar: %s: %w", strings.TrimSpace(string(out)), err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func domainDatabases(ctx context.Context, db *sql.DB, domainID int64, sk string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT db_name FROM db_accounts WHERE domain_id=? ORDER BY db_name`, domainID)
	if err != nil {
		return nil, fmt.Errorf("veritabanları listelenemedi: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !mysqlNameRE.MatchString(name) {
			return nil, fmt.Errorf("güvensiz veritabanı adı: %q", name)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Eski kurulumlarda metadata satırı olmadan oluşturulmuş varsayılan DB'yi de koru.
	mainName := sk + "_main"
	var exists int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?`, mainName).Scan(&exists); err == nil && exists > 0 {
		found := false
		for _, name := range names {
			if name == mainName {
				found = true
				break
			}
		}
		if !found {
			names = append(names, mainName)
		}
	}
	return names, nil
}
