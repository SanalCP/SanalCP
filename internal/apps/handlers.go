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
	"sanalcp/internal/middleware"

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

// GET /domains/{id}/apps
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	_, sk, alanAdi, ssl, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	root := "/home/" + sk + "/public_html"
	dizinler := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dizinler = append(dizinler, filepath.Join(root, e.Name()))
			}
		}
	}
	type satir struct {
		Tur string `json:"tur"`
		Ad  string `json:"ad"`
		Kurulum
	}
	out := []satir{}
	for _, u := range Hepsi() {
		for _, dir := range dizinler {
			if _, err := os.Stat(filepath.Join(dir, u.MarkerDosya())); err != nil {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(dir, root), "/")
			url := scheme(ssl) + alanAdi
			if rel != "" {
				url += "/" + rel
			}
			k, err := u.Bilgi(ctx, sk, dir, url)
			if err != nil {
				continue
			}
			k.Dizin = "/" + rel
			if k.Dizin == "/" {
				k.Dizin = "/ (kök)"
			}
			out = append(out, satir{Tur: u.Slug(), Ad: u.Ad(), Kurulum: k})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// GET /domains/{id}/apps/turler
func (h *Handlers) Turler(w http.ResponseWriter, r *http.Request) {
	type turBilgi struct {
		Slug    string     `json:"slug"`
		Ad      string     `json:"ad"`
		Alanlar []FormAlan `json:"form_alanlari"`
	}
	out := []turBilgi{}
	for _, u := range Hepsi() {
		out = append(out, turBilgi{Slug: u.Slug(), Ad: u.Ad(), Alanlar: u.FormAlanlari()})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// TumKurulum: tüm domainlerdeki tek bir kurulumun özeti (aggregate tablo satırı).
type TumKurulum struct {
	DomainID      int64  `json:"domain_id"`
	AlanAdi       string `json:"alan_adi"`
	Tur           string `json:"tur"`
	TurAdi        string `json:"tur_adi"`
	Dizin         string `json:"dizin"`
	Surum         string `json:"surum"`
	SonSurum      string `json:"son_surum"`
	Durum         string `json:"durum"`
	KurulumTarihi string `json:"kurulum_tarihi"`
	SiteURL       string `json:"site_url"`
	AdminURL      string `json:"admin_url"`
}

type aday struct {
	domainID    int64
	sk, alanAdi string
	ssl         bool
	u           Uygulama
	dir, root   string
}

// GET /apps/tumu — TÜM domainlerdeki kurulu uygulamaları tarar. BayiVeUstu.
func (h *Handlers) TumListe(w http.ResponseWriter, r *http.Request) {
	kosul, arg := middleware.KapsamSQL(r, "d")
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT d.id, d.sistem_kullanici, d.alan_adi, COALESCE(d.cert_path,'') FROM domains d`+kosul+` ORDER BY d.alan_adi`, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domainler listelenemedi")
		return
	}
	var adaylar []aday
	for rows.Next() {
		var id int64
		var sk, alanAdi, cert string
		if err := rows.Scan(&id, &sk, &alanAdi, &cert); err != nil {
			continue
		}
		if !adlar.SKGecerli(sk) {
			continue
		}
		root := "/home/" + sk + "/public_html"
		dizinler := []string{root}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					dizinler = append(dizinler, filepath.Join(root, e.Name()))
				}
			}
		}
		for _, u := range Hepsi() {
			for _, dir := range dizinler {
				if _, err := os.Stat(filepath.Join(dir, u.MarkerDosya())); err != nil {
					continue
				}
				adaylar = append(adaylar, aday{id, sk, alanAdi, cert != "", u, dir, root})
			}
		}
	}
	_ = rows.Err()
	rows.Close()

	out := make([]TumKurulum, len(adaylar))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range adaylar {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, a aday) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = h.incele(r.Context(), a)
		}(i, adaylar[i])
	}
	wg.Wait()
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) incele(ctx context.Context, a aday) TumKurulum {
	rel := strings.TrimPrefix(strings.TrimPrefix(a.dir, a.root), "/")
	dizinEt := "/" + rel
	if dizinEt == "/" {
		dizinEt = "/ (kök)"
	}
	tk := TumKurulum{DomainID: a.domainID, AlanAdi: a.alanAdi, Tur: a.u.Slug(), TurAdi: a.u.Ad(), Dizin: dizinEt, Durum: "bilinmiyor"}
	url := scheme(a.ssl) + a.alanAdi
	if rel != "" {
		url += "/" + rel
	}
	k, err := a.u.Bilgi(ctx, a.sk, a.dir, url)
	if err != nil {
		return tk
	}
	tk.Surum, tk.SonSurum, tk.Durum, tk.KurulumTarihi, tk.SiteURL, tk.AdminURL =
		k.Surum, k.SonSurum, k.Durum, k.KurulumTarihi, k.SiteURL, k.AdminURL
	if tk.Durum == "" {
		tk.Durum = "bilinmiyor"
	}
	return tk
}
