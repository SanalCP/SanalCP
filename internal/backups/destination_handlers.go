package backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/secretcrypt"
)

// GET /domains/{id}/backup-destination
// Parolayı gizleyerek döner (yalnız host/kullanıcı/durum gösterimi için).
func (h *Handlers) GetDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d, err := readDestination(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"yok": true})
		return
	}
	d.Parola = "" // gizle
	httpx.WriteJSON(w, http.StatusOK, d)
}

// PUT /domains/{id}/backup-destination
type destPutReq struct {
	Tip       string `json:"tip"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Kullanici string `json:"kullanici"`
	Parola    string `json:"parola"` // boş ise mevcut korunur
	UzakDizin string `json:"uzak_dizin"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`
	PathStyle bool   `json:"path_style"`
	Aktif     bool   `json:"aktif"`
}

func (h *Handlers) PutDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo abonelik için hedef tanımlanamaz")
		return
	}
	var req destPutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if !gecerliTip(req.Tip) {
		httpx.WriteError(w, http.StatusBadRequest, "tip: ftp|sftp|s3|b2")
		return
	}
	if req.Kullanici == "" {
		httpx.WriteError(w, http.StatusBadRequest, "kullanıcı / access key zorunlu")
		return
	}
	if objectStorageTip(req.Tip) {
		if req.Bucket == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bucket zorunlu")
			return
		}
		if req.Region == "" {
			req.Region = "us-east-1"
		}
		// Eski şemadaki NOT NULL host alanını anlamlı bir değerle doldur.
		req.Host = req.Endpoint
		probe := &Destination{
			Tip: req.Tip, Bucket: req.Bucket, Region: req.Region,
			Endpoint: req.Endpoint, PathStyle: req.PathStyle,
		}
		u, err := s3Endpoint(probe)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := hostGuvenliMi(r.Context(), u.Hostname()); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Port = 443
	} else if req.Host == "" {
		httpx.WriteError(w, http.StatusBadRequest, "host zorunlu")
		return
	} else if !gecerliHost(req.Host) {
		httpx.WriteError(w, http.StatusBadRequest, "host: geçersiz biçim (yalnız hostname/IP)")
		return
	} else if err := hostGuvenliMi(r.Context(), req.Host); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	} else if req.Port == 0 {
		if req.Tip == "sftp" {
			req.Port = 22
		} else {
			req.Port = 21
		}
	}
	if req.UzakDizin == "" {
		req.UzakDizin = "/"
	}
	// Parola boş gönderildi mi? Mevcut kaydı koru (zaten şifreliyse ciphertext'i
	// olduğu gibi taşı; eski düz-metin bir kayıtsa fırsattan istifade şifrele).
	var parolaToStore string
	if req.Parola != "" {
		enc, err := box.Encrypt(req.Parola)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "parola şifreleme: "+err.Error())
			return
		}
		parolaToStore = enc
	} else {
		var mevcutParola string
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COALESCE(parola,'') FROM backup_destinations WHERE domain_id=?`, id).Scan(&mevcutParola)
		if mevcutParola == "" {
			httpx.WriteError(w, http.StatusBadRequest, "parola zorunlu (yeni kayıt)")
			return
		}
		if secretcrypt.IsEncrypted(mevcutParola) {
			parolaToStore = mevcutParola
		} else {
			enc, err := box.Encrypt(mevcutParola)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "parola şifreleme: "+err.Error())
				return
			}
			parolaToStore = enc
		}
	}
	aktif := 0
	if req.Aktif {
		aktif = 1
	}
	_, err = h.DB.ExecContext(r.Context(),
		`INSERT INTO backup_destinations(
		   domain_id, tip, host, port, kullanici, parola, uzak_dizin,
		   bucket, region, endpoint, path_style, aktif)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   tip=VALUES(tip), host=VALUES(host), port=VALUES(port),
		   kullanici=VALUES(kullanici), parola=VALUES(parola),
		   uzak_dizin=VALUES(uzak_dizin), bucket=VALUES(bucket),
		   region=VALUES(region), endpoint=VALUES(endpoint),
		   path_style=VALUES(path_style), aktif=VALUES(aktif),
		   son_durum='', son_hata=''`,
		id, req.Tip, req.Host, req.Port, req.Kullanici, parolaToStore, req.UzakDizin,
		req.Bucket, req.Region, req.Endpoint, req.PathStyle, aktif)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB kayıt: "+err.Error())
		return
	}
	d, _ := readDestination(r.Context(), h.DB, id)
	if d != nil {
		d.Parola = "" // gizle
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

// DELETE /domains/{id}/backup-destination
func (h *Handlers) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM backup_destinations WHERE domain_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /domains/{id}/backup-destination/test
// Form: opsiyonel ad-hoc parametreler (form veya body) ile bağlantı testi.
// Body verilmezse mevcut DB kaydı kullanılır.
func (h *Handlers) TestDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var d *Destination
	var ad destPutReq
	if json.NewDecoder(r.Body).Decode(&ad) == nil &&
		(ad.Host != "" || (objectStorageTip(ad.Tip) && ad.Bucket != "")) {
		if ad.Host != "" && !objectStorageTip(ad.Tip) && !gecerliHost(ad.Host) {
			httpx.WriteError(w, http.StatusBadRequest, "host: geçersiz biçim (yalnız hostname/IP)")
			return
		}
		// Ad-hoc test (UI'dan kaydetmeden test): parola boşsa DB'den çek
		mevcutParola := ""
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COALESCE(parola,'') FROM backup_destinations WHERE domain_id=?`, id).Scan(&mevcutParola)
		if ad.Parola == "" {
			if secretcrypt.IsEncrypted(mevcutParola) {
				pt, err := box.Decrypt(mevcutParola)
				if err != nil {
					httpx.WriteError(w, http.StatusInternalServerError, "parola çözülemedi: "+err.Error())
					return
				}
				ad.Parola = pt
			} else {
				ad.Parola = mevcutParola
			}
		}
		port := ad.Port
		if port == 0 {
			if ad.Tip == "sftp" {
				port = 22
			} else {
				port = 21
			}
		}
		dz := ad.UzakDizin
		if dz == "" {
			dz = "/"
		}
		d = &Destination{
			DomainID: id, Tip: ad.Tip, Host: ad.Host, Port: port,
			Kullanici: ad.Kullanici, Parola: ad.Parola, UzakDizin: dz, Aktif: true,
			Bucket: ad.Bucket, Region: ad.Region, Endpoint: ad.Endpoint,
			PathStyle: ad.PathStyle,
		}
	} else {
		d, err = readDestination(r.Context(), h.DB, id)
		if err != nil || d == nil {
			httpx.WriteError(w, http.StatusBadRequest, "kayıt yok veya body geçersiz")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := testConnection(ctx, h.DB, d); err != nil {
		short := err.Error()
		if len(short) > 400 {
			short = short[:400]
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":   false,
			"hata": short,
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
