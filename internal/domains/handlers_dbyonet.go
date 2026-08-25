package domains

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/sqlimport"

	"github.com/go-chi/chi/v5"
)

// dbKullanicilariGetir: bir domain+db_name icin GRANT'li tum kullanicilari
// dondurur (Task 4-7'de paylasilan yardimci).
func dbKullanicilariGetir(ctx context.Context, db *sql.DB, domainID int64, dbAdi string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbAdi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

type dbKullaniciSatiri struct {
	ID          int64  `json:"id"`
	DBKullanici string `json:"db_kullanici"`
	DBParola    string `json:"db_parola"`
	Olusturulma string `json:"olusturulma"`
}

type dbGrupDetay struct {
	DBAdi        string              `json:"db_adi"`
	DBHost       string              `json:"db_host"`
	Charset      string              `json:"charset"`
	Collation    string              `json:"collation"`
	BoyutMB      float64             `json:"boyut_mb"`
	Kullanicilar []dbKullaniciSatiri `json:"kullanicilar"`
}

// DatabaseGrupDetay: GET /domains/{id}/databases/{dbAdi}
func (h *Handlers) DatabaseGrupDetay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? AND db_name=? ORDER BY id`, id, dbAdi)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB sorgu: "+err.Error())
		return
	}
	defer rows.Close()

	var out dbGrupDetay
	out.DBAdi = dbAdi
	for rows.Next() {
		var k dbKullaniciSatiri
		var host string
		if err := rows.Scan(&k.ID, &k.DBKullanici, &host, &k.DBParola, &k.Olusturulma); err != nil {
			continue
		}
		out.DBHost = host
		if dec, err := hesaplar.DecryptDBPassword(k.DBParola); err == nil {
			k.DBParola = dec
		} else {
			// Çözülemeyen şifreli metni ASLA "parola" diye istemciye dönme
			// (pma.TokenIste aynı durumda 500 döner, burada göstergeyi
			// boş bırakmak yeterli — kart zaten diğer kullanıcıları listeler).
			k.DBParola = ""
		}
		out.Kullanicilar = append(out.Kullanicilar, k)
	}
	if len(out.Kullanicilar) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var boyutMB sql.NullFloat64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT SUM(data_length+index_length)/1024/1024 FROM information_schema.tables WHERE table_schema=?`,
		dbAdi).Scan(&boyutMB)
	out.BoyutMB = boyutMB.Float64

	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name=?`,
		dbAdi).Scan(&out.Charset, &out.Collation)

	httpx.WriteJSON(w, http.StatusOK, out)
}

type dbIsimDegistirReq struct {
	YeniSonek string `json:"yeni_sonek"`
}

// DatabaseIsimDegistir: PUT /domains/{id}/databases/{dbAdi}/isim
func (h *Handlers) DatabaseIsimDegistir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	eskiAd := chi.URLParam(r, "dbAdi")

	var sk string
	var isDemo int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin veritabanı adı değiştirilemez")
		return
	}

	var req dbIsimDegistirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if !hesaplar.GecerliDBSonek(req.YeniSonek) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
		return
	}
	yeniAd := sk + "_" + req.YeniSonek
	if !hesaplar.GecerliDBKimlik(yeniAd) {
		httpx.WriteError(w, http.StatusBadRequest, "veritabanı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
		return
	}

	kullanicilar, err := dbKullanicilariGetir(r.Context(), h.DB, id, eskiAd)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sorgu: "+err.Error())
		return
	}
	if len(kullanicilar) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var cakisma int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM db_accounts WHERE db_name=?`, yeniAd).Scan(&cakisma); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "çakışma sorgusu: "+err.Error())
		return
	}
	if cakisma > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu isimde bir veritabanı zaten var: "+yeniAd)
		return
	}
	// Panel metadata'sında (db_accounts) kayıt olmasa da MariaDB'de fiziksel
	// olarak bir şema kalmış olabilir (ör. başarısız/yarım kalmış önceki bir
	// rename'den) — yalnız db_accounts'a bakmak bu artığı gözden kaçırıp
	// CREATE DATABASE IF NOT EXISTS + dump'ı onun ÜZERİNE birleştirebilirdi.
	var semaVarMi int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?`, yeniAd).Scan(&semaVarMi); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "şema çakışma sorgusu: "+err.Error())
		return
	}
	if semaVarMi > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu isimde bir MariaDB şeması zaten var (panelin bilmediği): "+yeniAd)
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	if err := hesaplar.MySQLRenameDB(r.Context(), h.DB, id, eskiAd, yeniAd, kullanicilar); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "isim değiştirme: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "eski_ad": eskiAd, "yeni_ad": yeniAd,
		"uyari": "Veritabanı adını kullanan uygulama ayar dosyalarınızı (örn. wp-config.php) elle güncellemeniz gerekir.",
	})
}

type dbKullaniciEkleReq struct {
	KullaniciTipi   string `json:"kullanici_tipi"` // "yeni" | "mevcut"
	KullaniciSonek  string `json:"kullanici_sonek"`
	MevcutKullanici string `json:"mevcut_kullanici"`
	Parola          string `json:"parola"`
}

// DatabaseKullaniciEkle: POST /domains/{id}/databases/{dbAdi}/kullanicilar
// Kota kontrolü YOK — yeni DB değil, mevcut DB'ye ek kullanıcı (bkz. spec kararı).
func (h *Handlers) DatabaseKullaniciEkle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")

	var sk string
	var isDemo int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe kullanıcı eklenemez")
		return
	}

	var dbVarMi int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&dbVarMi); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı sorgu: "+err.Error())
		return
	}
	if dbVarMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var req dbKullaniciEkleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}

	var dbKullanici, parola string
	mevcutModu := req.KullaniciTipi == "mevcut"

	if mevcutModu {
		if req.MevcutKullanici == "" || !hesaplar.GecerliDBKimlik(req.MevcutKullanici) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz mevcut kullanıcı")
			return
		}
		var sahip int
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_user=?`, id, req.MevcutKullanici).Scan(&sahip); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sorgu: "+err.Error())
			return
		}
		if sahip == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "seçilen kullanıcı bu domaine ait değil")
			return
		}
		dbKullanici = req.MevcutKullanici
	} else {
		if req.KullaniciSonek == "" || !hesaplar.GecerliDBSonek(req.KullaniciSonek) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
			return
		}
		dbKullanici = sk + "_" + req.KullaniciSonek
		if !hesaplar.GecerliDBKimlik(dbKullanici) {
			httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
			return
		}
		if req.Parola == "" {
			parola = hesaplar.RandomParola(24)
		} else {
			if ok, neden := hesaplar.ParolaGucluMu(req.Parola); !ok {
				httpx.WriteError(w, http.StatusBadRequest, neden)
				return
			}
			parola = req.Parola
		}
	}

	var zatenVar int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=?`, id, dbAdi, dbKullanici).Scan(&zatenVar); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "erişim sorgu: "+err.Error())
		return
	}
	if zatenVar > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu kullanıcının zaten bu veritabanına erişimi var")
		return
	}

	if mevcutModu {
		if err := hesaplar.MySQLGrantExistingUser(h.DB, id, dbAdi, dbKullanici); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı ekleme: "+err.Error())
			return
		}
		var encParola string
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT db_pass_plain FROM db_accounts WHERE domain_id=? AND db_user=? LIMIT 1`, id, dbKullanici).Scan(&encParola)
		if dec, err := hesaplar.DecryptDBPassword(encParola); err == nil {
			parola = dec
		}
	} else {
		if err := hesaplar.MySQLGrantNewUser(h.DB, id, dbAdi, dbKullanici, parola); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı ekleme: "+err.Error())
			return
		}
	}

	var yeniID int64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT id FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=?`, id, dbAdi, dbKullanici).Scan(&yeniID)

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "id": yeniID, "db_kullanici": dbKullanici, "db_parola": parola,
	})
}

func sonKullaniciMi(toplamKullanici int) bool { return toplamKullanici <= 1 }

// demoAboneMi: domainin demo abonelik olup olmadığını döner. Hata durumunda
// yanıtı KENDİ yazar ve ok=false döner (çağıran hemen return etmeli).
// DatabaseIsimDegistir/DatabaseKullaniciEkle zaten is_demo'yu kendi domains
// sorgularında okuyor; yalnız db_accounts'a bakan kardeş handler'lar bu ayrı
// sorguyu kullanır.
func (h *Handlers) demoAboneMi(w http.ResponseWriter, r *http.Request, domainID int64) (demo bool, ok bool) {
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(), `SELECT is_demo FROM domains WHERE id=?`, domainID).Scan(&isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return false, false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return false, false
	}
	return isDemo == 1, true
}

// DatabaseKullaniciSil: DELETE /domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}
// DB'yi SİLMEZ — yalnız bu kullanıcının erişimini kaldırır. Son kullanıcıysa
// 409 döner (DB'yi silmek için domain sil ucu kullanılmalı).
func (h *Handlers) DatabaseKullaniciSil(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")
	dbid, err := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı kaydı id")
		return
	}

	if demoMu, ok := h.demoAboneMi(w, r, id); !ok {
		return
	} else if demoMu {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin kullanıcısı silinemez")
		return
	}

	var dbUser string
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT db_user FROM db_accounts WHERE id=? AND domain_id=? AND db_name=?`, dbid, id, dbAdi).Scan(&dbUser)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "kullanıcı kaydı bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}

	var toplam int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&toplam); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sayısı sorgu: "+err.Error())
		return
	}
	if sonKullaniciMi(toplam) {
		httpx.WriteError(w, http.StatusConflict, "bu veritabanının tek kullanıcısı — silmek için veritabanının kendisini silin")
		return
	}

	var baskaYerde int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE db_user=? AND db_name<>?`, dbUser, dbAdi).Scan(&baskaYerde); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı kullanım sorgu: "+err.Error())
		return
	}
	if err := hesaplar.MySQLRevokeUser(h.DB, dbAdi, dbUser, baskaYerde == 0); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı silme: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen_kullanici": dbUser})
}

// DatabaseYedekle: GET /domains/{id}/databases/{dbAdi}/yedek
// mysqldump çıktısını gzip'leyip indirme yanıtı olarak döner. Önce geçici bir
// dosyaya yazılır (backups.Indir deseniyle aynı) — mysqldump ortasında hata
// verirse yanıt başlamadan 500 dönebiliriz, yarım/bozuk dosya indirtmeyiz.
func (h *Handlers) DatabaseYedekle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")
	// exec'e giden HER identifier bundan geçmeli (plan: Global Constraints).
	if !hesaplar.GecerliDBKimlik(dbAdi) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}

	if demoMu, ok := h.demoAboneMi(w, r, id); !ok {
		return
	} else if demoMu {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin yedeği alınamaz")
		return
	}

	var varMi int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&varMi); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı sorgu: "+err.Error())
		return
	}
	if varMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	tmp, err := os.CreateTemp("", "sanal-db-yedek-*.sql.gz")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	gz := gzip.NewWriter(tmp)
	cmd := exec.CommandContext(r.Context(), "mysqldump",
		"--single-transaction", "--routines", "--triggers", "--events", dbAdi)
	cmd.Stdout = gz
	var stderr strings.Builder
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	closeErr := gz.Close()
	_ = tmp.Close()
	if runErr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
		return
	}
	if closeErr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, closeErr.Error())
		return
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	dosya := dbAdi + "-" + time.Now().UTC().Format("20060102-150405") + ".sql.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+dosya+`"`)
	if st != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	_, _ = io.Copy(w, f)
}

const maxDBRestoreBytes = 200 * 1024 * 1024 // 200 MB

// maxDBRestoreAcikBayt: gzip açıldıktan SONRAKİ akış için üst sınır (gzip bomb
// savunması). 200 MB'lık bir .gz teoride terabaytlarca veriye açılabilir; sınır
// cömert tutulur çünkü meşru bir dump'ın bu kadar büyümesi beklenmez.
const maxDBRestoreAcikBayt = 5 * 1024 * 1024 * 1024 // 5 GiB

// DatabaseGeriYukle: POST /domains/{id}/databases/{dbAdi}/geri-yukle (multipart, alan adı "dosya")
// .sql veya .sql.gz kabul eder; mevcut tabloları EZEBİLİR (frontend'de tehlikeli-onay zorunlu).
//
// 🔴 Dump hedef veritabanının KENDİ düşük yetkili kullanıcısıyla uygulanır
// (internal/sqlimport). Panel root çalıştığı için `mysql <db>` ile
// root@localhost'a akıtmak, dump'ın içine konacak `CREATE USER ... GRANT ALL`
// ya da `USE mysql; ...` ifadeleriyle DB sunucusunun tamamını müşteriye
// devretmek olurdu — `mysql <db>` yalnız VARSAYILAN şemayı seçer, yetki sınırı
// KOYMAZ (bkz. internal/sqlimport paket açıklaması ve internal/iceaktarim).
func (h *Handlers) DatabaseGeriYukle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")

	if demoMu, ok := h.demoAboneMi(w, r, id); !ok {
		return
	} else if demoMu {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe geri yükleme yapılamaz")
		return
	}

	// Hedefin kendi kimliği; no-rows aynı zamanda "bu DB bu domaine ait mi"
	// varlık denetimidir (iceaktarim.dbHedefi ile aynı sorgu).
	hedef, err := h.dbGeriYukleHedefi(r, id, dbAdi)
	if err != nil {
		httpx.WriteError(w, hedefDurumKodu(err), err.Error())
		return
	}

	// Tek tenant'ın paralel geri yüklemelerle sunucuyu tüketmesini engelle
	// (files.Upload / iceaktarim.SQLYukle deseni) — anahtar TENANT bazında
	// (dbAdi'ye göre değil), aksi halde çok-DB'li bir tenant her DB için ayrı
	// slot alıp genel sınırı aşabilirdi.
	birak, ok := httpx.YuklemeSlotVeyaHata(w, "db-restore:"+strconv.FormatInt(id, 10))
	if !ok {
		return
	}
	defer birak()

	httpx.ExtendDeadline(w, 15*time.Minute)
	httpx.ExtendBodyLimit(w, r, maxDBRestoreBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if httpx.GovdeSinirAsildi(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "yükleme boyutu sınırı aştı (max 200 MB)")
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
	if fh.Size > maxDBRestoreBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "dosya çok büyük (max 200 MB)")
		return
	}

	// Uzantıya DEĞİL, magic byte'a bakılır (iceaktarim.dumpAkisi ile aynı):
	// kullanıcı .sql adıyla gzip yükleyebilir, o zaman uzantı denetimi ham
	// ikili veriyi mysql'e akıtırdı.
	br := bufio.NewReaderSize(file, 64<<10)
	sihir, perr := br.Peek(2)
	if perr != nil && perr != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "dosya okunamadı: "+perr.Error())
		return
	}
	var reader io.Reader = br
	if len(sihir) == 2 && sihir[0] == 0x1f && sihir[1] == 0x8b {
		gzr, gerr := gzip.NewReader(br)
		if gerr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gzip açılamadı: "+gerr.Error())
			return
		}
		defer gzr.Close()
		reader = gzr
	}
	// Açılmış akışı sınırla: küçük ama kötü niyetli bir .gz sınırsız büyüyemesin.
	// sayan, gerçekte kaç bayt okunduğunu tutar — LimitReader sınıra TAKILIP
	// akışı sessizce kesebilir; bu durumda mysql'e giden ifade tam da bir
	// deyim sınırında kesilmiş olabilir ve "başarıyla" tamamlanabilir. Bu,
	// yarım kalmış bir geri yüklemeyi 200 OK gibi göstermek demektir —
	// okunan bayt sınırı aştıysa import başarılı görünse bile hata döneriz.
	sayan := &sayanOkuyucu{Reader: io.LimitReader(reader, maxDBRestoreAcikBayt+1)}

	if err := sqlimport.Uygula(r.Context(), hedef, sayan); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sayan.okunan > maxDBRestoreAcikBayt {
		httpx.WriteError(w, http.StatusBadRequest,
			"açılmış dosya boyut sınırını aştı, geri yükleme yarım kalmış olabilir — dosyayı küçültüp tekrar deneyin")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "sonuc": dbAdi + " geri yüklendi"})
}

// sayanOkuyucu: sarmaladığı Reader'dan gerçekte kaç bayt okunduğunu sayar.
type sayanOkuyucu struct {
	io.Reader
	okunan int64
}

func (s *sayanOkuyucu) Read(p []byte) (int, error) {
	n, err := s.Reader.Read(p)
	s.okunan += int64(n)
	return n, err
}

// errDBBulunamadi: hedef kimlik araması no-rows ile döndüğünde (varlık denetimi).
var errDBBulunamadi = errors.New("veritabanı bulunamadı")

func hedefDurumKodu(err error) int {
	if errors.Is(err, errDBBulunamadi) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// dbGeriYukleHedefi: hedef veritabanının BU domaine ait olduğunu doğrular ve o
// veritabanının kendi (düşük yetkili) kimlik bilgilerini döner.
// iceaktarim.dbHedefi ile aynı sorgu — dump root'a DEĞİL bu kimliğe akıtılır.
func (h *Handlers) dbGeriYukleHedefi(r *http.Request, domainID int64, dbAdi string) (sqlimport.Hedef, error) {
	var kullanici, sifreli, host string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db_user, db_pass_plain, COALESCE(db_host,'localhost')
		   FROM db_accounts WHERE domain_id=? AND db_name=? ORDER BY id LIMIT 1`, domainID, dbAdi).
		Scan(&kullanici, &sifreli, &host)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlimport.Hedef{}, errDBBulunamadi
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

// DatabaseOptimize: POST /domains/{id}/databases/{dbAdi}/optimize
func (h *Handlers) DatabaseOptimize(w http.ResponseWriter, r *http.Request) {
	h.mysqlcheckCalistir(w, r, "--optimize")
}

// DatabaseOnar: POST /domains/{id}/databases/{dbAdi}/onar
func (h *Handlers) DatabaseOnar(w http.ResponseWriter, r *http.Request) {
	h.mysqlcheckCalistir(w, r, "--repair")
}

// mysqlcheckIkiliAdi: MariaDB 11 (Debian trixie ve sonrası) paketleri
// `mysqlcheck` symlink'ini kaldırdı, araç artık `mariadb-check`. RHEL/MariaDB
// 10.x kurulumlarında `mysqlcheck` hâlâ var — hangisi varsa o kullanılır.
func mysqlcheckIkiliAdi() string {
	if _, err := exec.LookPath("mysqlcheck"); err == nil {
		return "mysqlcheck"
	}
	return "mariadb-check"
}

func (h *Handlers) mysqlcheckCalistir(w http.ResponseWriter, r *http.Request, bayrak string) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz domain id")
		return
	}
	dbAdi := chi.URLParam(r, "dbAdi")
	// exec'e giden HER identifier bundan geçmeli (plan: Global Constraints).
	if !hesaplar.GecerliDBKimlik(dbAdi) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}

	if demoMu, ok := h.demoAboneMi(w, r, id); !ok {
		return
	} else if demoMu {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin veritabanı optimize/onar edilemez")
		return
	}

	var varMi int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&varMi); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı sorgu: "+err.Error())
		return
	}
	if varMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	ikili := mysqlcheckIkiliAdi()
	out, err := exec.CommandContext(r.Context(), ikili, bayrak, dbAdi).CombinedOutput()
	if err != nil {
		// İkili hiç yoksa out boş kalır; mesaj GERÇEKTE çalışan ikilinin
		// adını taşır (mysqlcheck yerine mariadb-check çalışmış olabilir).
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		httpx.WriteError(w, http.StatusInternalServerError, ikili+": "+msg)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "sonuc": string(out)})
}
