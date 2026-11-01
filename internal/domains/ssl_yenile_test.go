package domains

import (
	"testing"
	"time"
)

func TestSSLYenilenmeli(t *testing.T) {
	simdi := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	gun := func(n int) time.Time { return simdi.Add(time.Duration(n) * 24 * time.Hour) }
	for _, c := range []struct {
		kaynak string
		bitis  time.Time
		want   bool
	}{
		{"letsencrypt", gun(-1), true},  // süresi dolmuş (sanalcp.com vakası)
		{"letsencrypt", gun(3), true},   // acme.sh kaydı yok, 3 gün kaldı
		{"letsencrypt", gun(25), false}, // acme.sh'in penceresi: ona bırak
		{"letsencrypt", gun(90), false},
		{"self-signed", gun(-1), false}, // LE dışı kaynaklara dokunma
		{"imported", gun(3), false},
	} {
		if got := sslYenilenmeli(c.kaynak, c.bitis, simdi); got != c.want {
			t.Errorf("%s bitis=%s: got %v want %v", c.kaynak, c.bitis.Format("2006-01-02"), got, c.want)
		}
	}
}
