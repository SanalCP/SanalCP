// Package backups: per-domain tar + DB dump yedek
package backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
	"sanalcp/internal/secretcrypt"

	"github.com/go-chi/chi/v5"
)

const BackupRoot = "/var/backups/sanalcp"

// box: backup_destinations.parola sütununu şifreler. main.go'da Init() ile
// bir kez ayarlanır (bkz. hesaplar/provisioner/middleware paketleriyle aynı
// paket-seviyesi singleton deseni).
var box *secretcrypt.Box

// Init: panelin sır şifreleme kutusunu ayarlar. main.go'da herhangi bir
// backup-destination isteğinden ÖNCE çağrılmalıdır.
func Init(b *secretcrypt.Box) { box = b }

// RemoveDomainBackups: bir domainin per-domain backup dizinini kaldırır.
// ÖNEMLİ: Domain silme akışından ÇAĞRILMAZ — müşteri yanlışlıkla silmiş olabilir,
// yedekler kurtarma için kasıtlı saklanır. Bu sadece operatörün açıkça istediği
// manuel temizlik (ör. "silinmiş domain yedeğini kalıcı kaldır") için bir yardımcıdır.
// Guard: sk mutlaka "c_" ile başlamalı ve yol BackupRoot altında olmalı (path-escape koruması).
func RemoveDomainBackups(sk string) error {
	if !adlar.SKGecerli(sk) {
		return fmt.Errorf("geçersiz kullanıcı: %q", sk)
	}
	dir := filepath.Join(BackupRoot, sk)
	if dir == BackupRoot || !strings.HasPrefix(dir, BackupRoot+"/") {
		return fmt.Errorf("güvensiz backup yolu: %q", dir)
	}
	return os.RemoveAll(dir)
}

type Yedek struct {
	ID              int64  `json:"id"`
	DomainID        int64  `json:"domain_id"`
	Tip             string `json:"tip"`
	Dosya           string `json:"dosya"`
	BoyutB          int64  `json:"boyut_b"`
	Notlar          string `json:"notlar"`
	Olusturma       string `json:"olusturma"`
	UzakDurum       string `json:"uzak_durum"`
	UzakHata        string `json:"uzak_hata,omitempty"`
	DogrulamaDurum  string `json:"dogrulama_durum"`
	DogrulamaHata   string `json:"dogrulama_hata,omitempty"`
	DogrulamaSHA256 string `json:"dogrulama_sha256,omitempty"`
	DogrulamaZamani string `json:"dogrulama_zamani,omitempty"`
}

type Handlers struct {
	DB *sql.DB
}

func (h *Handlers) lookupDomain(r *http.Request) (id int64, alanAdi, sk string, demo bool, err error) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var dmo int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &dmo)
	demo = dmo == 1
	return
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_id, tip, dosya, boyut_b, notlar,
		        DATE_FORMAT(created_at,'%Y-%m-%d %H:%i'), uzak_durum, uzak_hata,
		        dogrulama_durum, dogrulama_hata, dogrulama_sha256,
		        COALESCE(DATE_FORMAT(dogrulama_zamani,'%Y-%m-%d %H:%i'),'')
		 FROM backups WHERE domain_id=? ORDER BY id DESC`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := make([]Yedek, 0)
	for rows.Next() {
		var y Yedek
		if err := rows.Scan(&y.ID, &y.DomainID, &y.Tip, &y.Dosya, &y.BoyutB,
			&y.Notlar, &y.Olusturma, &y.UzakDurum, &y.UzakHata, &y.DogrulamaDurum,
			&y.DogrulamaHata, &y.DogrulamaSHA256, &y.DogrulamaZamani); err == nil {
			out = append(out, y)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// OzetSatir: bir domainin yedek özeti (sunucu-geneli görünüm için).
type OzetSatir struct {
	DomainID int64  `json:"domain_id"`
	AlanAdi  string `json:"alan_adi"`
	Sayi     int    `json:"sayi"`
	ToplamB  int64  `json:"toplam_b"`
	SonYedek string `json:"son_yedek"`
	// Kırılım: arayüz "7 saklanır ama 9 var" çelişkisini ancak otomatik ve
	// manuel sayıyı ayrı gösterebilirse açıklayabilir.
	OtoSayi         int    `json:"oto_sayi"`
	ManuelSayi      int    `json:"manuel_sayi"`
	Freq            string `json:"freq"`
	Retention       int    `json:"retention"`
	ManuelRetention int    `json:"manuel_retention"`
}

// otomatikDosya: dosya adından yedeğin otomatik mi olduğunu söyler.
//
// runOneBackup "<sk>-auto-<damga>.tar.gz", manuel Create ise "<sk>-<damga>.tar.gz"
// yazar. Ayrım sk ÖNEKİNE dayanmalı, düz "-auto-" araması yapılmamalı: sistem
// kullanıcısı domainden türediği için "c_my-auto" gibi bir sk mümkündür ve onun
// manuel dosyası da "-auto-" içerir. Önek kontrolü bu tuzağa düşmez.
func otomatikDosya(sk, ad string) bool {
	return strings.HasPrefix(ad, sk+"-auto-")
}

// Ozet: GET /admin/backups/ozet — TÜM domainlerin yedek özeti (dosya sisteminden, gerçek disk kullanımı).
func (h *Handlers) Ozet(w http.ResponseWriter, r *http.Request) {
	// Kapsam: bayi yalnız kendi müşterilerinin yedek özetini görür.
	kosul, arg := middleware.KapsamSQL(r, "d")
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT d.id, d.alan_adi, d.sistem_kullanici,
		        COALESCE(d.backup_freq,'none'), COALESCE(d.backup_hour,3),
		        COALESCE(d.backup_retention,7), COALESCE(d.backup_manuel_retention,0)
		 FROM domains d`+kosul+` ORDER BY d.alan_adi`, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []OzetSatir{}
	var toplamB int64
	var toplamSayi, toplamOto, toplamManuel int
	// Banner artık sabit metin değil, gerçek veriden kuruluyor. Domainler farklı
	// ayarlarda olabildiği için tek değer yerine aralık taşınır; arayüz min==max
	// ise tek sayı, değilse "1–30" gösterir.
	otoDomain, saat := 0, -1
	saatKarisik := false
	retMin, retMax := 0, 0
	manRetMin, manRetMax := -1, -1
	for rows.Next() {
		var id int64
		var alanAdi, sk string
		var hour int
		s := OzetSatir{}
		if err := rows.Scan(&id, &alanAdi, &sk, &s.Freq, &hour,
			&s.Retention, &s.ManuelRetention); err != nil {
			continue
		}
		s.DomainID, s.AlanAdi = id, alanAdi
		var sonMod time.Time
		if entries, e := os.ReadDir(filepath.Join(BackupRoot, sk)); e == nil {
			for _, en := range entries {
				if en.IsDir() || !strings.HasSuffix(en.Name(), ".tar.gz") {
					continue
				}
				fi, e2 := en.Info()
				if e2 != nil {
					continue
				}
				s.Sayi++
				if otomatikDosya(sk, en.Name()) {
					s.OtoSayi++
				} else {
					s.ManuelSayi++
				}
				s.ToplamB += fi.Size()
				if fi.ModTime().After(sonMod) {
					sonMod = fi.ModTime()
				}
			}
		}
		if !sonMod.IsZero() {
			s.SonYedek = sonMod.Format("2006-01-02 15:04")
		}
		if s.Freq != "none" {
			otoDomain++
			if saat == -1 {
				saat = hour
			} else if saat != hour {
				saatKarisik = true
			}
			if retMin == 0 || s.Retention < retMin {
				retMin = s.Retention
			}
			if s.Retention > retMax {
				retMax = s.Retention
			}
		}
		if manRetMin == -1 || s.ManuelRetention < manRetMin {
			manRetMin = s.ManuelRetention
		}
		if s.ManuelRetention > manRetMax {
			manRetMax = s.ManuelRetention
		}
		out = append(out, s)
		toplamB += s.ToplamB
		toplamSayi += s.Sayi
		toplamOto += s.OtoSayi
		toplamManuel += s.ManuelSayi
	}
	_ = rows.Err()
	if saatKarisik {
		saat = -1 // arayüz "domain bazında" yazar
	}
	if manRetMin == -1 {
		manRetMin, manRetMax = 0, 0
	}
	var hedefSayisi int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM backup_destinations WHERE aktif=1`).Scan(&hedefSayisi)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"domainler":       out,
		"toplam_boyut_b":  toplamB,
		"toplam_yedek":    toplamSayi,
		"toplam_oto":      toplamOto,
		"toplam_manuel":   toplamManuel,
		"hedef_sayisi":    hedefSayisi,
		"otomatik_domain": otoDomain,
		"zamanlama_saat":  saat, // -1: domainler farklı saatlerde
		"retention_min":   retMin,
		"retention_max":   retMax,
		"manuel_ret_min":  manRetMin,
		"manuel_ret_max":  manRetMax,
	})
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id, alanAdi, sk, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin yedeği alınamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "güvenlik")
		return
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(BackupRoot, sk)
	_ = os.MkdirAll(dir, 0700)
	dosya := fmt.Sprintf("%s-%s.tar.gz", sk, stamp)
	abs := filepath.Join(dir, dosya)

	boyut, err := createArchive(r.Context(), h.DB, id, alanAdi, sk, abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "yedek oluşturma: "+err.Error())
		return
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO backups(domain_id, tip, dosya, boyut_b, notlar) VALUES(?,?,?,?,?)`,
		id, TipManuel, dosya, boyut, "alan adı: "+alanAdi)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB kayıt: "+err.Error())
		return
	}
	yid, _ := res.LastInsertId()
	startVerification(h.DB, yid, id, alanAdi, sk, abs)
	// Uzak hedef varsa arkaplanda yükle (API cevabını bloke etme)
	pushToDestinationAsync(h.DB, id, yid, abs, dosya)

	// Manuel retention'ı BURADA uygula, scheduler'da değil: scheduler yalnızca
	// backup_freq != 'none' olan domainlere uğrar, dolayısıyla otomatik yedeği
	// kapalı bir domainde manuel sınır hiç işlemezdi. Yeni kayıt en yüksek
	// id'ye sahip olduğu için budama onu asla silmez.
	var manuelRet int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(backup_manuel_retention,0) FROM domains WHERE id=?`, id).
		Scan(&manuelRet); err == nil {
		if err := pruneManuel(h.DB, id, sk, manuelRet); err != nil {
			log.Printf("manuel retention domain=%d: %v", id, err)
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"id":      yid,
		"dosya":   dosya,
		"boyut_b": boyut,
		"yol":     abs,
	})
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bid, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	var sk, dosya, uzakDurum string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.sistem_kullanici, b.dosya, b.uzak_durum FROM backups b
		 JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, bid, id).Scan(&sk, &dosya, &uzakDurum)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "yedek bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = os.Remove(filepath.Join(BackupRoot, sk, dosya))
	deleteRemoteBestEffort(h.DB, id, dosya, uzakDurum)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM backups WHERE id=? AND domain_id=?`, bid, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	// Büyük yedek indirmeleri sunucunun kısa varsayılan yazma zaman aşımını
	// (bkz. cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bid, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	var sk, dosya, uzakDurum string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.sistem_kullanici, b.dosya, b.uzak_durum FROM backups b
		 JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, bid, id).Scan(&sk, &dosya, &uzakDurum)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "yedek bulunamadı")
		return
	}
	abs, err := ensureLocalBackup(r.Context(), h.DB, id, sk, dosya, uzakDurum)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+dosya+`"`)
	if st != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	_, _ = io.Copy(w, f)
}

// Verify refreshes the restore-readiness proof for a local or remote backup.
func (h *Handlers) Verify(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bid, _ := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	var domain, sk, dosya, uzak string
	err := h.DB.QueryRowContext(r.Context(), `SELECT d.alan_adi,d.sistem_kullanici,b.dosya,b.uzak_durum FROM backups b JOIN domains d ON d.id=b.domain_id WHERE b.id=? AND b.domain_id=?`, bid, id).Scan(&domain, &sk, &dosya, &uzak)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "yedek bulunamadı")
		return
	}
	abs, err := ensureLocalBackup(r.Context(), h.DB, id, sk, dosya, uzak)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if err = verifyAndRecord(ctx, h.DB, bid, id, domain, sk, abs); err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "yedek doğrulanamadı: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "durum": "dogrulandi"})
}
