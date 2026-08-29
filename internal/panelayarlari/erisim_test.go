package panelayarlari

import (
	"encoding/json"
	"net"
	"testing"
)

// Boş liste JSON'da "null" değil "[]" olmalı: panel arayüzü cidrler/ip_istisnalari
// alanlarında doğrudan .join() çağırıyor ve null gelirse sayfa render sırasında
// "Cannot read properties of null (reading 'join')" ile çöküyordu.
func TestSatirlar_BosGirdideNullDegilBosDiziUretir(t *testing.T) {
	for _, ham := range []string{"", "   ", "\n\n", " , , "} {
		sonuc := satirlar(ham)
		if sonuc == nil {
			t.Errorf("satirlar(%q) nil döndü, boş dilim bekleniyordu", ham)
		}
		if len(sonuc) != 0 {
			t.Errorf("satirlar(%q)=%v, boş bekleniyordu", ham, sonuc)
		}
		b, err := json.Marshal(map[string]any{"cidrler": sonuc})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(b), `{"cidrler":[]}`; got != want {
			t.Errorf("satirlar(%q) JSON=%s, want %s", ham, got, want)
		}
	}
}

func TestNormalizeCIDRler_BosGirdideNullDegilBosDiziUretir(t *testing.T) {
	norm, _, err := normalizeCIDRler([]string{""})
	if err != nil {
		t.Fatal(err)
	}
	if norm == nil {
		t.Fatal("normalizeCIDRler nil döndü, boş dilim bekleniyordu")
	}
	b, _ := json.Marshal(map[string]any{"cidrler": norm})
	if got, want := string(b), `{"cidrler":[]}`; got != want {
		t.Errorf("JSON=%s, want %s", got, want)
	}
}

func TestNormalizeCIDRler_IPleriTekAdresAginaCevirir(t *testing.T) {
	norm, aglar, err := normalizeCIDRler([]string{"203.0.113.7", "2001:db8::1", "192.0.2.99/24", "203.0.113.7"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"203.0.113.7/32", "2001:db8::1/128", "192.0.2.0/24"}
	if len(norm) != len(want) {
		t.Fatalf("norm=%v", norm)
	}
	for i := range want {
		if norm[i] != want[i] {
			t.Errorf("norm[%d]=%q, want %q", i, norm[i], want[i])
		}
	}
	if !ipIzinli(net.ParseIP("192.0.2.42"), aglar) {
		t.Error("CIDR içindeki IP reddedildi")
	}
}

func TestNormalizeCIDRler_GecersizDegeriReddeder(t *testing.T) {
	if _, _, err := normalizeCIDRler([]string{"her-yer"}); err == nil {
		t.Fatal("geçersiz değer kabul edildi")
	}
}
