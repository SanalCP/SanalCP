package system

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/httpx"
)

var (
	hostnameOku      = os.Hostname
	hostnameDosyasi  = "/etc/hostname"
	cloudInitDosyasi = "/etc/cloud/cloud.cfg.d/99-sanalcp-hostname.cfg"
	nmDosyasi        = "/etc/NetworkManager/conf.d/99-sanalcp-hostname.conf"
	hostnameAyarla   = func(ctx context.Context, ad string) error {
		return exec.CommandContext(ctx, "hostnamectl", "set-hostname", ad).Run()
	}
)

type hostnameDurum struct {
	Hostname string `json:"hostname"`
	Korumali bool   `json:"korumali"`
	Aciklama string `json:"aciklama"`
}

func hostnameDogrula(raw string) (string, error) {
	ad := strings.TrimSpace(raw)
	ad = strings.ToLower(strings.TrimSuffix(ad, "."))
	if len(ad) == 0 || len(ad) > 253 {
		return "", errors.New("hostname 1–253 karakter olmalıdır")
	}
	if net.ParseIP(ad) != nil {
		return "", errors.New("hostname IP adresi olamaz")
	}
	if ad == "localhost" || strings.HasSuffix(ad, ".localhost") {
		return "", errors.New("localhost hostname olarak kullanılamaz")
	}
	for _, etiket := range strings.Split(ad, ".") {
		if len(etiket) == 0 || len(etiket) > 63 ||
			etiket[0] == '-' || etiket[len(etiket)-1] == '-' {
			return "", errors.New("geçersiz hostname etiketi")
		}
		for _, r := range etiket {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", errors.New("hostname yalnızca harf, rakam, tire ve nokta içerebilir")
			}
		}
	}
	return ad, nil
}

func atomikYaz(yol, icerik string, mod os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(yol), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(yol), ".sanalcp-hostname-*")
	if err != nil {
		return err
	}
	gecici := f.Name()
	defer os.Remove(gecici)
	if err = f.Chmod(mod); err == nil {
		_, err = f.WriteString(icerik)
	}
	if err == nil {
		err = f.Sync()
	}
	if kapatErr := f.Close(); err == nil {
		err = kapatErr
	}
	if err != nil {
		return err
	}
	return os.Rename(gecici, yol)
}

func hostnameKorumasiYaz() error {
	if err := atomikYaz(cloudInitDosyasi,
		"# SanalCP: sağlayıcı/cloud-init hostname'i geri yazmasın\npreserve_hostname: true\n", 0644); err != nil {
		return err
	}
	return atomikYaz(nmDosyasi,
		"# SanalCP: DHCP/NetworkManager statik hostname'i değiştirmesin\n[main]\nhostname-mode=none\n", 0644)
}

// HostnameDurum — GET /system/hostname.
func HostnameDurum(w http.ResponseWriter, _ *http.Request) {
	ad, err := hostnameOku()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hostname okunamadı")
		return
	}
	_, cloudErr := os.Stat(cloudInitDosyasi)
	_, nmErr := os.Stat(nmDosyasi)
	httpx.WriteJSON(w, http.StatusOK, hostnameDurum{
		Hostname: ad,
		Korumali: cloudErr == nil && nmErr == nil,
		Aciklama: "cloud-init ve DHCP/NetworkManager hostname değişiklikleri engellenir",
	})
}

// HostnameKaydet — PUT /system/hostname. Router'da AdminOnly ile korunur.
func HostnameKaydet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req struct {
		Hostname string `json:"hostname"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	ad, err := hostnameDogrula(req.Hostname)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := hostnameKorumasiYaz(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hostname kalıcılık koruması yazılamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := hostnameAyarla(ctx, ad); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hostname uygulanamadı")
		return
	}
	// hostnamectl zaten /etc/hostname'i yazar. Minimal/container ortamlarda da
	// kalıcılığı garanti etmek için sonucu ayrıca atomik biçimde sabitle.
	if err := atomikYaz(hostnameDosyasi, ad+"\n", 0644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hostname uygulandı ancak /etc/hostname yazılamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, hostnameDurum{
		Hostname: ad,
		Korumali: true,
		Aciklama: "hostname kalıcı olarak değiştirildi; cloud-init ve DHCP geri yazamaz",
	})
}
