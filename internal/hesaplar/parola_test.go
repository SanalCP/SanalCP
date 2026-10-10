package hesaplar

import "testing"

func TestParolaGucluMu(t *testing.T) {
	gecerli := []string{
		"guvenliPar12",
		"EnAzOnIki123",
		"abc123456789",
		"Parola!12345",
	}
	for _, p := range gecerli {
		if ok, neden := ParolaGucluMu(p); !ok {
			t.Errorf("ParolaGucluMu(%q) = false, neden: %s", p, neden)
		}
	}

	gecersiz := []struct {
		pw    string
		neden string
	}{
		{"", "parola en az 12 karakter olmalı"},
		{"kisa", "parola en az 12 karakter olmalı"},
		{"12345678901", "parola en az 12 karakter olmalı"},          // 11
		{"abcdefghijkl", "parola harf ve rakam içermeli (karışık)"}, // harf var rakam yok
		{"123456789012", "parola harf ve rakam içermeli (karışık)"}, // rakam var harf yok
		{"guvenli\nPar12", "parola geçersiz karakter (satır sonu/kontrol) içeriyor"},
		{"guvenli\rPar12", "parola geçersiz karakter (satır sonu/kontrol) içeriyor"},
	}
	for _, tc := range gecersiz {
		ok, neden := ParolaGucluMu(tc.pw)
		if ok {
			t.Errorf("ParolaGucluMu(%q) = true olmamalıydı", tc.pw)
			continue
		}
		if neden != tc.neden {
			t.Errorf("ParolaGucluMu(%q) neden = %q, want %q", tc.pw, neden, tc.neden)
		}
	}
}

func TestParolaGecerli(t *testing.T) {
	if !ParolaGecerli("normalParola123") {
		t.Error("normal parola geçerli olmalı")
	}
	for _, p := range []string{"a\nb", "a\rb", "a\x00b"} {
		if ParolaGecerli(p) {
			t.Errorf("ParolaGecerli(%q) = true olmamalıydı", p)
		}
	}
}

func TestParolaEnAzKarakterOnIki(t *testing.T) {
	if ParolaEnAzKarakter != 12 {
		t.Errorf("ParolaEnAzKarakter = %d, want 12", ParolaEnAzKarakter)
	}
}
