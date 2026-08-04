package dns

import "testing"

func TestGecerliNSHost(t *testing.T) {
	gecerli := []string{
		"ns1.sanalcp.com",
		"ns2.ornek.com.tr",
		"a.b.c.example.org",
	}
	for _, s := range gecerli {
		if !GecerliNSHost(s) {
			t.Errorf("geçerli host reddedildi: %q", s)
		}
	}

	// Zone dosyasına yazılacağı için satır/yönerge enjeksiyonu ve
	// tam-nitelikli-olmayan değerler REDDEDİLMELİ.
	kotu := []string{
		"",
		"ns1",                      // tek etiket, FQDN değil
		"ns1.com\nIN NS evil.com.", // satır enjeksiyonu
		"$INCLUDE /etc/passwd",     // zone yönerge enjeksiyonu
		"ns1 .ornek.com",           // boşluk
		"-ns1.ornek.com",           // tire ile başlıyor
		"ns1.ornek.c",              // TLD çok kısa
		"ns1.ornek.com;evil",       // yorum/ayraç
		"178.162.242.174",          // IP adresi NS hostname değildir
	}
	for _, s := range kotu {
		if GecerliNSHost(s) {
			t.Errorf("geçersiz host kabul edildi: %q", s)
		}
	}
}

// SOA'nın primary NS'i zone'un NS kayıtlarıyla tutarlı olmalı; geçersiz bir
// değer geldiğinde eski (vanity) davranışa güvenli biçimde düşmeli.
func TestDefaultSOAPrimaryNS(t *testing.T) {
	s := defaultSOA("musteri.com", "ns1.saglayici.com")
	if s.PrimaryNS != "ns1.saglayici.com" {
		t.Errorf("ortak ns1 SOA'ya yazılmalıydı: %q", s.PrimaryNS)
	}
	if s.Hostmaster != "admin@musteri.com" {
		t.Errorf("hostmaster domainden türetilmeli: %q", s.Hostmaster)
	}

	bos := defaultSOA("musteri.com", "")
	if bos.PrimaryNS != "ns1.musteri.com" {
		t.Errorf("geçersiz ns1'de vanity geri düşüşü bekleniyordu: %q", bos.PrimaryNS)
	}
	cop := defaultSOA("musteri.com", "ns1 bozuk\ndeğer")
	if cop.PrimaryNS != "ns1.musteri.com" {
		t.Errorf("bozuk ns1 zone'a sızmamalıydı: %q", cop.PrimaryNS)
	}
}

// Varsayılan şablon ortak nameserver modelinde olmalı: NS kayıtları {NS1}/{NS2}
// kullanmalı ve müşteri domaininin altında ns1/ns2 A kaydı ÜRETİLMEMELİ
// (vanity model her domain için glue record gerektirirdi — bkz. nameserver.go).
func TestVarsayilanSablonOrtakNameserverKullanir(t *testing.T) {
	var nsDegerleri []string
	for _, r := range builtinDefaults() {
		if r.Tip == "NS" {
			nsDegerleri = append(nsDegerleri, r.Deger)
		}
		if r.Tip == "A" && (r.Ad == "ns1" || r.Ad == "ns2") {
			t.Errorf("şablon hâlâ vanity ns A kaydı üretiyor: %s", r.Ad)
		}
		if r.Tip == "NS" && r.Deger == "ns1.{DOMAIN}" {
			t.Error("şablon hâlâ vanity NS kullanıyor")
		}
	}
	if len(nsDegerleri) != 2 {
		t.Fatalf("2 NS kaydı bekleniyordu: %v", nsDegerleri)
	}
	if nsDegerleri[0] != "{NS1}" || nsDegerleri[1] != "{NS2}" {
		t.Errorf("NS kayıtları {NS1}/{NS2} olmalı: %v", nsDegerleri)
	}
}

func TestSubstNameserverPlaceholder(t *testing.T) {
	got := subst("{NS1}", "musteri.com", "1.2.3.4", "default", "", "ns1.saglayici.com", "ns2.saglayici.com")
	if got != "ns1.saglayici.com" {
		t.Errorf("{NS1} çözülmedi: %q", got)
	}
	got = subst("{NS2}", "musteri.com", "1.2.3.4", "default", "", "ns1.saglayici.com", "ns2.saglayici.com")
	if got != "ns2.saglayici.com" {
		t.Errorf("{NS2} çözülmedi: %q", got)
	}
	// Diğer placeholder'lar bozulmamalı.
	got = subst("mail.{DOMAIN}", "musteri.com", "1.2.3.4", "default", "", "a.b.com", "c.d.com")
	if got != "mail.musteri.com" {
		t.Errorf("{DOMAIN} bozuldu: %q", got)
	}
}

// Panel çoğunlukla bir alt alan adında çalışır (cloud.saglayici.com).
// Otomatik türetim yapılsaydı "ns1.cloud.saglayici.com" yayınlanır ve
// sağlayıcının gerçek nameserver'ları olmadığı için müşteri domainleri
// çözülemezdi. Bu yüzden yalnız ÖNERİ üretilir ve marka alan adı tahmin edilir.
func TestOneriliNSMarkaAlanAdiniTahminEder(t *testing.T) {
	testler := map[string]string{
		"cloud.sanalcp.com":  "ns1.sanalcp.com",
		"panel.ornek.com.tr": "ns1.ornek.com.tr",
		// Zaten kök alan adı: ilk etiket atılırsa "com" kalırdı — atılmaz.
		"sanalcp.com": "ns1.sanalcp.com",
	}
	for girdi, beklenen := range testler {
		p := girdi
		if parcalar := splitIlk(p); parcalar != "" {
			p = parcalar
		}
		if got := "ns1." + p; got != beklenen {
			t.Errorf("%q için %q bekleniyordu, %q", girdi, beklenen, got)
		}
	}
}

// splitIlk: OneriliNS içindeki tahmin kuralının test edilebilir kopyası.
func splitIlk(p string) string {
	i := indexNokta(p)
	if i < 0 {
		return ""
	}
	kalan := p[i+1:]
	if indexNokta(kalan) < 0 {
		return "" // tek etikete düştü → tahmin anlamsız
	}
	return kalan
}

func indexNokta(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
