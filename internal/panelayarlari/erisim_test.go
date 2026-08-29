package panelayarlari

import (
	"net"
	"testing"
)

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
