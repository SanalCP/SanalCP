package transfers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"sanalpanel/internal/domains"
	"sanalpanel/internal/hesaplar"
	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const MaxUploadBytes = int64(20 << 30)

type Handlers struct {
	DB      *sql.DB
	Domains *domains.Handlers
}

// Analyze accepts a cPanel full backup and returns an inventory. It never
// extracts or persists archive contents.
func (h *Handlers) Analyze(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "arşiv yüklenemedi veya boyut sınırı aşıldı")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, hdr, err := r.FormFile("archive")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "archive alanında cPanel .tar.gz yedeği gerekli")
		return
	}
	defer f.Close()
	low := strings.ToLower(hdr.Filename)
	if !strings.HasSuffix(low, ".tar.gz") && !strings.HasSuffix(low, ".tgz") {
		httpx.WriteError(w, http.StatusBadRequest, "ilk sürüm yalnız cPanel .tar.gz/.tgz tam yedeklerini destekliyor")
		return
	}
	inv, err := AnalyzeCPanel(f)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrArchiveTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		httpx.WriteError(w, status, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

type importResponse struct {
	OK          bool      `json:"ok"`
	DomainID    int64     `json:"domain_id"`
	Domain      string    `json:"domain"`
	SystemUser  string    `json:"system_user"`
	WebFiles    int       `json:"web_files"`
	Databases   []DBMap   `json:"databases"`
	Credentials any       `json:"credentials"`
	Skipped     []string  `json:"skipped"`
	Source      Inventory `json:"source"`
}

type DBMap struct {
	Source string `json:"source"`
	Target string `json:"target"`
	User   string `json:"user"`
}

// Import creates a new SanalPanel domain and restores the web root plus a
// cPanel databases. Additional databases share the domain's default DB user,
// matching SanalPanel's supported one-user-to-many-databases model.
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	if h.Domains == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sağlayıcısı hazır değil")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "arşiv yüklenemedi veya boyut sınırı aşıldı")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, _, err := r.FormFile("archive")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "archive alanı zorunlu")
		return
	}
	defer f.Close()

	tmp, err := os.CreateTemp("", "sanalpanel-cpanel-*.tar.gz")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici arşiv oluşturulamadı")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, f); err != nil || tmp.Close() != nil {
		httpx.WriteError(w, http.StatusBadRequest, "arşiv kaydedilemedi")
		return
	}
	src, err := os.Open(tmpPath)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "arşiv açılamadı")
		return
	}
	inv, err := AnalyzeCPanel(src)
	src.Close()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	if domain == "" {
		domain = inv.PrimaryDomain
	}
	if domain == "" {
		httpx.WriteError(w, http.StatusBadRequest, "ana domain belirlenemedi")
		return
	}
	customerID, err := requiredInt64(r.FormValue("customer_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "customer_id zorunlu")
		return
	}
	phpVersion := strings.TrimSpace(r.FormValue("php_version"))
	if phpVersion == "" {
		phpVersion = "8.3"
	}
	var planID *int64
	if s := strings.TrimSpace(r.FormValue("plan_id")); s != "" {
		v, e := requiredInt64(s)
		if e != nil {
			httpx.WriteError(w, http.StatusBadRequest, "plan_id geçersiz")
			return
		}
		planID = &v
	}

	createBody, _ := json.Marshal(map[string]any{
		"alan_adi": domain, "php_surum": phpVersion,
		"customer_id": customerID, "plan_id": planID,
	})
	cr := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(createBody)).
		WithContext(r.Context())
	cr.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	h.Domains.Create(cw, cr)
	if cw.Code != http.StatusCreated {
		copyRecorded(w, cw)
		return
	}
	var created struct {
		ID              int64  `json:"id"`
		AlanAdi         string `json:"alan_adi"`
		SistemKullanici string `json:"sistem_kullanici"`
		DBAdi           string `json:"db_adi"`
		DBUser          string `json:"db_user"`
		Parolalar       any    `json:"olusturulan_parolalar"`
	}
	if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		httpx.WriteError(w, http.StatusInternalServerError, "oluşturulan domain yanıtı okunamadı")
		return
	}
	committed := false
	defer func() {
		if !committed {
			h.rollbackDomain(r, created.ID)
		}
	}()

	if err := restoreWeb(tmpPath, inv.ArchiveRoot, created.SistemKullanici); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "web dosyaları aktarılamadı: "+err.Error())
		return
	}
	dbMaps := databaseMappings(inv.Databases, created.SistemKullanici, created.DBAdi, created.DBUser)
	for i, m := range dbMaps {
		if i > 0 {
			if err := hesaplar.MySQLCreateDBForUser(h.DB, created.ID, m.Target, created.DBUser); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "ek veritabanı oluşturulamadı: "+err.Error())
				return
			}
		}
		if err := restoreDatabase(tmpPath, inv.ArchiveRoot, m.Source, m.Target); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "veritabanı aktarılamadı: "+err.Error())
			return
		}
	}
	committed = true
	httpx.WriteJSON(w, http.StatusCreated, importResponse{
		OK: true, DomainID: created.ID, Domain: created.AlanAdi,
		SystemUser: created.SistemKullanici, WebFiles: inv.WebFiles,
		Databases: dbMaps, Credentials: created.Parolalar, Source: inv,
		Skipped: []string{"E-posta kutuları, cron ve kaynak SSL sertifikaları bu ilk sürümde yalnız envanterlendi."},
	})
}

func databaseMappings(sources []string, sk, defaultDB, dbUser string) []DBMap {
	out := make([]DBMap, 0, len(sources))
	used := map[string]bool{defaultDB: true}
	for i, source := range sources {
		target := defaultDB
		if i > 0 {
			suffix := dbSuffix(source)
			maxSuffix := 64 - len(sk) - 1
			if maxSuffix < 1 {
				maxSuffix = 1
			}
			if len(suffix) > maxSuffix {
				suffix = suffix[:maxSuffix]
			}
			target = sk + "_" + suffix
			base := target
			for n := 2; used[target]; n++ {
				tail := "_" + strconv.Itoa(n)
				limit := 64 - len(tail)
				if len(base) > limit {
					base = base[:limit]
				}
				target = base + tail
			}
		}
		used[target] = true
		out = append(out, DBMap{Source: source, Target: target, User: dbUser})
	}
	return out
}

func dbSuffix(source string) string {
	s := strings.ToLower(source)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	s = strings.Trim(b.String(), "_")
	if s == "" {
		return "db"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.TrimRight(s, "_")
}

func requiredInt64(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("geçersiz sayı")
	}
	return v, nil
}

func copyRecorded(w http.ResponseWriter, rr *httptest.ResponseRecorder) {
	for k, values := range rr.Header() {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rr.Code)
	_, _ = w.Write(rr.Body.Bytes())
}

func (h *Handlers) rollbackDomain(r *http.Request, id int64) {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(id, 10))
	ctx := contextWithRoute(r, rc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(id, 10), nil).
		WithContext(ctx)
	h.Domains.Delete(httptest.NewRecorder(), req)
}

func contextWithRoute(r *http.Request, rc *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rc)
}

func restoreWeb(archivePath, root, sk string) error {
	if !strings.HasPrefix(sk, "c_") || root == "" {
		return errors.New("güvensiz hedef")
	}
	home := "/home/" + sk
	target := home + "/public_html"
	if out, err := exec.Command("runuser", "-u", sk, "--", "find", target, "-mindepth", "1", "-delete").CombinedOutput(); err != nil {
		return fmt.Errorf("hedef temizleme: %s", strings.TrimSpace(string(out)))
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	member := root + "/homedir/public_html"
	cmd := exec.Command("runuser", "-u", sk, "--", "tar", "-xz", "-f", "-", "-C", target,
		"--strip-components=3", member)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	return nil
}

func restoreDatabase(archivePath, root, sourceDB, targetDB string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	want := root + "/mysql/" + sourceDB + ".sql"
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return errors.New("SQL dump arşivde bulunamadı")
		}
		if err != nil {
			return err
		}
		if path.Clean(h.Name) != want {
			continue
		}
		cmd := exec.Command("mysql", targetDB)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		bw := bufio.NewWriter(stdin)
		br := bufio.NewReader(tr)
		for {
			line, readErr := br.ReadString('\n')
			upper := strings.ToUpper(strings.TrimSpace(line))
			if !strings.HasPrefix(upper, "CREATE DATABASE ") && !strings.HasPrefix(upper, "USE ") {
				if _, err := bw.WriteString(line); err != nil {
					_ = stdin.Close()
					_ = cmd.Wait()
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return readErr
			}
		}
		_ = bw.Flush()
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("mysql: %s", strings.TrimSpace(stderr.String()))
		}
		return nil
	}
}
