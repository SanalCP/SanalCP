package system

import (
	"context"
	"reflect"
	"testing"
)

func TestTimedatectlAlanlari(t *testing.T) {
	eski := timedatectlCalistir
	defer func() { timedatectlCalistir = eski }()
	timedatectlCalistir = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("Timezone=Europe/Istanbul\nNTP=yes\nNTPSynchronized=yes\nLocalRTC=no\n"), nil
	}
	m, err := timedatectlAlanlari(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m["Timezone"] != "Europe/Istanbul" || m["NTP"] != "yes" || m["LocalRTC"] != "no" {
		t.Fatalf("alanlar yanlış: %#v", m)
	}
}

func TestSaatDilimiListesiVeDogrulama(t *testing.T) {
	eski := timedatectlCalistir
	defer func() { timedatectlCalistir = eski }()
	timedatectlCalistir = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("UTC\nEurope/London\nEurope/Istanbul\n"), nil
	}
	z, err := saatDilimiListesi(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Europe/Istanbul", "Europe/London", "UTC"}
	if !reflect.DeepEqual(z, want) {
		t.Fatalf("liste=%v want=%v", z, want)
	}
	if !saatDilimiGecerli(z, "Europe/Istanbul") {
		t.Error("geçerli dilim reddedildi")
	}
	for _, bad := range []string{"", "../../etc/passwd", "Europe/Istanbul; reboot", "Mars/Olympus"} {
		if saatDilimiGecerli(z, bad) {
			t.Errorf("geçersiz dilim kabul edildi: %q", bad)
		}
	}
}
