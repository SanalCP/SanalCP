package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"sanalcp/internal/httpx"
)

var timedatectlCalistir = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "timedatectl", args...).CombinedOutput()
}

type saatDurum struct {
	SaatDilimi      string   `json:"saat_dilimi"`
	YerelSaat       string   `json:"yerel_saat"`
	UTCSaat         string   `json:"utc_saat"`
	NTPAktif        bool     `json:"ntp_aktif"`
	NTPSenkron      bool     `json:"ntp_senkron"`
	DonanimSaatiUTC bool     `json:"donanim_saati_utc"`
	SaatDilimleri   []string `json:"saat_dilimleri,omitempty"`
}

func timedatectlAlanlari(ctx context.Context) (map[string]string, error) {
	b, err := timedatectlCalistir(ctx, "show", "--property=Timezone", "--property=NTP", "--property=NTPSynchronized", "--property=LocalRTC")
	if err != nil {
		return nil, fmt.Errorf("timedatectl: %s", strings.TrimSpace(string(b)))
	}
	m := map[string]string{}
	for _, satir := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(satir, "=")
		if ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if m["Timezone"] == "" {
		return nil, errors.New("saat dilimi okunamadı")
	}
	return m, nil
}

func saatDilimiListesi(ctx context.Context) ([]string, error) {
	b, err := timedatectlCalistir(ctx, "list-timezones", "--no-pager")
	if err != nil {
		return nil, fmt.Errorf("saat dilimleri okunamadı: %s", strings.TrimSpace(string(b)))
	}
	var out []string
	for _, s := range strings.Fields(string(b)) {
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, nil
}

func saatDilimiGecerli(zones []string, timezone string) bool {
	i := sort.SearchStrings(zones, timezone)
	return i < len(zones) && zones[i] == timezone
}

func saatDurumuOku(ctx context.Context, liste bool) (saatDurum, error) {
	alan, err := timedatectlAlanlari(ctx)
	if err != nil {
		return saatDurum{}, err
	}
	z := alan["Timezone"]
	loc, err := time.LoadLocation(z)
	if err != nil {
		return saatDurum{}, fmt.Errorf("saat dilimi yüklenemedi: %w", err)
	}
	simdi := time.Now()
	d := saatDurum{SaatDilimi: z, YerelSaat: simdi.In(loc).Format(time.RFC3339), UTCSaat: simdi.UTC().Format(time.RFC3339),
		NTPAktif: alan["NTP"] == "yes", NTPSenkron: alan["NTPSynchronized"] == "yes", DonanimSaatiUTC: alan["LocalRTC"] != "yes"}
	if liste {
		d.SaatDilimleri, err = saatDilimiListesi(ctx)
	}
	return d, err
}

// SaatDurum — GET /system/saat. Router'da AdminOnly ile korunur.
func SaatDurum(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	d, err := saatDurumuOku(ctx, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

// SaatKaydet — PUT /system/saat {saat_dilimi, ntp_aktif}.
func SaatKaydet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req struct {
		SaatDilimi string `json:"saat_dilimi"`
		NTPAktif   bool   `json:"ntp_aktif"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	req.SaatDilimi = strings.TrimSpace(req.SaatDilimi)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	zones, err := saatDilimiListesi(ctx)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !saatDilimiGecerli(zones, req.SaatDilimi) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz saat dilimi")
		return
	}
	eski, err := saatDurumuOku(ctx, false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b, err := timedatectlCalistir(ctx, "set-timezone", req.SaatDilimi); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "saat dilimi uygulanamadı: "+strings.TrimSpace(string(b)))
		return
	}
	ntp := "false"
	if req.NTPAktif {
		ntp = "true"
	}
	if b, err := timedatectlCalistir(ctx, "set-ntp", ntp); err != nil {
		geriCtx, geriIptal := context.WithTimeout(context.Background(), 5*time.Second)
		defer geriIptal()
		_, _ = timedatectlCalistir(geriCtx, "set-timezone", eski.SaatDilimi)
		httpx.WriteError(w, http.StatusInternalServerError, "NTP ayarı uygulanamadı: "+strings.TrimSpace(string(b)))
		return
	}
	d, err := saatDurumuOku(ctx, false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}
