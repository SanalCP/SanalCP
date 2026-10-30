package uyari

import (
	"testing"
	"time"

	"sanalcp/internal/monitor"
)

func TestSSLDurum(t *testing.T) {
	simdi := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	gun := func(n int) *time.Time { t := simdi.AddDate(0, 0, n); return &t }
	cases := []struct {
		ad       string
		aktif    bool
		kaynak   string
		bitis    *time.Time
		durum    Durum
		uygulanr bool
	}{
		{"30 gün", true, "letsencrypt", gun(30), "ok", true},
		{"14 gün", true, "letsencrypt", gun(14), "uyari", true},
		{"4 gün", true, "letsencrypt", gun(4), "uyari", true},
		{"3 gün", true, "letsencrypt", gun(3), "hata", true},
		{"süresi geçmiş", true, "letsencrypt", gun(-1), "hata", true},
		{"self-signed", true, "self-signed", gun(1), "", false},
		{"pasif", false, "letsencrypt", gun(1), "", false},
		{"bitiş yok", true, "letsencrypt", nil, "", false},
	}
	for _, c := range cases {
		d, ok := SSLDurum(c.aktif, c.kaynak, c.bitis, simdi)
		if ok != c.uygulanr || d != c.durum {
			t.Errorf("%s: (%q,%v) beklenen (%q,%v)", c.ad, d, ok, c.durum, c.uygulanr)
		}
	}
}

func TestWebIkiArdisikHatadaHata(t *testing.T) {
	erisilemez := monitor.DomainHealth{}
	kod := func(k int) monitor.DomainHealth { return monitor.DomainHealth{Erisilebilir: true, DurumKodu: k} }
	cases := []struct {
		h       monitor.DomainHealth
		onceki  int
		durum   Durum
		ardisik int
	}{
		{erisilemez, 0, "ok", 1},
		{erisilemez, 1, "hata", 2},
		{kod(200), 5, "ok", 0},
		{kod(503), 1, "hata", 2},
		{kod(404), 0, "ok", 0},
	}
	for i, c := range cases {
		d, a := WebDurum(c.h, c.onceki)
		if d != c.durum || a != c.ardisik {
			t.Errorf("#%d: (%q,%d) beklenen (%q,%d)", i, d, a, c.durum, c.ardisik)
		}
	}
}

func TestYedekDurum(t *testing.T) {
	cases := map[string]struct {
		durum Durum
		ok    bool
	}{
		"basarisiz":    {"hata", true},
		"dogrulandi":   {"ok", true},
		"dogrulaniyor": {"", false},
		"bekliyor":     {"", false},
		"":             {"", false},
	}
	for girdi, c := range cases {
		if d, ok := YedekDurum(girdi); d != c.durum || ok != c.ok {
			t.Errorf("%q: (%q,%v) beklenen (%q,%v)", girdi, d, ok, c.durum, c.ok)
		}
	}
}
