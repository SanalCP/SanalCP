package adlar

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSKDogrula(t *testing.T) {
	gecerli := []string{
		"c_example_com",
		"c_a",
		"c_0",
		"c__",
		"c_" + strings.Repeat("a", SKMaksSlug),
	}
	for _, sk := range gecerli {
		if err := SKDogrula(sk); err != nil {
			t.Errorf("SKDogrula(%q) = %v, geçerli olmalıydı", sk, err)
		}
	}

	gecersiz := map[string]error{
		"":                                       ErrSKBos,
		"root":                                   ErrSKOnek,
		"x_example":                              ErrSKOnek,
		"C_example":                              ErrSKOnek, // büyük harf ön ek kabul edilmez
		"c_":                                     ErrSKUzunluk,
		"c_" + strings.Repeat("a", SKMaksSlug+1): ErrSKUzunluk,
		"c_../../etc":                            ErrSKKarakter,
		"c_a/b":                                  ErrSKKarakter,
		`c_a\b`:                                  ErrSKKarakter,
		"c_a.b":                                  ErrSKKarakter,
		"c_a b":                                  ErrSKKarakter,
		"c_a;id":                                 ErrSKKarakter,
		"c_a$(id)":                               ErrSKKarakter,
		"c_a`id`":                                ErrSKKarakter,
		"c_a|b":                                  ErrSKKarakter,
		"c_a\nb":                                 ErrSKKarakter,
		"c_A":                                    ErrSKKarakter, // yalnız küçük harf
		"c_ü":                                    ErrSKKarakter,
	}
	for sk, beklenen := range gecersiz {
		err := SKDogrula(sk)
		if err == nil {
			t.Errorf("SKDogrula(%q) = nil, reddedilmeliydi", sk)
			continue
		}
		if err != beklenen {
			t.Errorf("SKDogrula(%q) = %v, beklenen %v", sk, err, beklenen)
		}
	}
}

// TestSKEskiMuhafizlarinGecirdigi: bu paketten önce çağrı yerlerinin çoğu
// yalnızca strings.HasPrefix(sk, "c_") kontrol ediyordu. O muhafızın geçirdiği
// ama tehlikeli olan değerlerin artık reddedildiğini sabitler.
func TestSKEskiMuhafizlarinGecirdigi(t *testing.T) {
	tehlikeli := []string{
		"c_../../etc/passwd",
		"c_/etc/passwd",
		"c_..",
		"c_.",
		"c_x/../../root",
	}
	for _, sk := range tehlikeli {
		if !strings.HasPrefix(sk, "c_") {
			t.Fatalf("test kurgusu bozuk: %q eski muhafızdan geçmiyor", sk)
		}
		if SKGecerli(sk) {
			t.Errorf("SKGecerli(%q) = true, reddedilmeliydi", sk)
		}
	}
}

func TestAlanAdiNormalize(t *testing.T) {
	normalize := map[string]string{
		"example.com":      "example.com",
		"  example.com  ":  "example.com",
		"EXAMPLE.COM":      "example.com",
		"Example.CoM":      "example.com",
		"example.com.":     "example.com",
		"a.b.c.example.co": "a.b.c.example.co",
		"xn--nda.com":      "xn--nda.com", // punycode etiketi
		"a-b.example.com":  "a-b.example.com",
		"1example.com":     "1example.com",
	}
	for girdi, beklenen := range normalize {
		got, err := AlanAdiNormalize(girdi)
		if err != nil {
			t.Errorf("AlanAdiNormalize(%q) = %v, geçerli olmalıydı", girdi, err)
			continue
		}
		if got != beklenen {
			t.Errorf("AlanAdiNormalize(%q) = %q, beklenen %q", girdi, got, beklenen)
		}
	}

	gecersiz := map[string]error{
		"":             ErrAlanAdiBos,
		"   ":          ErrAlanAdiBos,
		".":            ErrAlanAdiBos,
		"example":      ErrAlanAdiEtiketSayisi,
		"a..com":       ErrEtiketBos, // eski alanAdiRe bunu KABUL ediyordu
		"a.com..":      ErrEtiketBos,
		".example.com": ErrEtiketBos,
		"a-.com":       ErrEtiketTire, // eski alanAdiRe bunu KABUL ediyordu
		"-a.com":       ErrEtiketTire,
		"a.-com":       ErrEtiketTire,
		"a.com-":       ErrEtiketTire,
		"a_b.com":      ErrEtiketKarakter,
		"a/b.com":      ErrEtiketKarakter,
		"a b.com":      ErrEtiketKarakter,
		"a.com/x":      ErrEtiketKarakter,
		"örnek.com":    ErrEtiketKarakter, // IDN, punycode'a çevrilmiş olmalı
		"a.123":        ErrTLDBicim,
		"a.c":          ErrTLDBicim,
		"a.c0m":        ErrTLDBicim,
	}
	for girdi, beklenen := range gecersiz {
		_, err := AlanAdiNormalize(girdi)
		if err == nil {
			t.Errorf("AlanAdiNormalize(%q) = nil hata, reddedilmeliydi", girdi)
			continue
		}
		if err != beklenen {
			t.Errorf("AlanAdiNormalize(%q) = %v, beklenen %v", girdi, err, beklenen)
		}
	}
}

func TestAlanAdiUzunlukSinirlari(t *testing.T) {
	// 63 karakterlik etiket geçerli, 64 değil.
	if _, err := AlanAdiNormalize(strings.Repeat("a", EtiketMaksUzunluk) + ".com"); err != nil {
		t.Errorf("63 karakterlik etiket reddedildi: %v", err)
	}
	if _, err := AlanAdiNormalize(strings.Repeat("a", EtiketMaksUzunluk+1) + ".com"); err != ErrEtiketUzunluk {
		t.Errorf("64 karakterlik etiket için %v, beklenen %v", err, ErrEtiketUzunluk)
	}

	// Toplam 253 sınırı.
	uzun := strings.Repeat("a.", 130) + "com" // 263 karakter
	if _, err := AlanAdiNormalize(uzun); err != ErrAlanAdiUzunluk {
		t.Errorf("253'ten uzun alan adı için %v, beklenen %v", err, ErrAlanAdiUzunluk)
	}
}

func TestAlanAdiGecerliNormalleştirmeYapmaz(t *testing.T) {
	// AlanAdiGecerli yalnız halihazırda kanonik değerlere true demeli.
	if AlanAdiGecerli("EXAMPLE.COM") {
		t.Error(`AlanAdiGecerli("EXAMPLE.COM") = true, kanonik olmadığı için false olmalı`)
	}
	if AlanAdiGecerli("example.com.") {
		t.Error(`AlanAdiGecerli("example.com.") = true, kanonik olmadığı için false olmalı`)
	}
	if !AlanAdiGecerli("example.com") {
		t.Error(`AlanAdiGecerli("example.com") = false, true olmalı`)
	}
}

func TestEtiketGecerli(t *testing.T) {
	for _, e := range []string{"a", "0", "a-b", "xn--nda", strings.Repeat("a", EtiketMaksUzunluk)} {
		if !EtiketGecerli(e) {
			t.Errorf("EtiketGecerli(%q) = false, true olmalı", e)
		}
	}
	for _, e := range []string{"", "-a", "a-", "a.b", "a_b", "A", strings.Repeat("a", EtiketMaksUzunluk+1)} {
		if EtiketGecerli(e) {
			t.Errorf("EtiketGecerli(%q) = true, false olmalı", e)
		}
	}
}

func TestRefererDeseni(t *testing.T) {
	gecerli := map[string]string{
		"example.com":     "example.com",
		".example.com":    ".example.com",
		"*.example.com":   "*.example.com",
		"*.EXAMPLE.COM":   "*.example.com",
		"  .example.com ": ".example.com",
		"example.com.":    "example.com",
	}
	for girdi, beklenen := range gecerli {
		got, err := RefererDeseni(girdi)
		if err != nil {
			t.Errorf("RefererDeseni(%q) = %v, geçerli olmalıydı", girdi, err)
			continue
		}
		if got != beklenen {
			t.Errorf("RefererDeseni(%q) = %q, beklenen %q", girdi, got, beklenen)
		}
	}

	// Yerini aldığı `^\*?\.?[a-zA-Z0-9.-]+$` deseninin KABUL ettiği ama
	// nginx valid_referers'a yazılmaması gereken değerler.
	// Not: "a.com." burada YOK — tek bir sondaki kök noktası kanonik yazımdır
	// ve normalleştirmede soyulur (bkz. yukarıdaki geçerli tablosu).
	eskiDesenGecirirdi := []string{"..", "---", "-a.com", "a..com", "*..com", ".", "*."}
	for _, v := range eskiDesenGecirirdi {
		if _, err := RefererDeseni(v); err == nil {
			t.Errorf("RefererDeseni(%q) = nil hata, reddedilmeliydi", v)
		}
	}

	// Çift joker ya da gömülü joker kabul edilmez.
	for _, v := range []string{"*.*.example.com", "ex*ample.com", "**.example.com"} {
		if _, err := RefererDeseni(v); err == nil {
			t.Errorf("RefererDeseni(%q) = nil hata, reddedilmeliydi", v)
		}
	}
}

// FuzzRefererDeseni: çıktı doğrudan nginx yapılandırma dosyasına yazılıyor.
// Başarılı her sonucun direktif bağlamında zararsız olduğunu sabitler.
func FuzzRefererDeseni(f *testing.F) {
	for _, s := range []string{
		"example.com", "*.example.com", ".example.com", "..", "---",
		"*.*.a.com", "a.com.", "EXAMPLE.COM", "", "*.", "a;b.com",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ham string) {
		d, err := RefererDeseni(ham)
		if err != nil {
			if d != "" {
				t.Fatalf("hata durumunda boş olmayan çıktı: %q", d)
			}
			return
		}
		// nginx direktifini sonlandırabilecek ya da yeni direktif açabilecek
		// hiçbir karakter bulunmamalı.
		if strings.ContainsAny(d, " \t\r\n;{}#\"'\\/") {
			t.Fatalf("çıktı %q nginx direktif bağlamında tehlikeli karakter içeriyor", d)
		}
		// En fazla tek bir joker, o da yalnız başta.
		if n := strings.Count(d, "*"); n > 1 {
			t.Fatalf("çıktı %q birden fazla joker içeriyor", d)
		} else if n == 1 && !strings.HasPrefix(d, "*.") {
			t.Fatalf("çıktı %q joker içeriyor ama \"*.\" ile başlamıyor", d)
		}
		// Ön ek soyulduğunda geriye kanonik bir alan adı kalmalı.
		govde := strings.TrimPrefix(strings.TrimPrefix(d, "*."), ".")
		if !AlanAdiGecerli(govde) {
			t.Fatalf("çıktı %q gövdesi %q kanonik alan adı değil", d, govde)
		}
		// İdempotent olmalı.
		if tekrar, err2 := RefererDeseni(d); err2 != nil || tekrar != d {
			t.Fatalf("idempotent değil: %q -> %q -> (%q, %v)", ham, d, tekrar, err2)
		}
	})
}

// FuzzSKGecerli: geçerli sayılan HER sk'nin, çağrı yerlerinin dayandığı
// güvenlik özelliklerini sağladığını sabitler. Çağrı yerleri bugün sk'yi hem
// filepath.Join ile hem düz string birleştirmesiyle yola çeviriyor ve exec
// argümanı olarak geçiriyor; ikisi de aşağıdaki özelliklere dayanıyor.
func FuzzSKGecerli(f *testing.F) {
	for _, s := range []string{
		"c_example_com", "c_", "c_a", "root", "",
		"c_../../etc", "c_a/b", "c_a;id", "c_a$(id)", "c_A", "c_ü",
		"c_" + strings.Repeat("a", SKMaksSlug+1),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sk string) {
		if !SKGecerli(sk) {
			return
		}
		// 1) Yol kaçışı imkânsız olmalı.
		if strings.ContainsAny(sk, `/\`) {
			t.Fatalf("geçerli sayılan %q yol ayırıcı içeriyor", sk)
		}
		if strings.Contains(sk, "..") {
			t.Fatalf("geçerli sayılan %q \"..\" içeriyor", sk)
		}
		// Hem filepath.Join hem de düz birleştirme /home altında kalmalı ve
		// aynı sonucu vermeli — çağrı yerlerinde iki biçim de kullanılıyor.
		join := filepath.Join("/home", sk)
		duz := "/home/" + sk
		if join != duz {
			t.Fatalf("filepath.Join(%q) = %q, düz birleştirme %q — ayrışıyor", sk, join, duz)
		}
		if !strings.HasPrefix(filepath.Clean(duz)+"/", "/home/") {
			t.Fatalf("geçerli sayılan %q /home dışına çıkıyor: %q", sk, filepath.Clean(duz))
		}
		if filepath.Base(sk) != sk {
			t.Fatalf("geçerli sayılan %q tek yol bileşeni değil", sk)
		}

		// 2) exec argümanı ve yapılandırma dosyası bağlamı için metakarakter olmamalı.
		if strings.ContainsAny(sk, " \t\r\n;|&$`'\"<>*?[]{}()!#~^%,:=+") {
			t.Fatalf("geçerli sayılan %q metakarakter içeriyor", sk)
		}

		// 3) setquota'nın "salt rakamsa UID say" davranışı kapalı kalmalı.
		if !strings.HasPrefix(sk, SKOnek) {
			t.Fatalf("geçerli sayılan %q %q ön ekiyle başlamıyor", sk, SKOnek)
		}

		// 4) Yalnız ASCII: her bayt tek karakter olmalı ki uzunluk sınırları
		//    çok baytlı bir girdiyle atlatılamasın.
		for i := 0; i < len(sk); i++ {
			if sk[i] >= 0x80 {
				t.Fatalf("geçerli sayılan %q ASCII dışı bayt içeriyor", sk)
			}
		}
	})
}

// FuzzAlanAdiNormalize: normalize başarılıysa çıktının kanonik, idempotent ve
// yol/yapılandırma bağlamında güvenli olduğunu sabitler.
func FuzzAlanAdiNormalize(f *testing.F) {
	for _, s := range []string{
		"example.com", "EXAMPLE.COM", "  example.com. ", "a..com", "a-.com",
		"-a.com", "a.b", "..", "a.com.", "örnek.com", "a/b.com",
		strings.Repeat("a", EtiketMaksUzunluk+1) + ".com",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ham string) {
		d, err := AlanAdiNormalize(ham)
		if err != nil {
			if d != "" {
				t.Fatalf("hata durumunda boş olmayan çıktı: %q", d)
			}
			return
		}
		// Kanonik: idempotent ve AlanAdiGecerli'yi geçmeli.
		tekrar, err2 := AlanAdiNormalize(d)
		if err2 != nil || tekrar != d {
			t.Fatalf("idempotent değil: %q -> %q -> (%q, %v)", ham, d, tekrar, err2)
		}
		if !AlanAdiGecerli(d) {
			t.Fatalf("normalize çıktısı %q AlanAdiGecerli'yi geçmiyor", d)
		}
		// Yol ve yapılandırma bağlamı için güvenli olmalı: nginx server_name'e
		// ve sertifika taleplerine bu değer gidiyor.
		if strings.ContainsAny(d, ` /\;{}"'`+"\t\r\n") {
			t.Fatalf("normalize çıktısı %q tehlikeli karakter içeriyor", d)
		}
		if strings.Contains(d, "..") {
			t.Fatalf("normalize çıktısı %q boş etiket içeriyor", d)
		}
		if d != strings.ToLower(d) {
			t.Fatalf("normalize çıktısı %q küçük harfli değil", d)
		}
		if strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") {
			t.Fatalf("normalize çıktısı %q nokta ile başlıyor/bitiyor", d)
		}
		if len(d) > AlanAdiMaksUzunluk {
			t.Fatalf("normalize çıktısı %d karakter, sınır %d", len(d), AlanAdiMaksUzunluk)
		}
		for _, e := range strings.Split(d, ".") {
			if !EtiketGecerli(e) {
				t.Fatalf("normalize çıktısı %q geçersiz etiket içeriyor: %q", d, e)
			}
		}
	})
}
