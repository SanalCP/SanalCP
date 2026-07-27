package provisioner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

func TestEnableImportedSSLRejectsWrongHostname(t *testing.T) {
	certPEM, keyPEM := testCertificate(t, "source.example", time.Now().Add(24*time.Hour))
	_, _, _, err := InstallImportedSSL("target.example", certPEM, keyPEM)
	if !errors.Is(err, ErrImportedSSLInvalid) {
		t.Fatalf("want ErrImportedSSLInvalid, got %v", err)
	}
}

func TestEnableImportedSSLRejectsExpiredCertificate(t *testing.T) {
	certPEM, keyPEM := testCertificate(t, "expired.example", time.Now().Add(-time.Hour))
	_, _, _, err := InstallImportedSSL("expired.example", certPEM, keyPEM)
	if !errors.Is(err, ErrImportedSSLInvalid) {
		t.Fatalf("want ErrImportedSSLInvalid, got %v", err)
	}
}

func testCertificate(t *testing.T, domain string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}
