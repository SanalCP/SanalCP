package domains

import "testing"

// Site tipi NE SAĞLANACAĞINI belirler. Bilinmeyen/boş değer "php"ye düşmeli:
// eski istemciler ve site_tipi göndermeyen API çağrıları bozulmamalı, ama
// "statik" da yanlışlıkla üretilmemeli (o durumda veritabanı açılmaz).
func TestGecerliSiteTipi(t *testing.T) {
	for _, d := range []struct{ girdi, bekleyen string }{
		{"statik", "statik"},
		{"STATIK", "statik"},
		{" statik ", "statik"},
		{"wordpress", "wordpress"},
		{"WordPress", "wordpress"},
		{"php", "php"},
		{"", "php"},       // eski istemci — alan hiç gönderilmiyor
		{"nodejs", "php"}, // henüz desteklenmiyor
		{"static", "php"}, // İngilizcesi değil, Türkçesi bekleniyor
		{"; DROP TABLE", "php"},
	} {
		if got := gecerliSiteTipi(d.girdi); got != d.bekleyen {
			t.Errorf("gecerliSiteTipi(%q) = %q, beklenen %q", d.girdi, got, d.bekleyen)
		}
	}
}

// 🔴 Asıl sözleşme: yalnız "statik" veritabanını atlar. Bu koşul yanlışlıkla
// tersine dönerse PHP/WordPress siteleri veritabanısız açılır ve kullanıcı bunu
// ancak uygulama bağlanamayınca fark eder.
func TestYalnizStatikVeritabaniniAtlar(t *testing.T) {
	for _, tip := range []string{"php", "wordpress"} {
		if gecerliSiteTipi(tip) == "statik" {
			t.Errorf("%q veritabanı atlanacak tip olarak değerlendirildi", tip)
		}
	}
	if gecerliSiteTipi("statik") != "statik" {
		t.Error("statik tip tanınmadı — veritabanı gereksiz yere açılırdı")
	}
}
