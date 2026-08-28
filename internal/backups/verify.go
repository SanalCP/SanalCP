package backups

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/archivex"
)

type verificationResult struct {
	SHA256 string
	Detail string
}

// verifyArchive proves that the archive is readable, jail-safe, belongs to the
// expected tenant, contains a usable home tree and structurally valid SQL dumps.
func verifyArchive(ctx context.Context, archive, domain, sk string) (verificationResult, error) {
	if !adlar.SKGecerli(sk) {
		return verificationResult{}, fmt.Errorf("geçersiz sistem kullanıcısı")
	}
	f, err := os.Open(archive)
	if err != nil {
		return verificationResult{}, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		f.Close()
		return verificationResult{}, err
	}
	f.Close()
	sha := hex.EncodeToString(h.Sum(nil))

	tmp, err := os.MkdirTemp("", "sanal-backup-verify-*")
	if err != nil {
		return verificationResult{}, err
	}
	defer os.RemoveAll(tmp)
	if out, e := exec.CommandContext(ctx, "chown", sk+":"+sk, tmp).CombinedOutput(); e != nil {
		return verificationResult{}, fmt.Errorf("geçici doğrulama dizini: %s: %w", strings.TrimSpace(string(out)), e)
	}
	if out, e := archivex.GuvenliCikar(archive, tmp, sk); e != nil {
		return verificationResult{}, fmt.Errorf("arşiv güvenli açılamadı: %s: %w", strings.TrimSpace(out), e)
	}
	home := filepath.Join(tmp, sk)
	if st, e := os.Stat(home); e != nil || !st.IsDir() {
		return verificationResult{}, fmt.Errorf("hesap dosya ağacı eksik")
	}
	type dbDump struct{ name, path string }
	var dumps []dbDump
	raw, manifestErr := os.ReadFile(filepath.Join(tmp, "manifest.json"))
	if manifestErr == nil {
		var m backupManifest
		if err = json.Unmarshal(raw, &m); err != nil {
			return verificationResult{}, fmt.Errorf("manifest bozuk: %w", err)
		}
		if m.Version < 2 || m.User != sk || !strings.EqualFold(m.Domain, domain) {
			return verificationResult{}, fmt.Errorf("manifest domain/kullanıcı eşleşmiyor")
		}
		for _, name := range m.Databases {
			dumps = append(dumps, dbDump{name, filepath.Join(tmp, "databases", name+".sql")})
		}
	} else {
		// Eski v1/cron arşivleri manifest taşımaz; kesin tenant kök dizini ve
		// kökteki dump.sql üzerinden geriye dönük, yine fail-closed doğrulanır.
		if _, e := os.Stat(filepath.Join(tmp, "dump.sql")); e == nil {
			dumps = append(dumps, dbDump{sk + "_main", filepath.Join(tmp, "dump.sql")})
		}
	}
	for _, dump := range dumps {
		name, p := dump.name, dump.path
		if !mysqlNameRE.MatchString(name) {
			return verificationResult{}, fmt.Errorf("geçersiz DB adı: %q", name)
		}
		st, e := os.Stat(p)
		if e != nil || st.Size() == 0 {
			return verificationResult{}, fmt.Errorf("%s SQL dump eksik/boş", name)
		}
		fd, e := os.Open(p)
		if e != nil {
			return verificationResult{}, e
		}
		buf := make([]byte, 256<<10)
		n, _ := fd.Read(buf)
		fd.Close()
		s := strings.ToUpper(string(buf[:n]))
		if !strings.Contains(s, "CREATE TABLE") && !strings.Contains(s, "INSERT INTO") && !strings.Contains(s, "MYSQL DUMP") {
			return verificationResult{}, fmt.Errorf("%s SQL dump yapısı doğrulanamadı", name)
		}
		if err := restoreDrillSQL(ctx, p); err != nil {
			return verificationResult{}, fmt.Errorf("%s geçici geri yükleme tatbikatı: %w", name, err)
		}
	}
	return verificationResult{SHA256: sha, Detail: fmt.Sprintf("arşiv güvenli açıldı; %d veritabanı geçici şemaya geri yüklendi", len(dumps))}, nil
}

func restoreDrillSQL(ctx context.Context, dump string) error {
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	tmpDB := "sanal_verify_" + suffix
	tmpUser := "sv_" + suffix
	secretRaw := make([]byte, 24)
	if _, err := rand.Read(secretRaw); err != nil {
		return err
	}
	tmpPass := base64.RawURLEncoding.EncodeToString(secretRaw)
	if !mysqlNameRE.MatchString(tmpDB) {
		return fmt.Errorf("geçici şema adı geçersiz")
	}
	setupSQL := "CREATE DATABASE `" + tmpDB + "`; CREATE USER '" + tmpUser + "'@'localhost' IDENTIFIED BY '" + tmpPass + "'; GRANT ALL ON `" + tmpDB + "`.* TO '" + tmpUser + "'@'localhost'"
	create := exec.CommandContext(ctx, "mysql", "-e", setupSQL)
	if out, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("geçici şema oluşturulamadı: %s", strings.TrimSpace(string(out)))
	}
	defer exec.Command("mysql", "-e", "DROP DATABASE IF EXISTS `"+tmpDB+"`; DROP USER IF EXISTS '"+tmpUser+"'@'localhost'").Run()
	f, err := os.Open(dump)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "mysql", "--user="+tmpUser, tmpDB)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+tmpPass)
	cmd.Stdin = f
	out, runErr := cmd.CombinedOutput()
	f.Close()
	if runErr != nil {
		return fmt.Errorf("SQL açılamadı: %s", strings.TrimSpace(string(out)))
	}
	check := exec.CommandContext(ctx, "mysql", "-N", "-B", "-e", "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+tmpDB+"'")
	raw, err := check.Output()
	if err != nil {
		return fmt.Errorf("tablo kontrolü başarısız")
	}
	if strings.TrimSpace(string(raw)) == "0" {
		return fmt.Errorf("geçici şemada tablo oluşmadı")
	}
	return nil
}

func verifyAndRecord(ctx context.Context, db *sql.DB, id, domainID int64, domain, sk, archive string) error {
	return verifyAndRecordExpected(ctx, db, id, domainID, domain, sk, archive, "")
}

func verifyAndRecordExpected(ctx context.Context, db *sql.DB, id, domainID int64, domain, sk, archive, expectedSHA string) error {
	_, _ = db.ExecContext(ctx, `UPDATE backups SET dogrulama_durum='dogrulaniyor',dogrulama_hata='' WHERE id=? AND domain_id=?`, id, domainID)
	res, err := verifyArchive(ctx, archive, domain, sk)
	if err == nil && expectedSHA != "" && !strings.EqualFold(expectedSHA, res.SHA256) {
		err = fmt.Errorf("yedek SHA-256 özeti önceki doğrulamayla eşleşmiyor")
	}
	if err != nil {
		_, _ = db.ExecContext(context.Background(), `UPDATE backups SET dogrulama_durum='basarisiz',dogrulama_hata=?,dogrulama_zamani=NOW() WHERE id=? AND domain_id=?`, truncateVerifyError(err.Error()), id, domainID)
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE backups SET dogrulama_durum='dogrulandi',dogrulama_hata=?,dogrulama_sha256=?,dogrulama_zamani=NOW() WHERE id=? AND domain_id=?`, res.Detail, res.SHA256, id, domainID)
	return err
}

func truncateVerifyError(s string) string {
	if len(s) > 950 {
		return s[:950]
	}
	return s
}

func startVerification(db *sql.DB, id, domainID int64, domain, sk, archive string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = verifyAndRecord(ctx, db, id, domainID, domain, sk, archive)
	}()
}

// verifyLatestBackups re-proves the newest backup of each domain once stale.
// It is deliberately sequential to avoid saturating disk and MariaDB.
func verifyLatestBackups(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT b.id,b.domain_id,d.alan_adi,d.sistem_kullanici,b.dosya,b.uzak_durum FROM backups b JOIN domains d ON d.id=b.domain_id JOIN (SELECT domain_id,MAX(id) id FROM backups GROUP BY domain_id) x ON x.id=b.id WHERE b.dogrulama_zamani IS NULL OR b.dogrulama_zamani < DATE_SUB(NOW(),INTERVAL 7 DAY)`)
	if err != nil {
		return
	}
	type item struct {
		id, domainID             int64
		domain, sk, file, remote string
	}
	var all []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.id, &x.domainID, &x.domain, &x.sk, &x.file, &x.remote) == nil {
			all = append(all, x)
		}
	}
	rows.Close()
	for _, x := range all {
		abs, e := ensureLocalBackup(ctx, db, x.domainID, x.sk, x.file, x.remote)
		if e != nil {
			_, _ = db.Exec(`UPDATE backups SET dogrulama_durum='basarisiz',dogrulama_hata=?,dogrulama_zamani=NOW() WHERE id=?`, truncateVerifyError(e.Error()), x.id)
			continue
		}
		_ = verifyAndRecord(ctx, db, x.id, x.domainID, x.domain, x.sk, abs)
	}
}
