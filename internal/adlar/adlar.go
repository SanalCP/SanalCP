// Package adlar: panelin her yerinde kullanılan kimlik adlarının (tenant sistem
// kullanıcısı ve alan adı) TEK doğrulama ve normalleştirme noktası.
//
// Neden var: bu paket yazılmadan önce `sk` için depoda birbirinden farklı
// güçte 25'ten fazla ad-hoc muhafız vardı. Çoğu yalnızca
// `strings.HasPrefix(sk, "c_")` kontrol ediyordu — bu, `c_../../etc` gibi bir
// değeri geçirir; ardından bazı çağrı yerleri sonucu `filepath.Join` yerine düz
// string birleştirmesiyle yola çeviriyordu. Aynı şekilde alan adı için beş ayrı
// regex vardı ve hiçbiri aynı girdi kümesini kabul etmiyordu.
//
// Bugün sömürülebilir bir yol yok: `sk` daima provisioner.SlugFromDomain
// çıktısıdır ve o üretim `[^a-z0-9]+` karakterlerini `_` ile değiştirir. Bu
// paketin amacı, o tek üretim noktası değiştiğinde ya da yeni bir giriş yolu
// açıldığında 25 ayrı muhafızın sessizce zayıf kalmasını önlemektir.
package adlar

import (
	"errors"
	"strings"
)

// Alan adı ve etiket sınırları (RFC 1035 §2.3.4, RFC 1123 §2.1).
const (
	// AlanAdiMaksUzunluk: tam nitelikli alan adının sondaki nokta hariç azami uzunluğu.
	AlanAdiMaksUzunluk = 253
	// EtiketMaksUzunluk: tek bir DNS etiketinin azami uzunluğu.
	EtiketMaksUzunluk = 63
	// SKOnek: panelin ürettiği tenant sistem kullanıcılarının zorunlu ön eki.
	SKOnek = "c_"
	// SKMaksSlug: ön ek sonrası azami slug uzunluğu.
	//
	// provisioner.SlugFromDomain slug'ı 26 karakterde kesiyor, yani üretimde
	// buraya kadar dolan bir değer yok. Sınır yine de 60'ta tutuluyor: daha
	// önceki reKotaSK / reWafSK desenleri {1,60} kullanıyordu ve bu paket o
	// muhafızların yerine geçtiği için sınırı daraltmak, eski bir sürümde
	// üretilmiş uzun bir kullanıcı adı varsa kotayı sessizce çalışmaz hâle
	// getirirdi. Güvenlik açısından belirleyici olan uzunluk değil, karakter
	// allowlist'idir.
	SKMaksSlug = 60
)

// Hata metinleri paket adı ön eki TAŞIMAZ: bu değerler httpx.WriteError ile
// doğrudan son kullanıcıya gösteriliyor (ör. domains.Create -> 400 gövdesi).
var (
	// ErrSKBos: sistem kullanıcısı boş.
	ErrSKBos = errors.New("sistem kullanıcısı boş")
	// ErrSKOnek: "c_" ön eki yok.
	ErrSKOnek = errors.New(`sistem kullanıcısı "c_" ile başlamalı`)
	// ErrSKUzunluk: slug boş ya da çok uzun.
	ErrSKUzunluk = errors.New("sistem kullanıcısı uzunluğu sınır dışı")
	// ErrSKKarakter: allowlist dışı karakter.
	ErrSKKarakter = errors.New("sistem kullanıcısında izin verilmeyen karakter")

	// ErrAlanAdiBos: alan adı boş.
	ErrAlanAdiBos = errors.New("alan adı boş")
	// ErrAlanAdiUzunluk: alan adı 253 karakteri aşıyor.
	ErrAlanAdiUzunluk = errors.New("alan adı çok uzun (en fazla 253 karakter)")
	// ErrAlanAdiEtiketSayisi: en az iki etiket gerekli (ör. example.com).
	ErrAlanAdiEtiketSayisi = errors.New("alan adı en az iki bölüm içermeli (örnek: example.com)")
	// ErrEtiketBos: iki nokta arasında etiket yok (ör. "a..com").
	ErrEtiketBos = errors.New("alan adında boş bölüm var (örnek: a..com)")
	// ErrEtiketUzunluk: etiket 63 karakteri aşıyor.
	ErrEtiketUzunluk = errors.New("alan adı bölümü çok uzun (en fazla 63 karakter)")
	// ErrEtiketKarakter: etikette a-z0-9- dışı karakter.
	ErrEtiketKarakter = errors.New("alan adı yalnız a-z, 0-9 ve tire içerebilir")
	// ErrEtiketTire: etiket tire ile başlıyor ya da bitiyor.
	ErrEtiketTire = errors.New("alan adı bölümü tire ile başlayamaz veya bitemez")
	// ErrTLDBicim: son etiket yalnız harflerden oluşmuyor ya da çok kısa.
	ErrTLDBicim = errors.New("alan adı uzantısı en az iki harf olmalı ve yalnız harf içermeli")
)

// SKDogrula: tenant sistem kullanıcısını doğrular.
//
// Kural: "c_" + 1..SKMaksSlug adet [a-z0-9_]. Bu allowlist üç şeyi birden
// kapatır — yol kaçışı ("/" ve "." hiç kabul edilmiyor, dolayısıyla ".." da
// oluşamaz), argüman/shell enjeksiyonu (boşluk ve metakarakter yok) ve
// setquota'nın "salt rakamsa UID say" davranışı ("c_" ön eki bunu devre dışı
// bırakır).
//
// Girdiyi normalleştirmez: sk panelin kendi ürettiği bir değerdir, kullanıcının
// yazdığı bir metin değil. Kırpma ya da küçük harfe çevirme, bozuk bir değeri
// sessizce kabul edilebilir hâle getirip asıl hatayı gizlerdi.
func SKDogrula(sk string) error {
	if sk == "" {
		return ErrSKBos
	}
	if !strings.HasPrefix(sk, SKOnek) {
		return ErrSKOnek
	}
	slug := sk[len(SKOnek):]
	if len(slug) == 0 || len(slug) > SKMaksSlug {
		return ErrSKUzunluk
	}
	for i := 0; i < len(slug); i++ {
		c := slug[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return ErrSKKarakter
		}
	}
	return nil
}

// SKGecerli: SKDogrula'nın bool biçimi.
func SKGecerli(sk string) bool { return SKDogrula(sk) == nil }

// AlanAdiNormalize: alan adını kanonik biçime getirir ve doğrular.
//
// Normalleştirme: baştaki/sondaki boşluklar kırpılır, küçük harfe çevrilir ve
// tek bir sondaki kök noktası ("example.com.") atılır. Doğrulama başarısızsa
// boş string ve hata döner.
//
// Doğrulama, önceki alanAdiRe deseninden DAHA SIKIDIR: o desen boş etiketli
// "a..com" ve tire ile biten "a-.com" gibi geçersiz DNS adlarını kabul
// ediyordu; ikisi de nginx server_name'e ve sertifika taleplerine akıyordu.
func AlanAdiNormalize(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	// Kök noktası kanonik olarak yazılabilir; tek bir tanesini atıyoruz.
	// Birden fazlaysa geriye boş etiket kalır ve aşağıdaki döngü reddeder.
	d = strings.TrimSuffix(d, ".")
	if d == "" {
		return "", ErrAlanAdiBos
	}
	if len(d) > AlanAdiMaksUzunluk {
		return "", ErrAlanAdiUzunluk
	}
	etiketler := strings.Split(d, ".")
	if len(etiketler) < 2 {
		return "", ErrAlanAdiEtiketSayisi
	}
	for _, e := range etiketler {
		if err := etiketDogrula(e); err != nil {
			return "", err
		}
	}
	// Son etiket (TLD) yalnız harf içermeli: "example.123" bir alan adı değildir.
	tld := etiketler[len(etiketler)-1]
	if len(tld) < 2 {
		return "", ErrTLDBicim
	}
	for i := 0; i < len(tld); i++ {
		if c := tld[i]; c < 'a' || c > 'z' {
			return "", ErrTLDBicim
		}
	}
	return d, nil
}

// AlanAdiGecerli: girdi normalleştirme GEREKTİRMEDEN geçerli mi?
//
// Yalnız halihazırda kanonik olan (küçük harfli, kırpılmış, sondaki noktasız)
// değerler için true döner. Kullanıcı girdisini doğrularken
// AlanAdiNormalize kullanın; DB'den okunan kanonik bir değeri kontrol
// ederken bunu kullanın.
func AlanAdiGecerli(d string) bool {
	n, err := AlanAdiNormalize(d)
	return err == nil && n == d
}

// RefererDeseni: nginx `valid_referers` direktifine gömülen referrer desenini
// doğrular ve kanonik biçimini döner. Kabul edilen biçimler:
//
//	example.com     tam eşleşme
//	.example.com    alan adı ve tüm alt alanları
//	*.example.com   yalnız alt alanlar
//
// Yerini aldığı `^\*?\.?[a-zA-Z0-9.-]+$` deseni "..", "a.com.", "-a.com" ve
// salt "---" gibi değerleri kabul ediyordu; hepsi doğrudan nginx yapılandırma
// dosyasına yazılıyordu.
func RefererDeseni(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	onek := ""
	switch {
	case strings.HasPrefix(s, "*."):
		onek, s = "*.", s[2:]
	case strings.HasPrefix(s, "."):
		onek, s = ".", s[1:]
	}
	d, err := AlanAdiNormalize(s)
	if err != nil {
		return "", err
	}
	return onek + d, nil
}

// EtiketGecerli: tek bir DNS etiketi (alt alan adı bileşeni) geçerli mi?
func EtiketGecerli(e string) bool { return etiketDogrula(e) == nil }

func etiketDogrula(e string) error {
	if len(e) == 0 {
		return ErrEtiketBos
	}
	if len(e) > EtiketMaksUzunluk {
		return ErrEtiketUzunluk
	}
	if e[0] == '-' || e[len(e)-1] == '-' {
		return ErrEtiketTire
	}
	for i := 0; i < len(e); i++ {
		c := e[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return ErrEtiketKarakter
		}
	}
	return nil
}
