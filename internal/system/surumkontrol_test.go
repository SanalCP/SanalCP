package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestSurumKontrolDurumKurulumKimligiIcerir — kurulum_kimlik'in rol kısıtlı
// (BayiVeUstu, bkz. cmd/server/main.go) /system/surum-kontrol yanıtında
// bulunduğunu doğrular. Önceden bu alan rol kısıtı OLMAYAN /system/surum'da
// (SurumBilgi) idi — son inceleme bulgusu üzerine buraya taşındı.
func TestSurumKontrolDurumKurulumKimligiIcerir(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/system/surum-kontrol", nil)
	w := httptest.NewRecorder()
	SurumKontrolDurum(w, r)

	var body map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("yanıt parse edilemedi: %v", err)
	}
	// KurulumKimligi() dosya yazımı başarısız olsa bile (test ortamında
	// /etc/sanalcp'e yazma izni olmayabilir) rastgele üretilmiş değeri
	// döndürür — bkz. surumkontrol.go, bu yüzden test izole dizin gerektirmez.
	kimlik, _ := body["kurulum_kimlik"].(string)
	if len(kimlik) < 16 {
		t.Errorf("kurulum_kimlik beklenmedik: %q", kimlik)
	}
}

// TestSurumBilgiKurulumKimligiIcermez — /system/surum rol kısıtı olmadan
// TÜM oturum açmış kullanıcılara (müşteri dahil) açık; kurulum_kimlik gibi
// kurulumun lisans/telemetri kimliğini taşıyan bir alan bu uca ASLA
// sızmamalı (bkz. surumkontrol.go'daki KurulumKimligi() gizlilik notu).
func TestSurumBilgiKurulumKimligiIcermez(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/system/surum", nil)
	w := httptest.NewRecorder()
	SurumBilgi(w, r)

	var body map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("yanıt parse edilemedi: %v", err)
	}
	if _, varMi := body["kurulum_kimlik"]; varMi {
		t.Errorf("SurumBilgi (/system/surum, rol kısıtsız) kurulum_kimlik içermemeli: %v", body)
	}
}
