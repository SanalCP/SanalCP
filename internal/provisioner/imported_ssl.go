package provisioner

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrImportedSSLInvalid, kaynak sertifika/anahtar çiftinin güvenle
// kullanılamadığını belirtir. Aktarım bu durumda hesabı SSL'siz bırakabilir.
var ErrImportedSSLInvalid = errors.New("geçersiz kaynak SSL sertifikası")

// InstallImportedSSL doğrulanmış bir PEM sertifika zinciri ve özel anahtarı
// sistem sertifika dizinine kurar. Çağıran önce DB'yi güncelleyip ardından
// RerenderVhost çağırır; böylece eşzamanlı vhost yenilemeleri SSL'yi ezemez.
func InstallImportedSSL(alanAdi string, certPEM, keyPEM []byte) (string, string, time.Time, error) {
	if err := ValidateDomain(alanAdi); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: %v", ErrImportedSSLInvalid, err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika ve özel anahtar eşleşmiyor", ErrImportedSSLInvalid)
	}
	if len(pair.Certificate) == 0 {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika bulunamadı", ErrImportedSSLInvalid)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika okunamadı", ErrImportedSSLInvalid)
	}
	now := time.Now()
	if leaf.NotBefore.After(now.Add(5 * time.Minute)) {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika henüz geçerli değil", ErrImportedSSLInvalid)
	}
	if !leaf.NotAfter.After(now) {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifikanın süresi dolmuş", ErrImportedSSLInvalid)
	}
	if err := leaf.VerifyHostname(alanAdi); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika %s alan adını kapsamıyor", ErrImportedSSLInvalid, alanAdi)
	}
	if len(leaf.ExtKeyUsage) > 0 && !hasServerAuth(leaf.ExtKeyUsage) {
		return "", "", time.Time{}, fmt.Errorf("%w: sertifika sunucu kimlik doğrulaması için uygun değil", ErrImportedSSLInvalid)
	}

	sslDir := certSystemDir(alanAdi)
	if err := os.MkdirAll(sslDir, 0o755); err != nil {
		return "", "", time.Time{}, err
	}
	certPath := filepath.Join(sslDir, alanAdi+".crt")
	keyPath := filepath.Join(sslDir, alanAdi+".key")
	if err := writeImportedPEM(certPath, certPEM, 0o644); err != nil {
		return "", "", time.Time{}, err
	}
	if err := writeImportedPEM(keyPath, keyPEM, 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", "", time.Time{}, err
	}
	yazCertKurulumu(sslDir, certPath, keyPath)
	return certPath, keyPath, leaf.NotAfter, nil
}

func hasServerAuth(usages []x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}

func writeImportedPEM(target string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".sanalpanel-import-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}
