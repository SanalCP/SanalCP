package iceaktarim

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/backups"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/sqlimport"
)

// dumpKapisi: aynı anda tek büyük import. Her istek sıkıştırılmış + açılmış
// geçici dosya tutabildiği için paralel istekler diski tüketmesin.
var dumpKapisi = make(chan struct{}, 1)

type sqlCevap struct {
	OK      bool   `json:"ok"`
	DBAdi   string `json:"db_adi"`
	Bayt    int64  `json:"bayt"`
	Bosalti bool   `json:"bosaltildi"`
}

// SQLYukle — POST /domains/{id}/ice-aktarim/sql
//
// multipart: dump (dosya), db_name (hedef veritabanı), bosalt ("1" → içe
// aktarmadan önce tüm tabloları düşür).
//
// 🔴 Dump hedef veritabanının KENDİ kullanıcısıyla uygulanır (internal/sqlimport).
// Panel root çalıştığı için `mysql <db>` ile root@localhost'a akıtmak, dump'ın
// içine konacak `USE mysql; GRANT ...` ifadeleriyle DB sunucusunun tamamını
// devretmek olurdu.
func (h *Handlers) SQLYukle(w http.ResponseWriter, r *http.Request) {
	domainID, _, sk, err := h.domain(r)
	if err != nil {
		httpx.WriteError(w, durumKodu(err), err.Error())
		return
	}
	// Tek tenant'ın paralel dump yüklemeleriyle diski tüketmesini engelle;
	// kota doluysa beklemeden 429 (kuyruğa almak slow-DoS'u yeniden üretirdi).
	birak, ok := httpx.YuklemeSlotVeyaHata(w, "sql:"+strconv.FormatInt(domainID, 10))
	if !ok {
		return
	}
	defer birak()
	// Büyük SQL dump yüklemeleri sunucunun kısa varsayılan zaman aşımını (bkz.
	// cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// ExtendBodyLimit (r.Body = ... değil): global httpx.LimitBody gövdeyi zaten
	// 2 MiB'e sarmaladı, üstüne sarmak iç içe iki sınırın KÜÇÜĞÜNÜ geçerli kılardı.
	httpx.ExtendBodyLimit(w, r, MaxDumpBayt+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "çok parçalı (multipart) gövde gerekli")
		return
	}

	var (
		dbAdi  string
		bosalt bool
		bayt   int64
		hamAd  string
	)
	ham, err := os.CreateTemp("", "sanalcp-ice-aktarim-dump-*.upload")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici dosya oluşturulamadı")
		return
	}
	hamAd = ham.Name()
	handoff := false
	defer func() {
		_ = ham.Close()
		if !handoff {
			_ = os.Remove(hamAd)
		}
	}()
	defer ham.Close()
	if err := ham.Chmod(0o600); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici dosya izni ayarlanamadı")
		return
	}

	var dumpGeldi bool
	for {
		p, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gövde okunamadı veya boyut sınırı aşıldı")
			return
		}
		switch p.FormName() {
		case "db_name":
			dbAdi = strings.TrimSpace(alanOku(p))
		case "bosalt":
			v := strings.TrimSpace(alanOku(p))
			bosalt = v == "1" || strings.EqualFold(v, "true")
		case "dump":
			n, cerr := io.Copy(ham, io.LimitReader(p, MaxDumpBayt+1))
			_ = p.Close()
			if cerr != nil {
				httpx.WriteError(w, http.StatusBadRequest, "dump okunamadı: "+cerr.Error())
				return
			}
			if n > MaxDumpBayt {
				httpx.WriteError(w, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("dump %d GiB sınırını aşıyor", MaxDumpBayt>>30))
				return
			}
			bayt = n
			dumpGeldi = true
			continue
		}
		_ = p.Close()
	}
	if !dumpGeldi {
		httpx.WriteError(w, http.StatusBadRequest, "dump alanı zorunlu")
		return
	}

	hedef, err := h.dbHedefi(r, domainID, dbAdi)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := ham.Sync(); err != nil {
		httpx.WriteError(w, 500, "dump diske yazılamadı")
		return
	}
	if err := ham.Close(); err != nil {
		httpx.WriteError(w, 500, "dump kapatılamadı")
		return
	}
	var alanAdi string
	if err = h.DB.QueryRowContext(r.Context(), `SELECT alan_adi FROM domains WHERE id=?`, domainID).Scan(&alanAdi); err != nil {
		httpx.WriteError(w, 500, "domain okunamadı")
		return
	}
	jobID, err := h.newJob(r.Context(), domainID, "database", hedef.DBAdi, "Kurtarma noktası bekleniyor")
	if err != nil {
		httpx.WriteError(w, 500, "aktarım işi oluşturulamadı")
		return
	}
	handoff = true
	go h.runSQLJob(jobID, domainID, alanAdi, sk, hamAd, hedef, bosalt)
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true, "job_id": jobID, "db_adi": hedef.DBAdi, "bayt": bayt, "bosaltildi": bosalt})
}

func (h *Handlers) runSQLJob(jobID, domainID int64, alanAdi, sk, dumpPath string, hedef sqlimport.Hedef, bosalt bool) {
	defer os.Remove(dumpPath)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	select {
	case dumpKapisi <- struct{}{}:
		defer func() { <-dumpKapisi }()
	case <-ctx.Done():
		h.jobUpdate(jobID, "failed", 0, "Sıra zaman aşımı", ctx.Err().Error(), "")
		return
	}
	h.jobUpdate(jobID, "running", 5, "Kurtarma noktası oluşturuluyor", "", "")
	name, _, err := backups.CreateRecoveryArchive(ctx, h.DB, domainID, alanAdi, sk, "SQL içe aktarımı öncesi otomatik kurtarma noktası")
	if err != nil {
		h.jobUpdate(jobID, "failed", 5, "İşlem durduruldu", "Kurtarma noktası oluşturulamadı: "+err.Error(), "")
		return
	}
	h.jobUpdate(jobID, "running", 30, "SQL dump içe aktarılıyor", "", name)
	f, err := os.Open(dumpPath)
	if err == nil {
		defer f.Close()
	}
	if err == nil {
		var src io.Reader
		src, err = dumpAkisi(f)
		if c, ok := src.(io.Closer); ok {
			defer c.Close()
		}
		if err == nil && bosalt {
			err = sqlimport.TablolariSil(ctx, hedef)
		}
		if err == nil {
			err = sqlimport.Uygula(ctx, hedef, src)
		}
	}
	if err == nil {
		h.jobUpdate(jobID, "success", 100, "Veritabanı aktarımı tamamlandı", "", name)
		return
	}
	mesaj := err.Error()
	h.jobUpdate(jobID, "running", 75, "Aktarım başarısız; veritabanı geri alınıyor", mesaj, name)
	backupPath := filepath.Join(backups.BackupRoot, sk, name)
	rbCtx, rbCancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer rbCancel()
	if rb := backups.RestoreRecoveryArchive(rbCtx, h.DB, domainID, sk, backupPath, "database", hedef.DBAdi); rb != nil {
		h.jobUpdate(jobID, "failed", 100, "Geri alma başarısız", mesaj+"; rollback: "+rb.Error(), name)
		return
	}
	h.jobUpdate(jobID, "rolled_back", 100, "Hata sonrası veritabanı geri alındı", mesaj, name)
}

// dbHedefi: hedef veritabanının BU domaine ait olduğunu doğrular ve o
// veritabanının kendi kimlik bilgilerini döner.
func (h *Handlers) dbHedefi(r *http.Request, domainID int64, dbAdi string) (sqlimport.Hedef, error) {
	if dbAdi == "" {
		return sqlimport.Hedef{}, errors.New("db_name alanı zorunlu")
	}
	var kullanici, sifreli, host string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db_user, db_pass_plain, COALESCE(db_host,'localhost')
		   FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbAdi).
		Scan(&kullanici, &sifreli, &host)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlimport.Hedef{}, errors.New("bu veritabanı bu domaine ait değil")
	}
	if err != nil {
		return sqlimport.Hedef{}, errors.New("veritabanı bilgisi okunamadı")
	}
	parola, err := hesaplar.DecryptDBPassword(sifreli)
	if err != nil {
		return sqlimport.Hedef{}, errors.New("veritabanı parolası çözülemedi")
	}
	return sqlimport.Hedef{DBAdi: dbAdi, Kullanici: kullanici, Parola: parola, Host: host}, nil
}

// dumpAkisi: geçici dosyayı başa sarar ve gzip ise şeffaf olarak açar.
// Uzantıya DEĞİL, magic byte'a bakılır (kullanıcı .sql adıyla gzip yükleyebilir).
func dumpAkisi(f *os.File) (io.Reader, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.New("geçici dosya okunamadı")
	}
	br := bufio.NewReaderSize(f, 64<<10)
	sihir, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return nil, errors.New("dump okunamadı: " + err.Error())
	}
	if len(sihir) == 2 && sihir[0] == 0x1f && sihir[1] == 0x8b {
		gzr, gerr := gzip.NewReader(br)
		if gerr != nil {
			return nil, errors.New("gzip açılamadı: " + gerr.Error())
		}
		// Not: açılmış boyut için ayrı bir sınır konmaz — veri diske değil
		// doğrudan mysql'e akar ve hedef veritabanı zaten disk kotasına tabidir.
		return gzr, nil
	}
	return br, nil
}

// alanOku: küçük bir multipart form alanını okur (sınırlı).
func alanOku(p *multipart.Part) string {
	b, _ := io.ReadAll(io.LimitReader(p, 4<<10))
	return string(b)
}
