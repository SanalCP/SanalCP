package provisioner

import (
	"strings"
	"testing"

	"sanalcp/internal/adlar"
)

// FuzzSlugFromDomainSKGecerli: panelin TEK sk üretim noktası ile TEK sk
// doğrulama noktası arasındaki sözleşmeyi sabitler.
//
// Depodaki 40'tan fazla muhafız artık adlar.SKGecerli'ye dayanıyor ve hepsi
// "sk daima SlugFromDomain çıktısıdır" varsayımıyla güvenli. Bu iki taraf
// birbirinden bağımsız değişirse — ör. SlugFromDomain'in izin verdiği karakter
// kümesi genişlerse ya da uzunluk kesme sınırı büyürse — muhafızlar geçerli
// hesapları reddetmeye başlar ve kota/WAF/yedek yolları sessizce kapanır.
// Buradaki değişmez o sapmayı derleme değil, test zamanında yakalar.
func FuzzSlugFromDomainSKGecerli(f *testing.F) {
	for _, d := range []string{
		"example.com",
		"EXAMPLE.COM",
		"a-b.example.co.uk",
		"1.example.com",
		strings.Repeat("a", 63) + ".com",
		strings.Repeat("ab.", 20) + "com",
		"xn--nda.com",
		"a..com",
		"-a.com",
		"örnek.com",
		"",
	} {
		f.Add(d)
	}
	f.Fuzz(func(t *testing.T, d string) {
		if ValidateDomain(d) != nil {
			return // geçersiz alan adı zaten hesap üretimine girmiyor
		}
		sk := SlugFromDomain(d)
		if err := adlar.SKDogrula(sk); err != nil {
			t.Fatalf("ValidateDomain(%q) geçti ama SlugFromDomain -> %q, SKDogrula: %v", d, sk, err)
		}
	})
}

// FuzzSlugFromDomainHicbirZamanTehlikeli: SlugFromDomain'e GEÇERSİZ alan adı da
// verilebilir (bazı yollar doğrulamadan önce slug üretiyor). Çıktı o durumda
// bile yol bileşeni olarak güvenli kalmalı — güvenli değilse, çağıranın
// doğrulamayı atladığı her yer yol kaçışına açılır.
func FuzzSlugFromDomainHicbirZamanTehlikeli(f *testing.F) {
	for _, d := range []string{
		"../../etc/passwd", "a/b", "a\\b", "..", ".", "a;id", "a$(id)",
		"example.com", "", "   ", "\x00", strings.Repeat("../", 40),
	} {
		f.Add(d)
	}
	f.Fuzz(func(t *testing.T, d string) {
		sk := SlugFromDomain(d)
		if strings.ContainsAny(sk, `/\`) {
			t.Fatalf("SlugFromDomain(%q) = %q, yol ayırıcı içeriyor", d, sk)
		}
		if strings.Contains(sk, "..") {
			t.Fatalf("SlugFromDomain(%q) = %q, \"..\" içeriyor", d, sk)
		}
		if !strings.HasPrefix(sk, adlar.SKOnek) {
			t.Fatalf("SlugFromDomain(%q) = %q, %q ön eki yok", d, sk, adlar.SKOnek)
		}
		// Tek istisna: girdi hiç alfasayısal karakter içermiyorsa slug boşalır
		// ve geriye salt "c_" kalır. Bu değer SKGecerli'yi GEÇMEZ, yani
		// muhafızlar onu reddeder — tehlikeli değil, sadece geçersizdir.
		if sk != adlar.SKOnek && !adlar.SKGecerli(sk) {
			t.Fatalf("SlugFromDomain(%q) = %q, ne boş slug ne de geçerli sk", d, sk)
		}
	})
}
