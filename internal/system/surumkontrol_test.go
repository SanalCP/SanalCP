package system

import (
	"testing"
	"time"
)

func TestSurumTetikleGerekliMi(t *testing.T) {
	simdi := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if !surumTetikleGerekliMi(time.Time{}, simdi) {
		t.Error("hiç kontrol yapılmamışsa (sıfır zaman) her zaman gerekli olmalı")
	}
	if surumTetikleGerekliMi(simdi.Add(-1*time.Minute), simdi) {
		t.Error("cooldown içindeyken (1dk önce) tekrar gerekmemeli")
	}
	if surumTetikleGerekliMi(simdi.Add(-4*time.Minute-59*time.Second), simdi) {
		t.Error("cooldown sınırının hemen altında tekrar gerekmemeli")
	}
	if !surumTetikleGerekliMi(simdi.Add(-5*time.Minute), simdi) {
		t.Error("cooldown süresi tam dolduğunda gerekli olmalı")
	}
	if !surumTetikleGerekliMi(simdi.Add(-24*time.Hour), simdi) {
		t.Error("cooldown'dan çok daha eskiyse gerekli olmalı")
	}
}
