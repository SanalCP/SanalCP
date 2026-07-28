package backups

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"sanalpanel/internal/archivex"
	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type restoreRequest struct {
	Scope    string `json:"scope"`    // full | files | file | database | email
	Path     string `json:"path"`     // file: /home/<sk> altındaki göreli yol
	Database string `json:"database"` // database: hedef DB adı
}

// Restore supports a complete account restore as well as web files, one file,
// one database, or Maildir-only recovery. An empty body remains "full" for API
// backwards compatibility.
func (h *Handlers) Restore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bid, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)

	var sk, dosya, alanAdi, uzakDurum string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.sistem_kullanici, d.alan_adi, d.is_demo, b.dosya, b.uzak_durum
		 FROM backups b JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, bid, id).
		Scan(&sk, &alanAdi, &isDemo, &dosya, &uzakDurum)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "yedek bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe geri yükleme yapılamaz")
		return
	}
	if !strings.HasPrefix(sk, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "güvenlik")
		return
	}

	req := restoreRequest{Scope: "full"}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz geri yükleme isteği")
			return
		}
	}
	if req.Scope == "" {
		req.Scope = "full"
	}
	switch req.Scope {
	case "full", "files", "file", "database", "email":
	default:
		httpx.WriteError(w, http.StatusBadRequest, "scope: full|files|file|database|email")
		return
	}

	abs, err := ensureLocalBackup(r.Context(), h.DB, id, sk, dosya, uzakDurum)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	tmpDir, err := os.MkdirTemp("", "sanal-restore-*")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	_, _ = exec.Command("chown", sk+":"+sk, tmpDir).CombinedOutput()
	if out, err := archivex.GuvenliCikar(abs, tmpDir, sk); err != nil {
		msg := err.Error()
		if strings.TrimSpace(out) != "" {
			msg += ": " + strings.TrimSpace(out)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "tar extract: "+msg)
		return
	}

	extractedHome := filepath.Join(tmpDir, sk)
	result := ""
	switch req.Scope {
	case "full":
		if err := restoreTree(extractedHome, filepath.Join("/home", sk), sk, true); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		imported, err := restoreAllDatabases(r, h.DB, id, tmpDir, sk)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = fmt.Sprintf("tüm hesap geri yüklendi; %d veritabanı içe aktarıldı", imported)
	case "files":
		if err := restoreTree(filepath.Join(extractedHome, "public_html"),
			filepath.Join("/home", sk, "public_html"), sk, true); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = "web dosyaları geri yüklendi"
	case "email":
		if err := restoreTree(filepath.Join(extractedHome, "mail"),
			filepath.Join("/home", sk, "mail"), sk, true); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = "e-posta kutuları geri yüklendi"
	case "file":
		rel, err := safeRestoreRelativePath(req.Path)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := restoreSingle(filepath.Join(extractedHome, rel),
			filepath.Join("/home", sk, rel), sk); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = rel + " geri yüklendi"
	case "database":
		if !mysqlNameRE.MatchString(req.Database) {
			httpx.WriteError(w, http.StatusBadRequest, "geçerli bir veritabanı seçin")
			return
		}
		if err := authorizeDatabase(r, h.DB, id, sk, req.Database); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		dump, err := findDatabaseDump(tmpDir, req.Database)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		if err := importDatabase(dump, req.Database); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = req.Database + " geri yüklendi"
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "alan_adi": alanAdi, "dosya": dosya,
		"scope": req.Scope, "sonuc": result, "db_import": result,
	})
}

func safeRestoreRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
	clean := filepath.Clean(value)
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("dosya yolu /home hesabına göre verilmelidir")
	}
	return clean, nil
}

func restoreTree(source, target, sk string, deleteMissing bool) error {
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return fmt.Errorf("seçilen bölüm bu yedekte bulunamadı")
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	args := []string{"-a"}
	if deleteMissing {
		args = append(args, "--delete")
	}
	args = append(args, source+"/", target+"/")
	if out, err := exec.Command("rsync", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("rsync: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_, _ = exec.Command("chown", "-R", sk+":"+sk, target).CombinedOutput()
	_, _ = exec.Command("restorecon", "-R", target).CombinedOutput()
	return nil
}

func restoreSingle(source, target, sk string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("seçilen dosya yedekte bulunamadı")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("yalnız normal bir dosya geri yüklenebilir")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	_, _ = exec.Command("chown", sk+":"+sk, target).CombinedOutput()
	_, _ = exec.Command("restorecon", target).CombinedOutput()
	return nil
}

// restoreAllDatabases — "full" kapsamında yedekteki bütün dump'ları geri yükler.
//
// 🔴 HER DUMP AYRI AYRI YETKİLENDİRİLİR: adın yalnızca biçimsel olarak geçerli
// olması yetmez. Arşiv adı ne olursa olsun `mysql <ad>` root yetkisiyle çalışır;
// hedef bu domaine ait değilse başka bir kiracının (hatta panelin kendi) şemasını
// ezerdi. Tekil geri yüklemedeki authorizeDatabase kontrolünün aynısı burada da
// uygulanır — eşleşmeyen dump atlanır ve çağırana bildirilir.
func restoreAllDatabases(r *http.Request, db *sql.DB, domainID int64, tmpDir, sk string) (int, error) {
	dbDir := filepath.Join(tmpDir, "databases")
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		// Legacy v1 backup: root-level *.sql was the default database.
		matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.sql"))
		if len(matches) == 0 {
			return 0, nil
		}
		if err := importDatabase(matches[0], sk+"_main"); err != nil {
			return 0, err
		}
		return 1, nil
	}
	count := 0
	var atlanan []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".sql")
		if !mysqlNameRE.MatchString(name) {
			return count, fmt.Errorf("yedekte geçersiz veritabanı adı: %q", name)
		}
		if err := authorizeDatabase(r, db, domainID, sk, name); err != nil {
			atlanan = append(atlanan, name)
			continue
		}
		if err := importDatabase(filepath.Join(dbDir, entry.Name()), name); err != nil {
			return count, err
		}
		count++
	}
	if len(atlanan) > 0 {
		return count, fmt.Errorf("bu domaine ait olmayan veritabanları geri yüklenmedi: %s",
			strings.Join(atlanan, ", "))
	}
	return count, nil
}

func findDatabaseDump(tmpDir, name string) (string, error) {
	current := filepath.Join(tmpDir, "databases", name+".sql")
	if _, err := os.Stat(current); err == nil {
		return current, nil
	}
	if name != "" && strings.HasSuffix(name, "_main") {
		matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.sql"))
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("%s veritabanı bu yedekte bulunamadı", name)
}

func importDatabase(dumpPath, database string) error {
	in, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer in.Close()
	cmd := exec.Command("mysql", database)
	cmd.Stdin = in
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysql %s: %s: %w", database, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func authorizeDatabase(r *http.Request, db *sql.DB, domainID int64, sk, database string) error {
	if database == sk+"_main" {
		return nil
	}
	var count int
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`,
		domainID, database).Scan(&count); err != nil || count == 0 {
		return fmt.Errorf("veritabanı bu domaine ait değil")
	}
	return nil
}
