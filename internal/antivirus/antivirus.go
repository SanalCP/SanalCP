// Package antivirus: per-domain zararlı yazılım taraması (ClamAV + hafif heuristik).
// Güvenlik: eşzamanlı tarama sayısı SEMAPHORE ile sınırlı (ClamAV DB ~1.5G RAM →
// eşzamanlı clamscan × N OOM riski); karantina = aynı-dosya-sistemi rename
// (fuser/rm YOK), yol domain home'una kilitli.
package antivirus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sys/unix"
)

const clamBin = "/usr/bin/clamscan"
const wpBin = "/usr/local/bin/wp"

type Handlers struct{ DB *sql.DB }

// sem: eşzamanlı tarama/freshclam sınırı. ClamAV imza DB'si ~1.5 GB RAM tutar,
// N tane paralel clamscan = ~1.5N GB → küçük kutularda OOM. Init() main.go'dan
// bir kez çağrılır; default 1 (env PANEL_AV_MAX_CONCURRENT ile yükseltilebilir).
// Init çağrılmadan acquire() no-op (kuyruk sınırsız) — bu kasıtlı: testler
// Init'i çağırmayı unutursa bellek riski yerine sessizlik yaşar.
var sem chan struct{}

// Init: eşzamanlı tarama sınırını ayarla. <1 değerler 1'e yükseltilir.
// main()'den bir kez çağrılmalı; testlerde de explicit çağrılır.
func Init(maxConcurrent int) {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	sem = make(chan struct{}, maxConcurrent)
}

// acquire: slot varsa alır (true). Doluysa HEMEN false döner — handler 409
// vermek için non-blocking istiyor (beklemek HTTP isteğini asılı tutar).
func acquire() bool {
	if sem == nil {
		return true // Init çağrılmamış: sınırsız (test fallback'i)
	}
	select {
	case sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// release: bir slotu iade eder. Her acquire başarılı çağrıya bir tane.
func release() {
	if sem == nil {
		return
	}
	<-sem
}

// MaxConcurrent: Init ile ayarlanan sınır (testler). 0 = Init çağrılmamış.
func MaxConcurrent() int {
	return cap(sem)
}

var errCap = errors.New("dosya-siniri")

// Düşük yanlış-pozitif, yüksek sinyalli PHP webshell/obfuscation imzaları
var heuristics = []struct {
	ad   string
	puan int
	re   *regexp.Regexp
}{
	{"PHP.Webshell.EvalBase64", 75, regexp.MustCompile(`(?i)eval\s*\(\s*(base64_decode|gzinflate|gzuncompress|str_rot13|convert_uudecode)\s*\(`)},
	{"PHP.Webshell.PregReplaceE", 55, regexp.MustCompile(`(?i)preg_replace\s*\(\s*['"][^'"]{0,200}/e['"]`)},
	{"PHP.Webshell.AssertInput", 75, regexp.MustCompile(`(?i)assert\s*\(\s*\$_(GET|POST|REQUEST|COOKIE)`)},
	{"PHP.Webshell.SystemInput", 80, regexp.MustCompile(`(?i)(shell_exec|passthru|system|popen|proc_open)\s*\(\s*\$_(GET|POST|REQUEST|COOKIE|SERVER)`)},
	{"PHP.Webshell.KnownMarker", 90, regexp.MustCompile(`(?i)(c99shell|r57shell|b374k|wso[_ ]?shell|filesman|indoxploit|angelshell|priv8|mini\s*shell)`)},
	{"PHP.Obf.CreateFunc", 65, regexp.MustCompile(`(?i)create_function\s*\([^)]*base64_decode`)},
	{"PHP.Obf.CharObfEval", 70, regexp.MustCompile(`(?is)\$\{?['"]?\w+['"]?\}?\s*\(\s*\$\{?['"]?\w+['"]?\}?\s*\)\s*;.{0,500}base64`)},
	{"PHP.Obf.LongBase64", 25, regexp.MustCompile(`(?i)['"][A-Za-z0-9+/]{500,}={0,2}['"]`)},
}

const bulguEsigi = 70

type Bulgu struct {
	ID            int64    `json:"id"`
	Dosya         string   `json:"dosya"`
	Imza          string   `json:"imza"`
	Motor         string   `json:"motor"`
	Karantina     int      `json:"karantina"`
	Puan          int      `json:"puan"`
	Risk          string   `json:"risk"`
	Gerekceler    []string `json:"gerekceler"`
	SHA256        string   `json:"sha256"`
	Istisna       int      `json:"istisna"`
	KarantinaYolu string   `json:"karantina_yolu"`
}

func (h *Handlers) domain(r *http.Request) (id int64, sk string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).Scan(&sk, &isDemo); err != nil {
		return id, "", false, false
	}
	return id, sk, isDemo == 1, true
}

func newestClamDB() string {
	var newest time.Time
	for _, f := range []string{"daily.cld", "daily.cvd", "main.cld", "main.cvd"} {
		if fi, err := os.Stat("/var/lib/clamav/" + f); err == nil {
			if fi.ModTime().After(newest) {
				newest = fi.ModTime()
			}
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.Format("2006-01-02 15:04")
}

func motorAdi() string {
	if _, err := os.Stat(clamBin); err == nil {
		return "clamav+heuristik"
	}
	return "heuristik"
}

// GET /domains/{id}/antivirus
func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	id, sk, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	_, clamErr := os.Stat(clamBin)
	resp := map[string]any{
		"clamav":      clamErr == nil,
		"imza_tarihi": newestClamDB(),
		"kullanici":   sk,
		"son_tarama":  nil,
		"bulgular":    []Bulgu{},
	}
	var sid int64
	var durum, motor, bas string
	var bitis sql.NullString
	var taranan, enfekte int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, durum, motor, taranan, enfekte, baslangic, bitis
		   FROM av_taramalar WHERE domain_id=? ORDER BY id DESC LIMIT 1`, id).
		Scan(&sid, &durum, &motor, &taranan, &enfekte, &bas, &bitis); err == nil {
		resp["son_tarama"] = map[string]any{
			"id": sid, "durum": durum, "motor": motor, "taranan": taranan,
			"enfekte": enfekte, "baslangic": bas, "bitis": bitis.String,
		}
		resp["bulgular"] = h.bulgular(r.Context(), sid)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handlers) bulgular(ctx context.Context, sid int64) []Bulgu {
	out := []Bulgu{}
	rows, err := h.DB.QueryContext(ctx, `SELECT id,dosya,imza,motor,karantina,COALESCE(puan,0),COALESCE(risk,''),COALESCE(gerekceler,'[]'),COALESCE(sha256,''),COALESCE(istisna,0),COALESCE(karantina_yolu,'') FROM av_bulgular WHERE tarama_id=? ORDER BY puan DESC,id`, sid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var b Bulgu
		var gerekceler string
		if err := rows.Scan(&b.ID, &b.Dosya, &b.Imza, &b.Motor, &b.Karantina, &b.Puan, &b.Risk, &gerekceler, &b.SHA256, &b.Istisna, &b.KarantinaYolu); err == nil {
			_ = json.Unmarshal([]byte(gerekceler), &b.Gerekceler)
			out = append(out, b)
		}
	}
	_ = rows.Err()
	return out
}

// POST /domains/{id}/antivirus/tara
func (h *Handlers) Tara(w http.ResponseWriter, r *http.Request) {
	id, sk, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	root := "/home/" + sk + "/public_html"
	if _, err := os.Stat(root); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "public_html bulunamadı")
		return
	}
	if !acquire() {
		httpx.WriteError(w, http.StatusConflict, "sunucuda eşzamanlı tarama sınırına ulaşıldı, lütfen bekleyin")
		return
	}
	res, err := h.DB.Exec(`INSERT INTO av_taramalar (domain_id, durum, motor) VALUES (?,?,?)`, id, "calisiyor", motorAdi())
	if err != nil {
		release()
		httpx.WriteError(w, http.StatusInternalServerError, "tarama kaydı oluşturulamadı")
		return
	}
	sid, _ := res.LastInsertId()
	go func() {
		defer release()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		taranan, findings := runScan(ctx, root, sk)
		aktifBulgu := 0
		for _, f := range findings {
			var istisna int
			_ = h.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM av_istisnalar WHERE domain_id=? AND dosya=? AND imza=? AND sha256=?)`, id, f.Dosya, f.Imza, f.SHA256).Scan(&istisna)
			gerekceler, _ := json.Marshal(f.Gerekceler)
			_, _ = h.DB.Exec(`INSERT INTO av_bulgular (tarama_id,domain_id,dosya,imza,motor,puan,risk,gerekceler,sha256,istisna) VALUES (?,?,?,?,?,?,?,?,?,?)`,
				sid, id, f.Dosya, f.Imza, f.Motor, f.Puan, f.Risk, gerekceler, f.SHA256, istisna)
			if istisna == 0 {
				aktifBulgu++
			}
		}
		_, _ = h.DB.Exec(`UPDATE av_taramalar SET durum='bitti', taranan=?, enfekte=?, bitis=NOW() WHERE id=?`,
			taranan, aktifBulgu, sid)
	}()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"scan_id": sid})
}

// GET /domains/{id}/antivirus/tara/{sid}
func (h *Handlers) TaraDurum(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	sid, _ := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var durum, motor, bas string
	var bitis sql.NullString
	var taranan, enfekte int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT durum, motor, taranan, enfekte, baslangic, bitis FROM av_taramalar WHERE id=? AND domain_id=?`, sid, id).
		Scan(&durum, &motor, &taranan, &enfekte, &bas, &bitis); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "tarama bulunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id": sid, "durum": durum, "motor": motor, "taranan": taranan,
		"enfekte": enfekte, "baslangic": bas, "bitis": bitis.String,
		"bulgular": h.bulgular(r.Context(), sid),
	})
}

// POST /domains/{id}/antivirus/karantina  {dosya}
func (h *Handlers) Karantina(w http.ResponseWriter, r *http.Request) {
	id, sk, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	var req struct {
		BulguID int64 `json:"bulgu_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	var dosya, beklenenSHA string
	var karantina int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT dosya,COALESCE(sha256,''),karantina FROM av_bulgular WHERE id=? AND domain_id=?`, req.BulguID, id).Scan(&dosya, &beklenenSHA, &karantina); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "bulgu bulunamadı")
		return
	}
	if karantina != 0 || beklenenSHA == "" {
		httpx.WriteError(w, http.StatusConflict, "bulgu karantinaya uygun değil")
		return
	}
	home, root := "/home/"+sk, "/home/"+sk+"/public_html"
	mevcutSHA, err := guvenliDosyaOzeti(root, dosya)
	if err != nil || mevcutSHA != beklenenSHA {
		httpx.WriteError(w, http.StatusConflict, "dosya taramadan sonra değişmiş; yeniden tarayın")
		return
	}
	if err := jailpath.DizinOlustur(home, ".karantina", sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "karantina dizini oluşturulamadı")
		return
	}
	if qd, err := jailpath.AcDizin(home, ".karantina"); err == nil {
		_ = unix.Fchmod(int(qd.Fd()), 0o700)
		_ = qd.Close()
	}
	kaynakRel, err := filepath.Rel(home, dosya)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz bulgu yolu")
		return
	}
	hedefRel := filepath.Join(".karantina", fmt.Sprintf("%d_%s", req.BulguID, filepath.Base(dosya)))
	if err := jailpath.Tasi(home, kaynakRel, hedefRel); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "taşınamadı: "+err.Error())
		return
	}
	if qf, err := jailpath.Ac(home, filepath.ToSlash(hedefRel), unix.O_RDONLY|unix.O_NONBLOCK, 0); err == nil {
		_ = unix.Fchmod(int(qf.Fd()), 0)
		_ = qf.Close()
	}
	hedef := filepath.Join(home, hedefRel)
	if _, err := h.DB.Exec(`UPDATE av_bulgular SET karantina=1,karantina_yolu=?,karantina_zamani=NOW() WHERE id=? AND domain_id=?`, hedef, req.BulguID, id); err != nil {
		_ = jailpath.Tasi(home, hedefRel, kaynakRel)
		httpx.WriteError(w, 500, "karantina kaydı güncellenemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /domains/{id}/antivirus/karantina-geri-al {bulgu_id}
func (h *Handlers) KarantinaGeriAl(w http.ResponseWriter, r *http.Request) {
	id, sk, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		BulguID int64 `json:"bulgu_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	var dosya, qyol, beklenenSHA string
	var karantina int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT dosya,COALESCE(karantina_yolu,''),COALESCE(sha256,''),karantina FROM av_bulgular WHERE id=? AND domain_id=?`, req.BulguID, id).Scan(&dosya, &qyol, &beklenenSHA, &karantina); err != nil {
		httpx.WriteError(w, 404, "bulgu bulunamadı")
		return
	}
	if karantina == 0 || qyol == "" || beklenenSHA == "" {
		httpx.WriteError(w, 409, "bulgu karantinada değil")
		return
	}
	home, root := "/home/"+sk, "/home/"+sk+"/public_html"
	qSHA, err := guvenliDosyaOzeti(root, qyol)
	if err != nil || qSHA != beklenenSHA {
		httpx.WriteError(w, 409, "karantina dosyası değişmiş veya okunamıyor")
		return
	}
	kaynakRel, e1 := filepath.Rel(home, qyol)
	hedefRel, e2 := filepath.Rel(home, dosya)
	if e1 != nil || e2 != nil {
		httpx.WriteError(w, 400, "geçersiz karantina yolu")
		return
	}
	if err := jailpath.Tasi(home, kaynakRel, hedefRel); err != nil {
		httpx.WriteError(w, 409, "özgün hedef dolu veya güvenli değil: "+err.Error())
		return
	}
	if f, err := jailpath.Ac(home, filepath.ToSlash(hedefRel), unix.O_RDONLY|unix.O_NONBLOCK, 0); err == nil {
		_ = unix.Fchmod(int(f.Fd()), 0o640)
		_ = f.Close()
	}
	if _, err := h.DB.Exec(`UPDATE av_bulgular SET karantina=0,karantina_yolu='',karantina_zamani=NULL WHERE id=? AND domain_id=?`, req.BulguID, id); err != nil {
		_ = jailpath.Tasi(home, hedefRel, kaynakRel)
		httpx.WriteError(w, 500, "geri alma kaydı güncellenemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

// POST /domains/{id}/antivirus/istisna {bulgu_id}. İstisna yalnız tam yol,
// imza ve SHA-256 üçlüsüne bağlıdır; dosya değişirse sonraki taramada yeniden çıkar.
func (h *Handlers) IstisnaEkle(w http.ResponseWriter, r *http.Request) {
	id, sk, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		BulguID int64 `json:"bulgu_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	var dosya, imza, beklenenSHA string
	var karantina int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT dosya,imza,COALESCE(sha256,''),karantina FROM av_bulgular WHERE id=? AND domain_id=?`, req.BulguID, id).Scan(&dosya, &imza, &beklenenSHA, &karantina); err != nil {
		httpx.WriteError(w, 404, "bulgu bulunamadı")
		return
	}
	if karantina != 0 || beklenenSHA == "" {
		httpx.WriteError(w, 409, "bulgu istisnaya uygun değil")
		return
	}
	mevcut, err := guvenliDosyaOzeti("/home/"+sk+"/public_html", dosya)
	if err != nil || mevcut != beklenenSHA {
		httpx.WriteError(w, 409, "dosya taramadan sonra değişmiş; yeniden tarayın")
		return
	}
	if _, err := h.DB.Exec(`INSERT IGNORE INTO av_istisnalar(domain_id,dosya,imza,sha256) VALUES(?,?,?,?)`, id, dosya, imza, beklenenSHA); err != nil {
		httpx.WriteError(w, 500, "istisna kaydedilemedi")
		return
	}
	_, _ = h.DB.Exec(`UPDATE av_bulgular SET istisna=1 WHERE domain_id=? AND dosya=? AND imza=? AND sha256=?`, id, dosya, imza, beklenenSHA)
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (h *Handlers) IstisnaSil(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	bid, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	var dosya, imza, sha string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT dosya,imza,COALESCE(sha256,'') FROM av_bulgular WHERE id=? AND domain_id=?`, bid, id).Scan(&dosya, &imza, &sha); err != nil {
		httpx.WriteError(w, 404, "bulgu bulunamadı")
		return
	}
	_, _ = h.DB.Exec(`DELETE FROM av_istisnalar WHERE domain_id=? AND dosya=? AND imza=? AND sha256=?`, id, dosya, imza, sha)
	_, _ = h.DB.Exec(`UPDATE av_bulgular SET istisna=0 WHERE domain_id=? AND dosya=? AND imza=? AND sha256=?`, id, dosya, imza, sha)
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

// POST /domains/{id}/antivirus/imza-guncelle  → freshclam
func (h *Handlers) ImzaGuncelle(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat("/usr/bin/freshclam"); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "freshclam kurulu değil")
		return
	}
	if !acquire() {
		httpx.WriteError(w, http.StatusConflict, "başka bir işlem sürüyor, bekleyin")
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/freshclam").CombinedOutput()
	cikti := string(out)
	if len(cikti) > 4000 {
		cikti = cikti[len(cikti)-4000:]
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": err == nil, "imza_tarihi": newestClamDB(), "cikti": cikti,
	})
}

// runScan: ClamAV (varsa) + heuristik. taranan dosya sayısı + bulgular döner.
func runScan(ctx context.Context, root string, kullanici ...string) (int, []Bulgu) {
	var findings []Bulgu
	seen := map[string]bool{}

	// 1) ClamAV
	if _, err := os.Stat(clamBin); err == nil {
		cmd := exec.CommandContext(ctx, clamBin, "-r", "-i", "--no-summary", "--stdout",
			"--max-filesize=25M", "--max-scansize=500M", root)
		out, _ := cmd.CombinedOutput()
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, " FOUND") {
				if i := strings.LastIndex(line, ": "); i > 0 {
					dosya := line[:i]
					imza := strings.TrimSuffix(line[i+2:], " FOUND")
					if !seen["c|"+dosya] {
						seen["c|"+dosya] = true
						findings = append(findings, Bulgu{Dosya: dosya, Imza: imza, Motor: "clamav", Puan: 100, Risk: "kritik", Gerekceler: []string{"ClamAV imza eşleşmesi"}})
					}
				}
			}
		}
	}

	// 2) Heuristik PHP webshell taraması
	taranan := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".karantina":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !phpish(strings.ToLower(filepath.Ext(p))) {
			return nil
		}
		fi, e := d.Info()
		if e != nil || fi.Size() > 3*1024*1024 {
			return nil
		}
		taranan++
		if taranan > 50000 {
			return errCap
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		puan := 0
		gerekceler := []string{}
		imzalar := []string{}
		for _, hs := range heuristics {
			if hs.re.Match(b) {
				puan += hs.puan
				imzalar = append(imzalar, hs.ad)
				gerekceler = append(gerekceler, hs.ad+" ("+strconv.Itoa(hs.puan)+")")
			}
		}
		if ek, neden := konumPuani(root, p); ek > 0 && puan > 0 {
			puan += ek
			gerekceler = append(gerekceler, neden+" ("+strconv.Itoa(ek)+")")
		}
		if puan > 100 {
			puan = 100
		}
		if puan >= bulguEsigi && !seen["h|"+p] {
			seen["h|"+p] = true
			findings = append(findings, Bulgu{Dosya: p, Imza: strings.Join(imzalar, ", "), Motor: "puanli-heuristik", Puan: puan, Risk: riskSeviyesi(puan), Gerekceler: gerekceler})
		}
		return nil
	})
	if len(kullanici) > 0 && kullanici[0] != "" {
		for _, f := range wordpressButunluk(ctx, root, kullanici[0]) {
			if !seen["w|"+f.Dosya] {
				seen["w|"+f.Dosya] = true
				findings = append(findings, f)
			}
		}
	}
	for i := range findings {
		findings[i].SHA256, _ = guvenliDosyaOzeti(root, findings[i].Dosya)
	}
	return taranan, findings
}

func guvenliDosyaOzeti(root, dosya string) (string, error) {
	home := filepath.Dir(root)
	rel, err := filepath.Rel(home, dosya)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("dosya tenant kökü dışında")
	}
	f, err := jailpath.Ac(home, filepath.ToSlash(rel), unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 25*1024*1024 {
		return "", errors.New("özetlenemeyen dosya")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

var wpChecksumSatiri = regexp.MustCompile(`(?m)^Warning:\s+(File (?:should not exist|doesn't verify against checksum):)\s+(.+?)\s*$`)

// wordpressButunluk yalnız kurulu WordPress köklerini denetler. WP-CLI/ağ
// hatası bulgu değildir; doğrulanamayan durum ile doğrulanmış ihlal ayrılır.
func wordpressButunluk(ctx context.Context, root, sk string) []Bulgu {
	if _, err := os.Stat(wpBin); err != nil {
		return nil
	}
	adaylar := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				adaylar = append(adaylar, filepath.Join(root, e.Name()))
			}
		}
	}
	var out []Bulgu
	for _, dir := range adaylar {
		if _, err := os.Stat(filepath.Join(dir, "wp-config.php")); err != nil {
			continue
		}
		full := []string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk,
			"/usr/bin/php", "-d", "memory_limit=256M", wpBin, "core", "verify-checksums",
			"--path=" + dir, "--skip-plugins", "--skip-themes", "--no-color"}
		b, _ := exec.CommandContext(ctx, "runuser", full...).CombinedOutput()
		out = append(out, wordpressChecksumBulgular(dir, string(b))...)
	}
	return out
}

func wordpressChecksumBulgular(root, cikti string) []Bulgu {
	out := []Bulgu{}
	for _, m := range wpChecksumSatiri.FindAllStringSubmatch(cikti, -1) {
		rel := filepath.Clean(strings.TrimSpace(m[2]))
		if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		beklenmeyen := strings.Contains(m[1], "should not exist")
		coreDizini := strings.HasPrefix(filepath.ToSlash(rel), "wp-admin/") || strings.HasPrefix(filepath.ToSlash(rel), "wp-includes/")
		if beklenmeyen && !coreDizini {
			continue
		}
		puan, risk := 100, "kritik"
		imza, gerekce := "WordPress.Core.ChecksumMismatch", "Resmî WordPress çekirdek checksum'u eşleşmedi"
		if beklenmeyen {
			puan, risk = 85, "yuksek"
			imza, gerekce = "WordPress.Core.UnexpectedFile", "WordPress çekirdek dizininde dağıtıma ait olmayan dosya"
		}
		out = append(out, Bulgu{Dosya: filepath.Join(root, rel), Imza: imza, Motor: "wordpress-checksum", Puan: puan, Risk: risk, Gerekceler: []string{gerekce}})
	}
	return out
}

func riskSeviyesi(puan int) string {
	if puan >= 90 {
		return "kritik"
	}
	if puan >= 70 {
		return "yuksek"
	}
	if puan >= 40 {
		return "orta"
	}
	return "dusuk"
}

// Konum tek başına bulgu üretmez; yalnız içerik sinyalini güçlendirir. Böylece
// uploads/cache altındaki meşru PHP dosyaları otomatik karantina adayına dönüşmez.
func konumPuani(root, p string) (int, string) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return 0, ""
	}
	d := "/" + strings.ToLower(filepath.ToSlash(filepath.Dir(rel))) + "/"
	for _, parca := range []string{"/uploads/", "/upload/", "/cache/", "/tmp/", "/images/", "/assets/"} {
		if strings.Contains(d, parca) {
			return 20, "PHP için olağandışı yazılabilir konum"
		}
	}
	if strings.Count(filepath.Base(p), ".") >= 2 {
		return 10, "Çok uzantılı dosya adı"
	}
	return 0, ""
}

func phpish(ext string) bool {
	switch ext {
	case ".php", ".phtml", ".php3", ".php4", ".php5", ".php7", ".php8", ".phar", ".inc", ".pht":
		return true
	}
	return false
}
