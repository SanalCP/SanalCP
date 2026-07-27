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

	"sanalpanel/internal/cron"
	"sanalpanel/internal/domains"
	"sanalpanel/internal/hesaplar"
	"sanalpanel/internal/httpx"
	"sanalpanel/internal/mail"

	"github.com/go-chi/chi/v5"
)

const MaxUploadBytes = int64(20 << 30)

type Handlers struct {
	DB      *sql.DB
	Domains *domains.Handlers
	Mail    *mail.Handlers
	Cron    *cron.Handlers
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
	OK          bool             `json:"ok"`
	DomainID    int64            `json:"domain_id"`
	Domain      string           `json:"domain"`
	SystemUser  string           `json:"system_user"`
	WebFiles    int              `json:"web_files"`
	Databases   []DBMap          `json:"databases"`
	Credentials any              `json:"credentials"`
	Mailboxes   []MailCredential `json:"mailboxes"`
	Aliases     int              `json:"aliases"`
	CronJobs    int              `json:"cron_jobs"`
	Skipped     []string         `json:"skipped"`
	Source      Inventory        `json:"source"`
}

type MailCredential struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
	mailCreds, aliasCount, err := h.importMail(r, tmpPath, inv, created.ID, created.AlanAdi, created.SistemKullanici)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "e-posta aktarılamadı: "+err.Error())
		return
	}
	cronCount, err := h.importCron(r, inv, created.ID, created.SistemKullanici)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "cron görevleri aktarılamadı: "+err.Error())
		return
	}
	committed = true
	httpx.WriteJSON(w, http.StatusCreated, importResponse{
		OK: true, DomainID: created.ID, Domain: created.AlanAdi,
		SystemUser: created.SistemKullanici, WebFiles: inv.WebFiles,
		Databases: dbMaps, Credentials: created.Parolalar, Mailboxes: mailCreds,
		Aliases: aliasCount, CronJobs: cronCount, Source: inv,
		Skipped: []string{"Kaynak SSL sertifikaları bu sürümde yalnız envanterlendi."},
	})
}

func (h *Handlers) importCron(r *http.Request, inv Inventory, domainID int64, targetUser string) (int, error) {
	if len(inv.CronJobs) == 0 {
		return 0, nil
	}
	if h.Cron == nil {
		return 0, errors.New("cron sağlayıcısı hazır değil")
	}
	created := 0
	for _, job := range inv.CronJobs {
		command := job.Command
		if inv.Username != "" {
			command = strings.ReplaceAll(command, "/home/"+inv.Username+"/", "/home/"+targetUser+"/")
		}
		body, _ := json.Marshal(map[string]string{
			"dakika": job.Minute, "saat": job.Hour, "gun": job.Day,
			"ay": job.Month, "hafta": job.Weekday,
			"komut": command, "yorum": job.Comment,
		})
		req := domainRequest(r, http.MethodPost, "/cron", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Cron.Create(rr, req)
		if rr.Code != http.StatusCreated {
			return 0, fmt.Errorf("%d. görev: %s", created+1, strings.TrimSpace(rr.Body.String()))
		}
		created++
	}
	return created, nil
}

func (h *Handlers) importMail(r *http.Request, archivePath string, inv Inventory, domainID int64, targetDomain, sk string) ([]MailCredential, int, error) {
	if len(inv.Mailboxes) == 0 && inv.AliasCount == 0 && inv.MailFiles == 0 {
		return []MailCredential{}, 0, nil
	}
	if h.Mail == nil {
		return nil, 0, errors.New("mail sağlayıcısı hazır değil")
	}
	if err := mail.MailUygula(r.Context(), h.DB, domainID); err != nil {
		return nil, 0, err
	}
	creds := make([]MailCredential, 0, len(inv.Mailboxes))
	for _, local := range inv.Mailboxes {
		body, _ := json.Marshal(map[string]string{"local_part": local})
		req := domainRequest(r, http.MethodPost, "/mail", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.Ekle(rr, req)
		if rr.Code != http.StatusCreated {
			return nil, 0, fmt.Errorf("%s kutusu: %s", local, strings.TrimSpace(rr.Body.String()))
		}
		var result struct {
			Email  string `json:"email"`
			Parola string `json:"parola"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return nil, 0, err
		}
		creds = append(creds, MailCredential{Email: result.Email, Password: result.Parola})
		if inv.PrimaryDomain != "" {
			if err := restoreMailbox(archivePath, inv.ArchiveRoot, inv.PrimaryDomain, local, sk); err != nil {
				return nil, 0, fmt.Errorf("%s mesajları: %w", local, err)
			}
		}
	}

	aliases, err := readAliases(archivePath, inv.ArchiveRoot, inv.PrimaryDomain, targetDomain)
	if err != nil {
		return nil, 0, err
	}
	created := 0
	for _, a := range aliases {
		body, _ := json.Marshal(map[string]string{"local_part": a.Local, "destination": a.Destination})
		req := domainRequest(r, http.MethodPost, "/mail/aliases", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.AliasEkle(rr, req)
		if rr.Code == http.StatusCreated {
			created++
			continue
		}
		return nil, 0, fmt.Errorf("%s aliası: %s", a.Local, strings.TrimSpace(rr.Body.String()))
	}
	return creds, created, nil
}

func domainRequest(parent *http.Request, method, url string, domainID int64, body io.Reader) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(domainID, 10))
	ctx := context.WithValue(parent.Context(), chi.RouteCtxKey, rc)
	req := httptest.NewRequest(method, url, body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func restoreMailbox(archivePath, root, sourceDomain, local, sk string) error {
	target := "/home/" + sk + "/mail/" + local
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	member := root + "/homedir/mail/" + sourceDomain + "/" + local
	cmd := exec.Command("runuser", "-u", sk, "--", "tar", "-xz", "-f", "-", "-C", target,
		"--strip-components=5", member)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		// Metadata'da bulunan boş bir kutunun arşivde Maildir'i olmayabilir.
		if strings.Contains(string(out), "Not found in archive") || strings.Contains(string(out), "Not found") {
			return nil
		}
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	return nil
}

type aliasImport struct {
	Local       string
	Destination string
}

func readAliases(archivePath, root, sourceDomain, targetDomain string) ([]aliasImport, error) {
	if sourceDomain == "" {
		return []aliasImport{}, nil
	}
	body, err := readSmallTarMember(archivePath, root+"/va/"+sourceDomain)
	if errors.Is(err, errMemberNotFound) {
		return []aliasImport{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []aliasImport{}
	for _, line := range strings.Split(string(body), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) != 2 {
			continue
		}
		source := strings.TrimSpace(p[0])
		destRaw := strings.TrimSpace(p[1])
		if source == "" || destRaw == "" || strings.HasPrefix(destRaw, ":") || strings.HasPrefix(destRaw, "|") {
			continue
		}
		local := strings.TrimSuffix(strings.ToLower(source), "@"+strings.ToLower(sourceDomain))
		if local == "*" {
			local = ""
		}
		if local != "" && !localPartRE.MatchString(local) {
			continue
		}
		var dests []string
		for _, d := range strings.Split(destRaw, ",") {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if !strings.Contains(d, "@") && localPartRE.MatchString(d) {
				d += "@" + targetDomain
			}
			d = strings.ReplaceAll(d, "@"+strings.ToLower(sourceDomain), "@"+targetDomain)
			if strings.Contains(d, "@") {
				dests = append(dests, d)
			}
		}
		if len(dests) > 0 {
			out = append(out, aliasImport{Local: local, Destination: strings.Join(dests, ",")})
		}
	}
	return out, nil
}

var errMemberNotFound = errors.New("arşiv üyesi bulunamadı")

func readSmallTarMember(archivePath, want string) ([]byte, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errMemberNotFound
		}
		if err != nil {
			return nil, err
		}
		if path.Clean(h.Name) == path.Clean(want) {
			if h.Size > maxMetadataBytes {
				return nil, ErrArchiveTooLarge
			}
			return io.ReadAll(io.LimitReader(tr, maxMetadataBytes))
		}
	}
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
