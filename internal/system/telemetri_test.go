package system

import (
	"os"
	"strings"
	"testing"
)

func TestIpBicimGecerliMi(t *testing.T) {
	vakalar := []struct {
		girdi   string
		gecerli bool
	}{
		{"1.2.3.4", true},
		{"2001:db8::1", true},
		{"", false},
		{"<html>hata</html>", false},
		{"1.2.3.4\n5.6.7.8", false},
	}
	for _, v := range vakalar {
		if got := ipBicimGecerliMi(v.girdi); got != v.gecerli {
			t.Errorf("ipBicimGecerliMi(%q) = %v, beklenen %v", v.girdi, got, v.gecerli)
		}
	}
}

func TestTelemetriHazirMi(t *testing.T) {
	if telemetriHazirMi("", "") {
		t.Error("proje ve anahtar boşken hazır olmamalı")
	}
	if telemetriHazirMi("proje", "") {
		t.Error("anahtar boşken hazır olmamalı")
	}
	if telemetriHazirMi("", "anahtar") {
		t.Error("proje boşken hazır olmamalı")
	}
	if !telemetriHazirMi("proje", "anahtar") {
		t.Error("ikisi de doluyken hazır olmalı")
	}
}

func TestFirestoreDegerleriOlustur(t *testing.T) {
	d := firestoreDegerleriOlustur("kimlik123", "0.9.7", "1.2.3.4", "almalinux", "tr", "2026-01-01T00:00:00Z")
	alanlar, ok := d["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields alanı map[string]any değil: %#v", d)
	}
	kimlik, ok := alanlar["kurulum_kimlik"].(map[string]any)
	if !ok || kimlik["stringValue"] != "kimlik123" {
		t.Errorf("kurulum_kimlik = %#v, beklenen stringValue=kimlik123", alanlar["kurulum_kimlik"])
	}
	ilkZaman, ok := alanlar["ilk_kurulum_zamani"].(map[string]any)
	if !ok || ilkZaman["timestampValue"] != "2026-01-01T00:00:00Z" {
		t.Errorf("ilk_kurulum_zamani = %#v, beklenen timestampValue=2026-01-01T00:00:00Z", alanlar["ilk_kurulum_zamani"])
	}
	if _, ok := alanlar["son_gorulme_zamani"].(map[string]any); !ok {
		t.Error("son_gorulme_zamani alanı yok")
	}
}

func TestFirestoreURL(t *testing.T) {
	old := os.Getenv("PANEL_FIREBASE_PROJE")
	os.Setenv("PANEL_FIREBASE_PROJE", "test-proje")
	defer os.Setenv("PANEL_FIREBASE_PROJE", old)

	u := firestoreURL("kimlik123")
	if !strings.Contains(u, "/projects/test-proje/databases/(default)/documents/kurulumlar/kimlik123") {
		t.Errorf("URL yol beklenmedik: %s", u)
	}
	for _, alan := range telemetriAlanlar {
		if !strings.Contains(u, "updateMask.fieldPaths="+alan) {
			t.Errorf("URL, %q için updateMask içermiyor: %s", alan, u)
		}
	}
}
