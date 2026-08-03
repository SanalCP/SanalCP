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

// TestSurumOnbellekGuvenilirMi: canlıda gözlemlenen gerçek hatayı kapsar —
// güncelleme sonrası eski (bir önceki sürüm tarafından yazılmış) önbellek
// yüklenirse "kurulu: 0.3.7 → yeni: 0.3.5" gibi ters/anlamsız bir bildirim
// çıkıyordu.
func TestSurumOnbellekGuvenilirMi(t *testing.T) {
	if !surumOnbellekGuvenilirMi("0.3.7", "0.3.7") {
		t.Error("aynı sürüm tarafından yazılmışsa güvenilir olmalı (ör. sadece systemctl restart)")
	}
	if surumOnbellekGuvenilirMi("0.3.5", "0.3.7") {
		t.Error("FARKLI (eski) sürüm tarafından yazılmışsa güvenilmez olmalı — güncelleme araya girmiş")
	}
	if surumOnbellekGuvenilirMi("", "0.3.7") {
		t.Error("bu alanı hiç içermeyen eski biçimli önbellek güvenilmez sayılmalı")
	}
}
