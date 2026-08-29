package guvenlikolay

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnahtarKararlıVeKapsamlı(t *testing.T) {
	a := aday{Tur: "yogun_giris_deneme", IP: "192.0.2.10", Ilk: "2026-08-29 10:01:00", Son: "2026-08-29 10:09:00", DomainID: sql.NullInt64{Int64: 7, Valid: true}}
	if anahtar(a) != anahtar(a) {
		t.Fatal("aynı olay farklı dedup anahtarı üretti")
	}
	b := a
	b.IP = "192.0.2.11"
	if anahtar(a) == anahtar(b) {
		t.Fatal("farklı IP aynı anahtara düştü")
	}
	c := a
	c.DomainID.Int64 = 8
	if anahtar(a) == anahtar(c) {
		t.Fatal("farklı domain aynı anahtara düştü")
	}
}

func TestWAFAuditNormalizasyonu(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	veri := `--abc-A--
[29/Aug/2026:10:00:00 +0300] id 192.0.2.4 1234 203.0.113.2 443
--abc-B--
GET /?id=1 HTTP/1.1
Host: shop.example.com
--abc-H--
ModSecurity: Warning. [id "942100"] [msg "SQL Injection Attack Detected"]
--abc-Z--
`
	if err := os.WriteFile(p, []byte(veri), 0600); err != nil {
		t.Fatal(err)
	}
	got := wafAdaylari(p)
	if len(got) != 1 {
		t.Fatalf("aday=%#v", got)
	}
	if got[0].IP != "192.0.2.4" || got[0].Sayi != 1 || !strings.Contains(got[0].Aciklama, "shop.example.com") {
		t.Fatalf("normalizasyon yanlış: %#v", got[0])
	}
}
