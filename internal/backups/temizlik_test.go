package backups

import "testing"

// Yedek tipini dosya adından ayırmak, DB kaydı olmayan (yetim) dosyalar için
// tek dayanaktır — toplu temizlikte "otomatik" ile "manuel"i burası ayırır.
// Yanlış sınıflandırma, kullanıcının korumak istediği manuel yedeği siler.
func TestOtomatikDosya(t *testing.T) {
	durumlar := []struct {
		ad      string
		sk      string
		dosya   string
		bekleme bool
	}{
		{"otomatik yedek", "c_ornek", "c_ornek-auto-20260812-030000.tar.gz", true},
		{"manuel yedek", "c_ornek", "c_ornek-20260812-131500.tar.gz", false},

		// Kritik tuzak: sistem kullanıcısı domainden türer, dolayısıyla sk'nın
		// KENDİSİ "-auto" ile bitebilir. Düz strings.Contains(ad, "-auto-")
		// kontrolü bu domainin MANUEL yedeğini otomatik sanır ve "tüm otomatik
		// yedekleri sil" işleminde onu da götürürdü.
		{"sk '-auto' ile biterken manuel", "c_my-auto", "c_my-auto-20260812-131500.tar.gz", false},
		{"sk '-auto' ile biterken otomatik", "c_my-auto", "c_my-auto-auto-20260812-030000.tar.gz", true},

		// Başka bir domainin dosyası kendi dizininde görünmemeli; görünürse de
		// önek tutmadığı için otomatik sayılmaz (fail-safe: silinmez tarafa düşer).
		{"başka domainin dosyası", "c_ornek", "c_baska-auto-20260812-030000.tar.gz", false},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			if got := otomatikDosya(d.sk, d.dosya); got != d.bekleme {
				t.Errorf("otomatikDosya(%q, %q) = %v; beklenen %v", d.sk, d.dosya, got, d.bekleme)
			}
		})
	}
}
