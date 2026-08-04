package iceaktarim

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

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
	domainID, _, _, err := h.domain(r)
	if err != nil {
		httpx.WriteError(w, durumKodu(err), err.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxDumpBayt+(1<<20))
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
	defer os.Remove(hamAd)
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

	select {
	case dumpKapisi <- struct{}{}:
		defer func() { <-dumpKapisi }()
	case <-r.Context().Done():
		httpx.WriteError(w, http.StatusRequestTimeout, "içe aktarma sırası beklenirken istek sona erdi")
		return
	}

	src, acHata := dumpAkisi(ham)
	if acHata != nil {
		httpx.WriteError(w, http.StatusBadRequest, acHata.Error())
		return
	}
	if kapat, ok := src.(io.Closer); ok {
		defer kapat.Close()
	}

	if bosalt {
		if err := sqlimport.TablolariSil(r.Context(), hedef); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := sqlimport.Uygula(r.Context(), hedef, src); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sqlCevap{OK: true, DBAdi: hedef.DBAdi, Bayt: bayt, Bosalti: bosalt})
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
