package system

import "testing"

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
