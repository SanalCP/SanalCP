package proxyport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 127.0.0.1:8090 (uid 0) ve 0.0.0.0:3000 (uid 1001) dinleniyor; 127.0.0.1:4000
// ESTABLISHED (dinleme değil, sayılmamalı).
const ornekTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F9A 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0000000000000000 100 0 0 10 0
   1: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1001        0 2222 1 0000000000000000 100 0 0 10 0
   2: 0100007F:0FA0 0100007F:D431 01 00000000:00000000 00:00000000 00000000  1002        0 3333 1 0000000000000000 100 0 0 10 0
`

func ornekProc(t *testing.T) {
	t.Helper()
	yol := filepath.Join(t.TempDir(), "tcp")
	if err := os.WriteFile(yol, []byte(ornekTCP), 0o644); err != nil {
		t.Fatal(err)
	}
	eski := procNetDosyalari
	procNetDosyalari = []string{yol, filepath.Join(t.TempDir(), "tcp6-yok")}
	t.Cleanup(func() { procNetDosyalari = eski })
}

func TestDinleyenUIDler(t *testing.T) {
	ornekProc(t)
	for port, beklenen := range map[int][]int{8090: {0}, 3000: {1001}, 4000: nil, 5000: nil} {
		got, err := DinleyenUIDler(port)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(beklenen) || (len(got) == 1 && got[0] != beklenen[0]) {
			t.Errorf("port %d: %v, beklenen %v", port, got, beklenen)
		}
	}
}

func TestTenantIcinDogrula(t *testing.T) {
	ornekProc(t)
	ctx := context.Background()
	durumlar := []struct {
		ad       string
		port     int
		uid      int
		reddedil bool
		mesaj    string
	}{
		{"panel CLI API", 8090, 1001, true, "ayrılmış"},
		{"panel API", 8080, 1001, true, "ayrılmış"},
		{"başka tenant'ın uygulaması", 3000, 1002, true, "başka bir hizmet"},
		{"yeni domain dolu porta", 3000, -1, true, "başka bir hizmet"},
		{"kendi uygulaması", 3000, 1001, false, ""},
		{"boş port", 5000, 1002, false, ""},
		{"yalnız ESTABLISHED olan port", 4000, 1001, false, ""},
	}
	for _, d := range durumlar {
		err := TenantIcinDogrula(ctx, nil, d.port, "c_test", d.uid, 0)
		if d.reddedil && (err == nil || !strings.Contains(err.Error(), d.mesaj)) {
			t.Errorf("%s: reddedilmeliydi (%q), hata=%v", d.ad, d.mesaj, err)
		}
		if !d.reddedil && err != nil {
			t.Errorf("%s: kabul edilmeliydi, hata=%v", d.ad, err)
		}
	}
}
