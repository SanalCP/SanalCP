package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestSayfalamaAyristir(t *testing.T) {
	tests := []struct {
		ad           string
		url          string
		kip          bool
		hata         bool
		sayfa, limit int
		arama        string
	}{
		{"legacy", "/?bilinmeyen=1", false, false, 0, 0, ""},
		{"varsayılanlar", "/?arama=", true, false, 1, 50, ""},
		{"arama kırpılır", "/?arama=%20alan%20", true, false, 1, 50, "alan"},
		{"özel değerler", "/?sayfa=3&limit=200", true, false, 3, 200, ""},
		{"sayfa sıfır", "/?sayfa=0", true, true, 0, 0, ""},
		{"sayfa metin", "/?sayfa=x", true, true, 0, 0, ""},
		{"limit boş", "/?limit=", true, true, 0, 0, ""},
		{"limit negatif", "/?limit=-1", true, true, 0, 0, ""},
		{"limit üstü", "/?limit=201", true, true, 0, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.ad, func(t *testing.T) {
			p, kip, err := SayfalamaAyristir(httptest.NewRequest("GET", tc.url, nil))
			if kip != tc.kip || (err != nil) != tc.hata {
				t.Fatalf("kip=%v hata=%v", kip, err)
			}
			if err == nil && kip && (p.Sayfa != tc.sayfa || p.Limit != tc.limit || p.Arama != tc.arama) {
				t.Fatalf("sayfalama=%+v", p)
			}
		})
	}
}

func TestYeniSayfaliYanit(t *testing.T) {
	y := YeniSayfaliYanit([]int{1}, Sayfalama{Sayfa: 2, Limit: 50}, 51)
	if y.ToplamSayfa != 2 || y.Toplam != 51 {
		t.Fatalf("yanıt=%+v", y)
	}
}

func TestLikeDeseni(t *testing.T) {
	if got := LikeDeseni("a!%_b"); got != "%a!!!%!_b%" {
		t.Fatalf("desen=%q", got)
	}
}
