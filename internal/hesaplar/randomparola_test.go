package hesaplar

import (
	"math"
	"strings"
	"testing"
)

const testAlfabe = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

func TestRandomParola_UzunlukVeAlfabe(t *testing.T) {
	for _, n := range []int{1, 8, 18, 20, 24, 64} {
		p := RandomParola(n)
		if len(p) != n {
			t.Fatalf("RandomParola(%d) uzunluk %d döndü", n, len(p))
		}
		for _, c := range p {
			if !strings.ContainsRune(testAlfabe, c) {
				t.Fatalf("alfabe dışı karakter %q (n=%d, parola=%q)", c, n, p)
			}
		}
	}
	// n<=0 → varsayılan 20
	if got := len(RandomParola(0)); got != 20 {
		t.Fatalf("RandomParola(0) uzunluk %d, 20 bekleniyordu", got)
	}
	if got := len(RandomParola(-5)); got != 20 {
		t.Fatalf("RandomParola(-5) uzunluk %d, 20 bekleniyordu", got)
	}
}

// TestRandomParola_ModuloBiasYok — asıl regresyon testi.
//
// Eski kod `harf[b[i]%56]` yapıyordu; 256%56=32 olduğundan alfabenin ilk 32
// karakteri kalan 24'ten %25 daha sık çıkardı. Reddetme örneklemesiyle bu fark
// kapanmalı. Test, "ilk 32" ve "son 24" gruplarının karakter-başına ortalama
// frekanslarını karşılaştırır: biaslı kodda oran ~1.25, düzgün dağılımda ~1.00.
func TestRandomParola_ModuloBiasYok(t *testing.T) {
	const ornek = 200000
	sayac := map[rune]int{}
	for toplam := 0; toplam < ornek; toplam += 100 {
		for _, c := range RandomParola(100) {
			sayac[c]++
		}
	}

	// 256 = 4*56 + 32 → 224..255 aralığı ilk 32 indekse fazladan bir tur bindirir,
	// yani indeks 0..31 beş kez, 32..55 dört kez erişilebilirdi (5/4 = 1.25).
	biasliSinir := 256 % len(testAlfabe) // 32
	var ilkGrup, sonGrup int
	for i, c := range testAlfabe {
		if i < biasliSinir {
			ilkGrup += sayac[c]
		} else {
			sonGrup += sayac[c]
		}
	}
	ilkOrt := float64(ilkGrup) / float64(biasliSinir)
	sonOrt := float64(sonGrup) / float64(len(testAlfabe)-biasliSinir)
	oran := ilkOrt / sonOrt

	// Biaslı kodda 1.25 çıkar. %4 tolerans örnekleme gürültüsü için fazlasıyla yeterli.
	if math.Abs(oran-1.0) > 0.04 {
		t.Fatalf("modulo bias saptandı: ilk32/son24 frekans oranı %.4f (1.00 bekleniyordu, "+
			"biaslı uygulamada 1.25 olurdu)", oran)
	}

	// Alfabenin her karakteri en az bir kez görülmeli (hiçbiri erişilemez olmasın).
	for _, c := range testAlfabe {
		if sayac[c] == 0 {
			t.Fatalf("%q karakteri hiç üretilmedi", c)
		}
	}
}

// TestRandomParola_Benzersiz — sıfırlanmış tampon (yutulan rand hatası)
// regresyonu: aynı çağrı iki kez aynı sonucu vermemeli.
func TestRandomParola_Benzersiz(t *testing.T) {
	gorulen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		p := RandomParola(20)
		if gorulen[p] {
			t.Fatalf("yinelenen parola üretildi: %q", p)
		}
		gorulen[p] = true
		if p == strings.Repeat("A", 20) {
			t.Fatal("sıfır tampon belirtisi: parola tamamen 'A'")
		}
	}
}
