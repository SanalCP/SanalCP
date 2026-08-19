package osfam

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SHA256SUMS, assets/ ağacının TAMAMINI kapsamalı.
//
// 🔴 NEDEN: installer ve sanalcp-update indirdikleri release'i
// `sha256sum -c assets/SHA256SUMS` ile doğrular. Manifest bayatsa kurulum ve
// güncelleme "release asset SHA-256 doğrulaması başarısız" ile DURUR — yani
// unutulmuş bir `package-release.sh` çalıştırması tüm sunucularda güncellemeyi
// kırar. `sha256sum -c` yalnız LİSTELENEN dosyaları kontrol eder; manifeste hiç
// girmemiş yeni bir dosyayı FARK ETMEZ. Bu test o boşluğu kapatır.
//
// Faz 4b'de ve Faz 5b hazırlığında iki kez tam olarak bu oldu.
func TestAssetManifestiTumDosyalariKapsar(t *testing.T) {
	kok, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(kok, "assets", "SHA256SUMS")
	f, err := os.Open(manifest)
	if err != nil {
		t.Skipf("manifest okunamadı: %v", err)
	}
	defer f.Close()

	listelenen := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		// "<sha>  assets/yol" biçimi
		if _, yol, ok := strings.Cut(s.Text(), "  "); ok {
			listelenen[strings.TrimSpace(yol)] = true
		}
	}

	var eksik []string
	err = filepath.Walk(filepath.Join(kok, "assets"), func(yol string, bilgi os.FileInfo, err error) error {
		if err != nil || bilgi.IsDir() {
			return err
		}
		gorece, rerr := filepath.Rel(kok, yol)
		if rerr != nil {
			return rerr
		}
		if gorece == filepath.Join("assets", "SHA256SUMS") {
			return nil
		}
		if !listelenen[gorece] {
			eksik = append(eksik, gorece)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(eksik) > 0 {
		t.Errorf("assets/SHA256SUMS bu dosyaları KAPSAMIYOR — installer/update bütünlük "+
			"kontrolü eksik kalır, `scripts/package-release.sh` çalıştırılmamış olabilir:\n  %s",
			strings.Join(eksik, "\n  "))
	}
}
