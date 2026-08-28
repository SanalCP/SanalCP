package iceaktarim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/archivex"
	"sanalcp/internal/backups"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
)

// isaretDosyalari: arşivde aranan uygulama imzaları. Hem kullanıcıya "bu bir
// WordPress yedeği" demek hem de aktarım sonrası config güncellemesini
// önerebilmek için.
var isaretDosyalari = []string{
	"wp-config.php",     // WordPress
	"artisan",           // Laravel
	".env",              // Laravel / Symfony
	"configuration.php", // Joomla
	"settings.php",      // Drupal
	"index.php",
}

// ArsivOzet: yükleme+analiz yanıtı.
type ArsivOzet struct {
	StageID    string        `json:"stage_id"`
	DosyaAdi   string        `json:"dosya_adi"`
	Boyut      int64         `json:"boyut"`
	Ozet       archivex.Ozet `json:"ozet"`
	Uygulama   string        `json:"uygulama"`    // wordpress | laravel | joomla | drupal | ""
	ConfigYolu string        `json:"config_yolu"` // uygulamanın kök klasörü (arşiv içi)
	Uyarilar   []string      `json:"uyarilar"`
}

// ArsivYukle — POST /domains/{id}/ice-aktarim/arsiv
//
// Arşivi tenant'ın staging alanına akıtır ve ÇIKARMADAN analiz eder. Çıkarma
// ayrı bir çağrıdır (ArsivUygula): kullanıcı önce kök klasörü ve hedefi
// onaylasın. Arşiv iki kez yüklenmez — staging kimliğiyle referans verilir.
func (h *Handlers) ArsivYukle(w http.ResponseWriter, r *http.Request) {
	_, home, sk, err := h.domain(r)
	if err != nil {
		httpx.WriteError(w, durumKodu(err), err.Error())
		return
	}
	stageTemizle(home)

	// Tek tenant'ın paralel arşiv yüklemeleriyle diski tüketmesini engelle;
	// kota doluysa beklemeden 429 (kuyruğa almak slow-DoS'u yeniden üretirdi).
	birak, ok := httpx.YuklemeSlotVeyaHata(w, "arsiv:"+sk)
	if !ok {
		return
	}
	defer birak()
	// Büyük arşiv yüklemeleri sunucunun kısa varsayılan zaman aşımını (bkz.
	// cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// ExtendBodyLimit (r.Body = ... değil): global httpx.LimitBody gövdeyi zaten
	// 2 MiB'e sarmaladı, üstüne sarmak iç içe iki sınırın KÜÇÜĞÜNÜ geçerli kılardı.
	httpx.ExtendBodyLimit(w, r, MaxArsivBayt+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "çok parçalı (multipart) gövde gerekli")
		return
	}
	var part *multipart.Part
	for {
		p, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "arşiv yüklenemedi veya boyut sınırı aşıldı")
			return
		}
		if p.FormName() == "archive" {
			part = p
			break
		}
		_ = p.Close()
	}
	if part == nil {
		httpx.WriteError(w, http.StatusBadRequest, "archive alanı zorunlu")
		return
	}
	defer part.Close()

	dosyaAdi := part.FileName()
	stageID, boyut, err := stageYaz(home, sk, dosyaAdi, part)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	mutlak := path.Join(home, stagingRel, stageID)

	tur := archivex.TuruBelirle(stageID)
	ozet, err := archivex.Ozetle(mutlak, tur, isaretDosyalari)
	if err != nil {
		_ = jailpath.Sil(home, path.Join(stagingRel, stageID))
		httpx.WriteError(w, http.StatusBadRequest, "arşiv okunamadı: "+err.Error())
		return
	}

	cevap := ArsivOzet{
		StageID: stageID, DosyaAdi: path.Base(dosyaAdi), Boyut: boyut,
		Ozet: ozet, Uyarilar: []string{},
	}
	cevap.Uygulama, cevap.ConfigYolu = uygulamaTespit(ozet)
	if ozet.UyeSayisi == 0 {
		cevap.Uyarilar = append(cevap.Uyarilar, "Arşiv boş görünüyor.")
	}
	if ozet.KokKlasor == "" && len(ozet.Kokler) > 1 {
		cevap.Uyarilar = append(cevap.Uyarilar,
			"Arşivin kökünde birden fazla girdi var; tek bir kapsayıcı klasör bulunamadı.")
	}
	if tur == archivex.TurZip || tur == archivex.TurRar {
		if !archivex.StripDesteklenirMi() {
			cevap.Uyarilar = append(cevap.Uyarilar,
				"Bu sunucuda bsdtar kurulu değil — zip/rar arşivlerinde kök klasör atlanamaz.")
		}
	}
	httpx.WriteJSON(w, http.StatusOK, cevap)
}

// uygulamaTespit: işaret dosyalarından uygulamayı ve arşiv içindeki kök
// dizinini çıkarır. En sığ (kök klasöre en yakın) eşleşme seçilir.
func uygulamaTespit(oz archivex.Ozet) (uygulama, dizin string) {
	sec := func(ad string) (string, bool) {
		yollar := oz.Isaretler[ad]
		if len(yollar) == 0 {
			return "", false
		}
		en := yollar[0]
		for _, y := range yollar[1:] {
			if strings.Count(y, "/") < strings.Count(en, "/") {
				en = y
			}
		}
		return en, true
	}
	if d, ok := sec("wp-config.php"); ok {
		return "wordpress", d
	}
	if d, ok := sec("artisan"); ok {
		return "laravel", d
	}
	if d, ok := sec("configuration.php"); ok {
		return "joomla", d
	}
	if d, ok := sec("settings.php"); ok {
		return "drupal", d
	}
	if d, ok := sec(".env"); ok {
		return "laravel", d
	}
	return "", ""
}

type arsivUygulaReq struct {
	StageID string `json:"stage_id"`
	Hedef   string `json:"hedef"`    // ev dizinine göreli; boş → public_html
	KokAtla bool   `json:"kok_atla"` // arşivin tek kapsayıcı klasörünü atla
	Temizle bool   `json:"temizle"`  // hedefin İÇERİĞİNİ önce sil
}

type arsivUygulaCevap struct {
	OK      bool   `json:"ok"`
	JobID   int64  `json:"job_id"`
	Hedef   string `json:"hedef"`
	Atlanan string `json:"atlanan_kok,omitempty"`
	Silindi bool   `json:"hedef_temizlendi"`
}

// ArsivUygula — POST /domains/{id}/ice-aktarim/arsiv-uygula
func (h *Handlers) ArsivUygula(w http.ResponseWriter, r *http.Request) {
	domainID, home, sk, err := h.domain(r)
	if err != nil {
		httpx.WriteError(w, durumKodu(err), err.Error())
		return
	}
	var req arsivUygulaReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	mutlakArsiv, err := stageYol(home, req.StageID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	hedefRel, err := hedefDogrula(req.Hedef)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Hedef dizin symlink-güvenli oluşturulur: bileşenlerden biri symlink ise
	// REDDEDİLİR (panel root çalışıyor — bkz. paket açıklaması).
	if err := jailpath.DizinOlustur(home, hedefRel, sk); err != nil {
		httpx.WriteError(w, http.StatusBadRequest,
			"hedef dizin güvenli değil (symlink?): "+err.Error())
		return
	}
	strip := 0
	atlanan := ""
	if req.KokAtla {
		tur := archivex.TuruBelirle(mutlakArsiv)
		ozet, oerr := archivex.Ozetle(mutlakArsiv, tur, nil)
		if oerr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "arşiv okunamadı: "+oerr.Error())
			return
		}
		if ozet.KokKlasor == "" {
			httpx.WriteError(w, http.StatusBadRequest,
				"arşivde tek bir kapsayıcı klasör yok; kök atlanamaz")
			return
		}
		strip = 1
		atlanan = ozet.KokKlasor
	}

	var alanAdi string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT alan_adi FROM domains WHERE id=?`, domainID).Scan(&alanAdi); err != nil {
		httpx.WriteError(w, 500, "domain okunamadı")
		return
	}
	jobID, err := h.newJob(r.Context(), domainID, "files", hedefRel, "Kurtarma noktası bekleniyor")
	if err != nil {
		httpx.WriteError(w, 500, "aktarım işi oluşturulamadı")
		return
	}
	go h.runArchiveJob(jobID, domainID, alanAdi, home, sk, req.StageID, mutlakArsiv, hedefRel, strip, req.Temizle)
	httpx.WriteJSON(w, http.StatusAccepted, arsivUygulaCevap{OK: true, JobID: jobID, Hedef: hedefRel, Atlanan: atlanan, Silindi: req.Temizle})
}

func (h *Handlers) runArchiveJob(jobID, domainID int64, alanAdi, home, sk, stageID, archive, target string, strip int, temizle bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	h.jobUpdate(jobID, "running", 5, "Kurtarma noktası oluşturuluyor", "", "")
	name, _, err := backups.CreateRecoveryArchive(ctx, h.DB, domainID, alanAdi, sk, "Web sitesi içe aktarımı öncesi otomatik kurtarma noktası")
	if err != nil {
		h.jobUpdate(jobID, "failed", 5, "İşlem durduruldu", "Kurtarma noktası oluşturulamadı: "+err.Error(), "")
		return
	}
	h.jobUpdate(jobID, "running", 25, "Arşiv güvenli biçimde çıkarılıyor", "", name)
	if temizle {
		if err = jailpath.IceriginiSil(home, target); err != nil {
			h.jobUpdate(jobID, "failed", 25, "Hedef temizlenemedi", err.Error(), name)
			return
		}
	}
	hedefMutlak := path.Join(home, target)
	out, err := archivex.GuvenliCikarStrip(archive, hedefMutlak, sk, strip)
	if err == nil {
		sahiplendir(hedefMutlak, sk)
		_ = jailpath.Sil(home, path.Join(stagingRel, stageID))
		h.jobUpdate(jobID, "success", 100, "Dosya aktarımı tamamlandı", "", name)
		return
	}
	mesaj := err.Error()
	if s := strings.TrimSpace(out); s != "" {
		mesaj += ": " + s
	}
	h.jobUpdate(jobID, "running", 75, "Aktarım başarısız; otomatik geri alınıyor", mesaj, name)
	backupPath := filepath.Join(backups.BackupRoot, sk, name)
	scope := rollbackScope(target)
	rbCtx, rbCancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer rbCancel()
	if rb := backups.RestoreRecoveryArchive(rbCtx, h.DB, domainID, sk, backupPath, scope, ""); rb != nil {
		h.jobUpdate(jobID, "failed", 100, "Geri alma başarısız", mesaj+"; rollback: "+rb.Error(), name)
		return
	}
	h.jobUpdate(jobID, "rolled_back", 100, "Hata sonrası dosyalar geri alındı", mesaj, name)
}

func rollbackScope(target string) string {
	if target == "public_html" || strings.HasPrefix(target, "public_html/") {
		return "files"
	}
	return "home"
}

// hedefDogrula: hedefi ev dizinine göreli, temiz bir yola indirger.
//
// jailpath zaten home dışına çıkışı engelliyor; buradaki ek kural staging
// alanının hedef seçilmesini önlemek (kendi kaynağının üstüne çıkarma).
func hedefDogrula(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "public_html"
	}
	temiz := strings.Trim(path.Clean("/"+strings.ReplaceAll(rel, "\\", "/")), "/")
	if temiz == "" || temiz == "." {
		return "", errors.New("hedef ev dizininin kendisi olamaz")
	}
	if temiz == stagingRel || strings.HasPrefix(temiz, stagingRel+"/") {
		return "", errors.New("hedef olarak içe aktarma çalışma alanı seçilemez")
	}
	return temiz, nil
}
