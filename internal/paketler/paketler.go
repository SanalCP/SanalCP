// Package paketler: sunucu paket yoneticisi (DNF wrapper)
package paketler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/osfam"
)

// Kritik paketler — kaldirilirsa sistem coker
var KORUNAN = map[string]bool{
	"bash": true, "glibc": true, "kernel": true, "systemd": true,
	"openssh": true, "openssh-server": true, "openssh-clients": true,
	"sudo": true, "dnf": true, "rpm": true, "filesystem": true,
	"setup": true, "selinux-policy": true, "selinux-policy-targeted": true,
	"libselinux": true, "policycoreutils": true,
	// Panel'in calismasi icin gerekli
	"nginx": true, "mariadb": true, "mariadb-server": true, "mariadb-common": true,
	"bind": true, "bind-utils": true,
	"pure-ftpd": true, "pure-ftpd-mysql": true,
	"php": true, "php-fpm": true, "php-cli": true, "php-common": true,
}

type Handlers struct {
	DB *sql.DB
}

// Paket icin guvenli ad
func safe(s string) bool {
	if s == "" || len(s) > 80 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '+') {
			return false
		}
	}
	return true
}

type Paket struct {
	Adi      string `json:"adi"`
	Surum    string `json:"surum,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Aciklama string `json:"aciklama,omitempty"`
	Kurulu   bool   `json:"kurulu"`
	Korunan  bool   `json:"korunan"`
}

// Ara: dnf search ile arama (max 200 sonuc)
func (h *Handlers) Ara(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "q parametresi gerekli")
		return
	}
	if !safe(q) {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz arama")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// Paket arama: dnf search / apt-cache search. Iki cikti bicimi de
	// "<ad> : <aciklama>" veya "<ad> - <aciklama>" satirlari uretir; asagidaki
	// ayristirici ikisini de tanir.
	b := osfam.Mevcut()
	out, _ := b.Komut(ctx, b.AraArgs(q)).CombinedOutput()
	lines := strings.Split(string(out), "\n")
	paketler := []Paket{}
	kuruluSet := installedSet()
	for _, ln := range lines {
		name, desc, ok := osfam.AramaSatiriAyristir(ln)
		if !ok {
			continue
		}
		paketler = append(paketler, Paket{
			Adi:      name,
			Aciklama: desc,
			Kurulu:   kuruluSet[name],
			Korunan:  KORUNAN[name],
		})
		if len(paketler) >= 200 {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"q":      q,
		"toplam": len(paketler),
		"icerik": paketler,
	})
}

// installedSet: tüm kurulu paket adlarini set olarak don
func installedSet() map[string]bool {
	b := osfam.Mevcut()
	out, _ := b.Komut(context.Background(), b.KuruluListeArgs()).CombinedOutput()
	set := make(map[string]bool, 600)
	for _, ad := range osfam.KuruluListeAyristir(string(out)) {
		set[ad] = true
	}
	return set
}

// Kurulu: tüm kurulu paketleri opsiyonel filtre ile listele
func (h *Handlers) Kurulu(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	b := osfam.Mevcut()
	out, _ := b.Komut(r.Context(), b.KuruluDetayArgs()).CombinedOutput()
	paketler := []Paket{}
	for _, satir := range osfam.KuruluListeAyristir(string(out)) {
		parts := strings.SplitN(satir, "|", 3)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		if q != "" && !strings.Contains(strings.ToLower(name), q) && !strings.Contains(strings.ToLower(parts[2]), q) {
			continue
		}
		paketler = append(paketler, Paket{
			Adi:      name,
			Surum:    parts[1],
			Aciklama: parts[2],
			Kurulu:   true,
			Korunan:  KORUNAN[name],
		})
		if len(paketler) >= 500 {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"toplam": len(paketler),
		"icerik": paketler,
	})
}

type opReq struct {
	Paket string `json:"paket"`
}

// Kur: dnf install
func (h *Handlers) Kur(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz govde")
		return
	}
	if !safe(req.Paket) {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz paket adi")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	out, err := osfam.PaketKur(ctx, req.Paket)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"paket kurulamadi: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"paket":  req.Paket,
		"output": string(out),
	})
}

// Kaldir: dnf remove (korumalı paketler reddedilir)
func (h *Handlers) Kaldir(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz govde")
		return
	}
	if !safe(req.Paket) {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz paket adi")
		return
	}
	if KORUNAN[req.Paket] {
		httpx.WriteError(w, http.StatusForbidden,
			"bu paket sistem icin kritik, kaldirilamaz: "+req.Paket)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	out, err := osfam.PaketSil(ctx, req.Paket)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"paket kaldirilamadi: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"paket":  req.Paket,
		"output": string(out),
	})
}

// Guncelle: dnf upgrade
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz govde")
		return
	}
	if req.Paket != "" && !safe(req.Paket) {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz paket")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	b := osfam.Mevcut()
	out, err := b.Komut(ctx, b.YukseltArgs(req.Paket)).CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"paket yukseltilemedi: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"paket":  req.Paket,
		"output": string(out),
	})
}

// Bilgi: dnf info <paket>
func (h *Handlers) Bilgi(w http.ResponseWriter, r *http.Request) {
	ad := r.URL.Query().Get("ad")
	if !safe(ad) {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz ad")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	b := osfam.Mevcut()
	out, _ := b.Komut(ctx, b.BilgiArgs(ad)).CombinedOutput()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ad":     ad,
		"output": string(out),
	})
}

// Durum: virgüllü paket adlari listesi için kurulu durumunu döner
func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	adlarStr := r.URL.Query().Get("adlar")
	if adlarStr == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{})
		return
	}
	set := installedSet()
	res := make(map[string]bool)
	for _, ad := range strings.Split(adlarStr, ",") {
		ad = strings.TrimSpace(ad)
		if ad == "" || !safe(ad) {
			continue
		}
		res[ad] = set[ad]
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
