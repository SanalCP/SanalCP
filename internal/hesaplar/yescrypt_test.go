package hesaplar

import (
	"strings"
	"testing"

	yescrypt "github.com/openwall/yescrypt-go"
)

func TestYescryptHashFTPRoundTrip(t *testing.T) {
	pw := "Ftp-Parola-123"
	hash, err := yescryptHashFTP(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !isYescryptHash(hash) {
		t.Fatalf("üretilen hash $y$j ile başlamıyor: %q", hash)
	}
	if !strings.HasPrefix(hash, "$y$j9T$") {
		t.Fatalf("beklenen sabit ayar $y$j9T$, alınan: %q", hash)
	}
	// chpasswd -e / Pure-FTPd MYSQLCrypt=crypt'in yaptığı doğrulamayla aynı yol:
	// hash'in kendisini "setting" olarak vererek aynı hash'in üretilip
	// üretilmediğine bakılır (crypt(3) semantiği).
	verify, err := yescrypt.Hash([]byte(pw), []byte(hash))
	if err != nil {
		t.Fatal(err)
	}
	if string(verify) != hash {
		t.Fatalf("doğrulama uyuşmadı: got %q want %q", verify, hash)
	}
	// Yanlış parola aynı hash'i üretmemeli.
	wrong, err := yescrypt.Hash([]byte("başka-parola"), []byte(hash))
	if err != nil {
		t.Fatal(err)
	}
	if string(wrong) == hash {
		t.Fatal("yanlış parola doğrulamayı geçti")
	}
}

func TestYescryptHashFTPUnique(t *testing.T) {
	a, err := yescryptHashFTP("aynı-parola")
	if err != nil {
		t.Fatal(err)
	}
	b, err := yescryptHashFTP("aynı-parola")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("aynı parola için aynı hash üretildi (salt tekrarı)")
	}
}

func TestIsYescryptHash(t *testing.T) {
	if isYescryptHash("düz-metin-eski-parola") {
		t.Fatal("düz metin yescrypt hash'i olarak tanındı")
	}
	if !isYescryptHash("$y$j9T$abc$def") {
		t.Fatal("geçerli $y$j önekli değer tanınmadı")
	}
}
