package phpsurum

import (
	"testing"
	"time"
)

// Sweeper sorguları önbellekten (dnf -C) yapar; önbelleği tur başında TEK bir
// geniş timeout'lu tazeleme doldurur. Tazeleme sorgulardan önce gelmezse ilk
// açılışta (boş önbellek) tüm Remi sürümleri "kurulamaz" görünür.
func TestSweepOnceSorgulardanOnceDepoyuBirKezTazeler(t *testing.T) {
	eskiListe := DesteklenenSurumler
	DesteklenenSurumler = []SurumMeta{{"8.1", "81", "remi"}, {"8.2", "82", "remi"}}
	defer func() { DesteklenenSurumler = eskiListe }()

	var olaylar []string
	eskiProbe, eskiTazele := dnfProbe, depoTazele
	dnfProbe = func(pkg string) bool { olaylar = append(olaylar, "probe"); return true }
	depoTazele = func() { olaylar = append(olaylar, "tazele") }
	defer func() { dnfProbe, depoTazele = eskiProbe, eskiTazele }()

	sweepOnce()

	want := []string{"tazele", "probe", "probe"}
	if len(olaylar) != len(want) {
		t.Fatalf("olaylar = %v, want %v", olaylar, want)
	}
	for i := range want {
		if olaylar[i] != want[i] {
			t.Fatalf("olaylar = %v, want %v", olaylar, want)
		}
	}
}

// PHP sürüm listesi nadiren değişir; 10 dakikada bir dnf çalıştırmak 1 GB RAM'li
// sunucuda gereksiz yük ve (eski davranışta) disk sızıntısıydı.
func TestSweepPeriyoduSaatlerMertebesinde(t *testing.T) {
	if availTTL < time.Hour {
		t.Fatalf("availTTL = %v; en az 1 saat olmalı", availTTL)
	}
}
