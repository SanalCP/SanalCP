package cliapi

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/sqlimport"
)

type Handlers struct{ DB *sql.DB }

const maxDBImportBytes int64 = 2 << 30

// Aynı anda tek büyük import: her istek sıkıştırılmış + açılmış geçici dosya
// tutabildiği için paralel 2 GiB istekler RAM yerine bu kez diski tüketmesin.
var dbImportGate = make(chan struct{}, 1)

// GET /db/export?databaseName=...&file=...
// "file" sadece uzantıya bakılıp gzip'lenip lenmeyeceğine karar vermek için kullanılır,
// bir dosya yolu olarak SUNUCU TARAFINDA hiç kullanılmaz — path traversal yüzeyi yok.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	// Büyük DB dump indirmeleri sunucunun kısa varsayılan zaman aşımını (bkz.
	// cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 10*time.Minute)
	domainID, _, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	dbName := r.URL.Query().Get("databaseName")
	if !hesaplar.GecerliDBKimlik(dbName) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}
	var exists int
	if err := h.DB.QueryRow(`SELECT 1 FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbName).Scan(&exists); err != nil {
		httpx.WriteError(w, http.StatusForbidden, "bu veritabanı size ait değil")
		return
	}

	gz := strings.HasSuffix(r.URL.Query().Get("file"), ".gz")
	var buf, stderr bytes.Buffer
	cmd := exec.Command("mysqldump", "--single-transaction", dbName)
	cmd.Stderr = &stderr

	if gz {
		gzw := gzip.NewWriter(&buf)
		cmd.Stdout = gzw
		runErr := cmd.Run()
		_ = gzw.Close()
		if runErr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
	} else {
		cmd.Stdout = &buf
		if err := cmd.Run(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
			return
		}
		w.Header().Set("Content-Type", "application/sql")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// POST /db/import?databaseName=... — govde ham SQL veya gzip'li SQL baytlari
// (ilk 2 byte 0x1f 0x8b ise otomatik gzip olarak algilanir, dosya uzantisina bakilmaz).
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	// Büyük DB dump yüklemeleri sunucunun kısa varsayılan zaman aşımını (bkz.
	// cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 10*time.Minute)
	domainID, _, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	dbName := r.URL.Query().Get("databaseName")
	if !hesaplar.GecerliDBKimlik(dbName) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}
	// 🔴 Hedefin KENDİ kimliği alınır: dump asla MySQL root'una akıtılmaz.
	// Panel root çalıştığı için `mysql <db>` root@localhost'a bağlanırdı ve
	// dump'a konan `USE mysql; GRANT ...` ifadeleri DB sunucusunun tamamını
	// ele geçirirdi (bkz. internal/sqlimport paket açıklaması).
	hedef, err := h.dbHedefi(r, domainID, dbName)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	select {
	case dbImportGate <- struct{}{}:
		defer func() { <-dbImportGate }()
	case <-r.Context().Done():
		httpx.WriteError(w, http.StatusRequestTimeout, "içe aktarma sırası beklenirken istek sona erdi")
		return
	}

	// İçe aktarmayı RAM'e alma. Önce 0600 geçici dosyada boyutu doğrula, ancak
	// tamamı geçerliyse mysql'e ver. Doğrudan mysql'e sınırlı stream etmek,
	// limit aşımında yarım dump uygulanmasına yol açardı.
	body := bufio.NewReaderSize(r.Body, 32<<10)
	magic, err := body.Peek(2)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		httpx.WriteError(w, http.StatusBadRequest, "gövde okunamadı: "+err.Error())
		return
	}

	raw, err := os.CreateTemp("", "sanalcp-db-import-*.upload")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici dosya oluşturulamadı")
		return
	}
	rawName := raw.Name()
	defer os.Remove(rawName)
	defer raw.Close()
	n, err := io.Copy(raw, io.LimitReader(body, maxDBImportBytes+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gövde okunamadı: "+err.Error())
		return
	}
	if n > maxDBImportBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "veritabanı içe aktarma sınırı 2 GiB")
		return
	}
	if _, err := raw.Seek(0, io.SeekStart); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici dosya okunamadı")
		return
	}

	var sqlFile *os.File = raw
	if isGzip(magic) {
		gzr, err := gzip.NewReader(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gzip okunamadı: "+err.Error())
			return
		}
		defer gzr.Close()

		expanded, err := os.CreateTemp("", "sanalcp-db-import-*.sql")
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "geçici SQL dosyası oluşturulamadı")
			return
		}
		expandedName := expanded.Name()
		defer os.Remove(expandedName)
		defer expanded.Close()
		n, err := io.Copy(expanded, io.LimitReader(gzr, maxDBImportBytes+1))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gzip açılamadı: "+err.Error())
			return
		}
		if n > maxDBImportBytes {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "açılmış SQL sınırı 2 GiB")
			return
		}
		if _, err := expanded.Seek(0, io.SeekStart); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "geçici SQL dosyası okunamadı")
			return
		}
		sqlFile = expanded
	}

	if err := sqlimport.Uygula(r.Context(), hedef, sqlFile); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// dbHedefi: veritabanının bu domaine ait olduğunu doğrular ve o veritabanının
// KENDİ (düşük yetkili) kimlik bilgilerini döner.
func (h *Handlers) dbHedefi(r *http.Request, domainID int64, dbName string) (sqlimport.Hedef, error) {
	var kullanici, sifreli, host string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db_user, db_pass_plain, COALESCE(db_host,'localhost')
		   FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbName).
		Scan(&kullanici, &sifreli, &host)
	if err != nil {
		return sqlimport.Hedef{}, errors.New("bu veritabanı size ait değil")
	}
	parola, err := hesaplar.DecryptDBPassword(sifreli)
	if err != nil {
		return sqlimport.Hedef{}, errors.New("veritabanı parolası çözülemedi")
	}
	return sqlimport.Hedef{DBAdi: dbName, Kullanici: kullanici, Parola: parola, Host: host}, nil
}

func isGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}
