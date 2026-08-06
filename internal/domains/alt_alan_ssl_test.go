package domains

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sertifikaYaz: verilen konu/veren ile bir sertifika üretip diske yazar.
// konu == veren ise sertifika kendi kendini imzalamış olur (self-signed).
func sertifikaYaz(t *testing.T, crtYol, keyYol, konu, veren string) {
	t.Helper()
	anahtar, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Veren farklıysa onu imzalayacak ayrı bir CA üret — böylece Issuer alanı
	// gerçekten farklı olur (Let's Encrypt zincirinin taklidi).
	imzalayan := anahtar
	sablon := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: konu},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	ust := sablon
	if veren != konu {
		caAnahtar, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		ust = &x509.Certificate{
			SerialNumber:          big.NewInt(2),
			Subject:               pkix.Name{CommonName: veren},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(48 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
		}
		imzalayan = caAnahtar
	}
	der, err := x509.CreateCertificate(rand.Reader, sablon, ust, &anahtar.PublicKey, imzalayan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crtYol, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyYol, []byte("anahtar-yer-tutucu"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 🔴 Kırmızı rozetin dayandığı karar: self-signed sertifika DOĞRU tespit
// edilmeli. Yanlış pozitif, gerçek bir Let's Encrypt sertifikasını "güvensiz"
// göstererek kullanıcıyı boşuna telaşlandırır; yanlış negatif ise ziyaretçinin
// tarayıcı uyarısı aldığı bir siteyi "güvenli" gösterir.
func TestAltAlanSSLKaynakTespiti(t *testing.T) {
	kok := t.TempDir()
	sslDizin := filepath.Join(kok, "home", "c_ornek", "ssl")
	if err := os.MkdirAll(sslDizin, 0o755); err != nil {
		t.Fatal(err)
	}

	// altAlanSSL /home altını okuduğu için testi o köke bağlayamıyoruz;
	// tespit mantığını aynı girdilerle doğrudan sınıyoruz.
	for _, d := range []struct {
		ad          string
		konu, veren string
		bekleyenKay string
	}{
		{"self.ornek.com", "self.ornek.com", "self.ornek.com", "self-signed"},
		{"le.ornek.com", "le.ornek.com", "Test Yetkilisi R3", "letsencrypt"},
	} {
		crt := filepath.Join(sslDizin, d.ad+".crt")
		key := filepath.Join(sslDizin, d.ad+".key")
		sertifikaYaz(t, crt, key, d.konu, d.veren)

		// GERÇEK fonksiyon çağrılıyor — mantığın kopyasını sınamak, kodda
		// yapılan bir değişikliği yakalamazdı.
		if kaynak := sertifikaKaynagi(crt); kaynak != d.bekleyenKay {
			t.Errorf("%s: kaynak %q, beklenen %q", d.ad, kaynak, d.bekleyenKay)
		}
		_ = key
	}
}

// Sertifika yoksa SSL kapalı sayılmalı; .crt var ama .key yoksa da kapalı —
// nginx yalnız ikisi birdenken TLS sunar (bkz. internal/subdomain/ssl.go).
func TestAltAlanSSLEksikDosya(t *testing.T) {
	if aktif, kaynak := altAlanSSL("c_olmayan_kullanici", "yok.ornek.com"); aktif || kaynak != "" {
		t.Errorf("sertifika yokken aktif=%v kaynak=%q; false/\"\" bekleniyordu", aktif, kaynak)
	}
}

// Okunamayan ya da PEM olmayan sertifikada kaynak BOŞ (bilinmiyor) dönmeli —
// "letsencrypt" varsaymak, self-signed bir sertifikayı yeşil gösterebilirdi.
func TestSertifikaKaynagiBozukDosya(t *testing.T) {
	dizin := t.TempDir()
	bozuk := filepath.Join(dizin, "bozuk.crt")
	if err := os.WriteFile(bozuk, []byte("bu bir sertifika değil"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k := sertifikaKaynagi(bozuk); k != "" {
		t.Errorf("bozuk sertifikada kaynak %q, boş bekleniyordu", k)
	}
	if k := sertifikaKaynagi(filepath.Join(dizin, "yok.crt")); k != "" {
		t.Errorf("olmayan dosyada kaynak %q, boş bekleniyordu", k)
	}
}
