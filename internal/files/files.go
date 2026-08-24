// Package files: domain ev dizinine chroot edilmis dosya yoneticisi API'si
package files

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sys/unix"
)

// MaxUploadBytes: tek yükleme için üst sınır. Onceden 10 GiB idi ve ParseMultipartForm'a
// maxMemory olarak veriliyordu → tek istekle 10 GiB RAM ayrilabiliyordu (DoS).
const MaxUploadBytes = 10 * 1024 * 1024 * 1024 // 10 GiB

// maxMultipartMemory: multipart ayrıştırmada RAM'de tutulacak azami tampon. Fazlası
// otomatik olarak geçici diske taşar → büyük yüklemelerde RAM patlamaz.
const maxMultipartMemory = 32 << 20 // 32 MiB

type Handlers struct {
	DB *sql.DB
}

// home: domain id -> /home/c_<user>
func (h *Handlers) home(r *http.Request) (string, string, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", os.ErrNotExist
	}
	if err != nil {
		return "", "", err
	}
	if isDemo == 1 {
		return "", "", errDemo
	}
	if !adlar.SKGecerli(sk) {
		return "", "", errBadUser
	}
	return "/home/" + sk, sk, nil
}

var (
	errDemo    = errors.New("demo aboneliğin dosyaları yönetilemez")
	errBadUser = errors.New("güvenlik: geçersiz sistem kullanıcısı")
	errEscape  = errors.New("güvenlik: ev dizini dışına çıkış engellendi")
)

type Entry struct {
	Adi      string `json:"adi"`
	Yol      string `json:"yol"` // home'a goreceli (panel UI icin)
	Tip      string `json:"tip"` // "klasor" | "dosya" | "sembolik"
	BoyutB   int64  `json:"boyut_b"`
	Mod      string `json:"mod"`      // "0644"
	Yetkiler string `json:"yetkiler"` // rwx dizesi: "rwxr-xr-x"
	Sahip    string `json:"sahip"`    // owner kullanıcı adı (uid çözülemezse sayı)
	Grup     string `json:"grup"`     // grup adı (gid çözülemezse sayı)
	Degisme  string `json:"degisme"`  // RFC3339
}

// uid/gid → isim çözüm cache'i (aynı user/grup tekrar tekrar lookup edilmesin).
var (
	uidAdiMu  sync.RWMutex
	uidAdiMap = map[uint32]string{}
	gidAdiMu  sync.RWMutex
	gidAdiMap = map[uint32]string{}
)

func uidAdi(uid uint32) string {
	uidAdiMu.RLock()
	if v, ok := uidAdiMap[uid]; ok {
		uidAdiMu.RUnlock()
		return v
	}
	uidAdiMu.RUnlock()
	ad := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(ad); err == nil && u.Username != "" {
		ad = u.Username
	}
	uidAdiMu.Lock()
	uidAdiMap[uid] = ad
	uidAdiMu.Unlock()
	return ad
}

func gidAdi(gid uint32) string {
	gidAdiMu.RLock()
	if v, ok := gidAdiMap[gid]; ok {
		gidAdiMu.RUnlock()
		return v
	}
	gidAdiMu.RUnlock()
	ad := strconv.FormatUint(uint64(gid), 10)
	if g, err := user.LookupGroupId(ad); err == nil && g.Name != "" {
		ad = g.Name
	}
	gidAdiMu.Lock()
	gidAdiMap[gid] = ad
	gidAdiMu.Unlock()
	return ad
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	rel := r.URL.Query().Get("yol")
	if rel == "" {
		rel = "/"
	}
	// TOCTOU symlink-güvenli listeleme: dizin openat2(RESOLVE_BENEATH|NO_SYMLINKS)
	// ile pinlenir, girdiler AT_SYMLINK_NOFOLLOW ile fstatat edilir (bkz. safeio.go).
	// Eski os.ReadDir(jailJoinStrict(...)) resolved-string üzerinde çalışıyordu →
	// ara-bileşen symlink takasıyla jail DIŞI bir dizin listelenebilirdi.
	dir, err := readDirBeneath(home, rel)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "okuma: "+err.Error())
		return
	}
	out := make([]Entry, 0, len(dir))
	for _, e := range dir {
		tip := "dosya"
		if e.Mode.IsDir() {
			tip = "klasor"
		} else if e.Mode&os.ModeSymlink != 0 {
			tip = "sembolik"
		}
		out = append(out, Entry{
			Adi:      e.Ad,
			Yol:      filepath.ToSlash(filepath.Join(rel, e.Ad)),
			Tip:      tip,
			BoyutB:   e.Boyut,
			Mod:      "0" + strconv.FormatInt(int64(e.Mode.Perm()), 8),
			Yetkiler: yetkiRWX(e.Mode),
			Sahip:    uidAdi(e.UID),
			Grup:     gidAdi(e.GID),
			Degisme:  e.MTime.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	// klasörler önce, sonra alfabetik
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Tip == "klasor") != (out[j].Tip == "klasor") {
			return out[i].Tip == "klasor"
		}
		return strings.ToLower(out[i].Adi) < strings.ToLower(out[j].Adi)
	})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"yol":    filepath.ToSlash(rel),
		"icerik": out,
		"toplam": len(out),
	})
}

// Dosya icerigini ham olarak donder (download)
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	// DEMO: ham dosya akışı JSON alanı gibi kısmi maskelenemez — tamamen kapatılır.
	if middleware.DemoPaneliMi(r) {
		httpx.WriteError(w, http.StatusForbidden, "demo modunda dosya indirilemez")
		return
	}
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	// Büyük dosya indirmeleri sunucunun kısa varsayılan yazma zaman aşımını
	// (bkz. cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	rel := r.URL.Query().Get("yol")
	// TOCTOU symlink-güvenli: dosyayı openat2 ile AÇ, sonra AÇIK fd üzerinden
	// stat'la ve akıt. Eski os.Stat+os.Open(jailJoinStrict(...)) resolved-string
	// üzerinde çalışıyordu → yarışla /etc/shadow indirilebilirdi (bkz. safeio.go).
	f, err := openReadBeneath(home, rel)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "bulunamadı")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okunamadı")
		return
	}
	if info.IsDir() {
		httpx.WriteError(w, http.StatusBadRequest, "klasör indirilemez")
		return
	}
	// İndirilen ad kullanıcı-verdiği yoldan gelir; başlık enjeksiyonunu (CRLF) ve
	// tırnak kaçışını engellemek için ad temizlenir.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+dosyaAdiTemizle(filepath.Base(relClean(rel)))+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = io.Copy(w, f)
}

// dosyaAdiTemizle: Content-Disposition'daki tırnaklı ad alanını bozabilecek
// karakterleri (çift tırnak, ters bölü, kontrol karakterleri) atar. Go'nun
// başlık yazıcısı CR/LF'i zaten temizler, yani bu başlık enjeksiyonuna karşı
// ikinci katman; asıl işi tenant'ın seçtiği adın alanı kapatmasını önlemek.
func dosyaAdiTemizle(ad string) string {
	temiz := strings.Map(func(c rune) rune {
		if c == '\r' || c == '\n' || c == '"' || c == '\\' || c < 0x20 {
			return -1
		}
		return c
	}, ad)
	if temiz == "" {
		return "dosya"
	}
	return temiz
}

// Metin dosyasini okuma (editor icin)
func (h *Handlers) Read(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	rel := r.URL.Query().Get("yol")
	// TOCTOU symlink-güvenli okuma (bkz. safeio.go): boyut kontrolü de AÇIK fd
	// üzerinden yapılır, yani "küçük dosyayı stat'la, büyük/başka dosyayı oku"
	// yarışı mümkün değildir.
	const editorSinir = 2 * 1024 * 1024
	data, boyut, err := readFileBeneath(home, rel, editorSinir)
	if errors.Is(err, errTooBig) {
		httpx.WriteError(w, http.StatusBadRequest, "dosya 2 MB'tan büyük; düzenleme için uygun değil")
		return
	}
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"yol":    rel,
		"icerik": icerikGoster(middleware.DemoPaneliMi(r), data),
		"boyut":  boyut,
	})
}

// icerikGoster: demo panelde dosya içeriği (kaynak kod, .env, wp-config.php
// vb. sır taşıyabilir) yerine sabit bir maske döner.
func icerikGoster(demoPanel bool, data []byte) string {
	return middleware.Maskele(demoPanel, string(data))
}

// statusFromFsErr: symlink-güvenli dosya işlemlerinin errno'sunu HTTP durumuna
// çevirir. ELOOP/EXDEV = jail ihlali girişimi (403), ENOENT = 404, gerisi 500.
func statusFromFsErr(err error) int {
	switch {
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.EXDEV):
		return http.StatusForbidden
	case errors.Is(err, os.ErrNotExist), errors.Is(err, unix.ENOENT):
		return http.StatusNotFound
	case errors.Is(err, unix.ENOTDIR), errors.Is(err, unix.EISDIR), errors.Is(err, errTooBig):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

type mkdirReq struct {
	Yol string `json:"yol"`
}

func (h *Handlers) Mkdir(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req mkdirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	// TOCTOU symlink-güvenli: mkdir -p'yi her bileşende Mkdirat+O_NOFOLLOW ile yürüt; yeni
	// dizinler fd üzerinden tenant'a chown edilir (bkz. safeio.go). Eski os.MkdirAll(abs)
	// resolved-string üzerinde çalışıp ara-dizin symlink takasına açıktı.
	if err := mkdirAllBeneath(home, req.Yol, sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "yol": req.Yol})
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	rel := r.URL.Query().Get("yol")
	if p := relClean(rel); p == "" || p == "." {
		httpx.WriteError(w, http.StatusBadRequest, "ana ev dizini silinemez")
		return
	}
	// TOCTOU symlink-güvenli: parent'ı openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS) ile pinle,
	// fd-özyinelemeli unlinkat ile sil (bkz. safeio.go). Eski os.RemoveAll(abs) resolved-string
	// üzerinde çalışıp ara-dizin symlink takasıyla jail-dışı silmeye kandırılabilirdi.
	if err := removeAllBeneath(home, rel); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen": rel})
}

func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	rel := r.URL.Query().Get("yol")
	if rel == "" {
		rel = "/"
	}
	// Tek tenant'ın paralel yüklemelerle diski/goroutine'leri tüketmesini engelle;
	// kota doluysa beklemeden 429 (kuyruğa almak slow-DoS'u yeniden üretirdi).
	birak, ok := httpx.YuklemeSlotVeyaHata(w, "files:"+sk)
	if !ok {
		return
	}
	defer birak()
	// Büyük dosya yüklemeleri sunucunun kısa varsayılan zaman aşımını (bkz.
	// cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// DoS savunması: istek gövdesini üst sınırla kes. MaxBytesReader hem RAM'i hem
	// diski korur; sınır aşılınca okuma *http.MaxBytesError döner. ExtendBodyLimit
	// (r.Body = ... değil) çünkü global httpx.LimitBody gövdeyi zaten 2 MiB'e
	// sarmaladı — üstüne sarmak iç içe iki sınırın KÜÇÜĞÜNÜ geçerli kılar,
	// yani yükleme 2 MiB'de kesilirdi.
	httpx.ExtendBodyLimit(w, r, MaxUploadBytes)
	// maxMemory küçük → gövde RAM yerine geçici diske taşar (RAM DoS engellenir).
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		// multipart ayrıştırıcı MaxBytesError'ı bazen sarmalamadan düz metne
		// çevirdiği için metin kontrolü de korunur.
		if httpx.GovdeSinirAsildi(err) || strings.Contains(err.Error(), "too large") {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "yükleme boyutu sınırı aştı (max 2 GiB)")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "form parse: "+err.Error())
		return
	}
	file, fh, err := r.FormFile("dosya")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "dosya alanı bulunamadı: "+err.Error())
		return
	}
	defer file.Close()
	if fh.Size > MaxUploadBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "dosya çok büyük (max 2 GiB)")
		return
	}
	// TOCTOU symlink-güvenli: hedefi openat2 ile aç (ara-bileşen/leaf symlink REDDEDİLİR),
	// fd'ye akışla kopyala, sonra fd üzerinden tenant'a chown (bkz. safeio.go). Eski
	// os.Create(abs) resolved-string üzerinde çalışıp symlink takasına açıktı.
	dstRel := filepath.Join(rel, fh.Filename)
	written, err := copyStreamBeneath(home, dstRel, file, sk)
	if err != nil {
		_ = removeAllBeneath(home, dstRel)
		httpx.WriteError(w, http.StatusInternalServerError, "yazma: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":    true,
		"yol":   filepath.ToSlash(filepath.Join(rel, fh.Filename)),
		"boyut": written,
		"isim":  fh.Filename,
	})
}

// yetkiRWX: izin bitlerini rwx dizesine çevirir (ör. 0644 → "rw-r--r--").
func yetkiRWX(m os.FileMode) string {
	const rwx = "rwxrwxrwx"
	p := m.Perm()
	b := []byte("---------")
	for i := 0; i < 9; i++ {
		if p&(1<<uint(8-i)) != 0 {
			b[i] = rwx[i]
		}
	}
	return string(b)
}

func statusFromErr(err error) int {
	switch err {
	case os.ErrNotExist:
		return http.StatusNotFound
	case errDemo:
		return http.StatusForbidden
	case errBadUser, errEscape:
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
