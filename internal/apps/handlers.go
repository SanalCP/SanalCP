package apps

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

// kurulumKilit: eşzamanlı kurulumları hedef-yola göre serileştirir (çift-tık koruması).
// Değer önemsiz; anahtar = mutlak hedef dizin.
var kurulumKilit sync.Map

func scheme(ssl bool) string {
	if ssl {
		return "https://"
	}
	return "http://"
}

func (h *Handlers) domain(r *http.Request) (id int64, sk, alanAdi string, ssl, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var cert string
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, alan_adi, COALESCE(cert_path,''), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&sk, &alanAdi, &cert, &isDemo); err != nil {
		return id, "", "", false, false, false
	}
	return id, sk, alanAdi, cert != "", isDemo == 1, true
}

// alanlariDogrula: FormAlanlari şemasına göre girdiyi doğrular + kırpar. Hata boşsa geçerli.
func alanlariDogrula(alanlar []FormAlan, girdi map[string]string) (map[string]string, string) {
	temiz := map[string]string{}
	for _, fa := range alanlar {
		v := strings.TrimSpace(girdi[fa.Anahtar])
		temiz[fa.Anahtar] = v
		if fa.Zorunlu && v == "" {
			return nil, fa.Etiket + " gerekli"
		}
		if fa.Tur == "email" && v != "" && !reEmail.MatchString(v) {
			return nil, "geçersiz e-posta: " + fa.Etiket
		}
	}
	return temiz, ""
}

// POST /domains/{id}/apps/{tur}/kur
func (h *Handlers) Kur(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	id, sk, alanAdi, ssl, demo, ok := h.domain(r)
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
		AltDizin string            `json:"alt_dizin"`
		Alanlar  map[string]string `json:"alanlar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	req.AltDizin = strings.Trim(strings.TrimSpace(req.AltDizin), "/")
	if req.AltDizin != "" && !reAltDizin.MatchString(req.AltDizin) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz alt dizin (küçük harf/rakam/-)")
		return
	}
	temizAlanlar, hataMsg := alanlariDogrula(u.FormAlanlari(), req.Alanlar)
	if hataMsg != "" {
		httpx.WriteError(w, http.StatusBadRequest, hataMsg)
		return
	}

	root := "/home/" + sk + "/public_html"
	hedef := root
	if req.AltDizin != "" {
		hedef = filepath.Join(root, req.AltDizin)
	}
	if _, sur := kurulumKilit.LoadOrStore(hedef, struct{}{}); sur {
		httpx.WriteError(w, http.StatusConflict, "bu dizine kurulum zaten sürüyor — lütfen bekleyin")
		return
	}
	defer kurulumKilit.Delete(hedef)
	if msg, kurulu := zatenKuruluMu(hedef, u.MarkerDosya()); kurulu {
		httpx.WriteError(w, http.StatusConflict, msg)
		return
	}
	if err := os.MkdirAll(hedef, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hedef dizin oluşturulamadı")
		return
	}
	_ = exec.Command("chown", "-R", sk+":"+sk, hedef).Run()
	_ = exec.Command("restorecon", "-R", hedef).Run()

	slug := randSlug()
	dbOnEk := u.DBOnEki()
	dbName := dbOnEk + "_" + slug
	dbUser := dbOnEk + "u_" + slug
	dbPass := hesaplar.RandomParola(24)
	if err := hesaplar.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı oluşturulamadı: "+err.Error())
		return
	}
	temizle := func(asama string, err error) {
		_ = hesaplar.MySQLDropDB(h.DB, dbName, dbUser)
		if req.AltDizin != "" {
			_ = os.RemoveAll(hedef)
		}
		msg := err.Error()
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		httpx.WriteError(w, http.StatusInternalServerError, asama+" başarısız: "+msg)
	}

	url := scheme(ssl) + alanAdi
	if req.AltDizin != "" {
		url += "/" + req.AltDizin
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	sonuc, err := u.Kur(ctx, KurulumIstek{
		DomainID: id, SK: sk, AlanAdi: alanAdi, SSL: ssl,
		Hedef: hedef, URL: url,
		DBAdi: dbName, DBKullanici: dbUser, DBParola: dbPass,
		Alanlar: temizAlanlar,
	})
	if err != nil {
		temizle(u.Ad()+" kurulumu", err)
		return
	}
	_ = exec.Command("chown", "-R", sk+":"+sk, hedef).Run()
	_ = exec.Command("restorecon", "-R", hedef).Run()

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tur": u.Slug(),
		"site_url": sonuc.SiteURL, "admin_url": sonuc.AdminURL,
		"admin_kullanici": sonuc.AdminKullanici, "admin_parola": sonuc.AdminParola,
		"surum": sonuc.Surum, "db_adi": dbName, "ekstra": sonuc.Ekstra,
	})
}

// DELETE /domains/{id}/apps/{tur}  {dizin, db_sil}
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	domID, sk, _, _, demo, ok := h.domain(r)
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
	var sreq struct {
		Dizin string `json:"dizin"`
		DBSil bool   `json:"db_sil"`
	}
	_ = json.NewDecoder(r.Body).Decode(&sreq)
	dir, err := cozDizin(sk, sreq.Dizin, u.MarkerDosya())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	root := "/home/" + sk + "/public_html"
	if dir == root {
		httpx.WriteError(w, http.StatusBadRequest, "kök dizindeki kurulum panelden silinemez (tüm site gider); Dosya Yöneticisi'nden kaldırın")
		return
	}
	if sreq.DBSil {
		if dbName, bulundu := u.DBAdiOku(dir); bulundu {
			if ok, err := h.dbSahipMi(r.Context(), dbName, domID); err == nil && ok {
				// h.DB panel bağlantısı yalnız GRANT ALL ON panel.* yetkisine sahip —
				// gerçek DROP DATABASE yetkisi yalnız hesaplar paketinin root
				// bağlantısında (rootExecAll). hesaplar.MySQLDropDBKeepUser bunu doğru
				// yapar + db_accounts satırını temizler (kullanıcıya dokunmaz — "mevcut
				// kullanıcı" modunda aynı kullanıcı başka DB'de de olabilir; bu ihtimal
				// Kur akışında tek DB'ye tek özel kullanıcı oluşturulduğu için bu türde
				// pratikte oluşmaz, ama fonksiyon davranışı bilinçli olarak muhafazakâr).
				_ = hesaplar.MySQLDropDBKeepUser(h.DB, dbName)
			}
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// dbSahipMi: dbName GERÇEKTEN bu domain'e ait mi? (db_accounts sahiplik kontrolü)
func (h *Handlers) dbSahipMi(ctx context.Context, dbName string, domainID int64) (bool, error) {
	var n int
	err := h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_accounts WHERE db_name=? AND domain_id=?`, dbName, domainID).Scan(&n)
	return n > 0, err
}

// POST /domains/{id}/apps/{tur}/guncelle  {dizin}
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	if !u.GuncelleDesteklenir() {
		httpx.WriteError(w, http.StatusBadRequest, "bu uygulama için güncelleme desteklenmiyor")
		return
	}
	_, sk, _, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var greq struct {
		Dizin string `json:"dizin"`
	}
	_ = json.NewDecoder(r.Body).Decode(&greq)
	dir, err := cozDizin(sk, greq.Dizin, u.MarkerDosya())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := u.Guncelle(ctx, sk, dir); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncelleme: "+err.Error())
		return
	}
	bilgi, _ := u.Bilgi(ctx, sk, dir, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "surum": bilgi.Surum})
}
