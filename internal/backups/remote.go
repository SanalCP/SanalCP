package backups

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sanalcp/internal/adlar"
	"time"
)

func ensureLocalBackup(ctx context.Context, db *sql.DB, domainID int64, sk, fileName, remoteState string) (string, error) {
	if !adlar.SKGecerli(sk) || filepath.Base(fileName) != fileName {
		return "", fmt.Errorf("güvensiz yedek yolu")
	}
	dir := filepath.Join(BackupRoot, sk)
	localPath := filepath.Join(dir, fileName)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}
	if remoteState != "basarili" {
		return "", fmt.Errorf("yedek yerel diskte yok ve kullanılabilir uzak kopya bulunamadı")
	}
	d, err := readDestination(ctx, db, domainID)
	if err != nil || d == nil {
		return "", fmt.Errorf("uzak hedef okunamadı")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err := downloadFromRemote(downloadCtx, db, d, fileName, localPath); err != nil {
		return "", err
	}
	return localPath, nil
}

func deleteRemoteBestEffort(db *sql.DB, domainID int64, fileName, remoteState string) {
	if remoteState != "basarili" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	d, err := readDestination(ctx, db, domainID)
	if err != nil || d == nil {
		return
	}
	_ = deleteFromRemote(ctx, db, d, fileName)
}
