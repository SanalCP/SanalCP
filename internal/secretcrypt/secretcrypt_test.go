package secretcrypt

import (
	"crypto/rand"
	"testing"
)

func testBox(t *testing.T) *Box {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	b := testBox(t)
	plain := "Sup3rSecretDbPassw0rd!"
	enc, err := b.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("şifreli çıktı 'v1:' önekiyle başlamıyor: %q", enc)
	}
	if enc == plain {
		t.Fatal("çıktı düz metinle aynı")
	}
	dec, err := b.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Fatalf("round-trip uyuşmadı: got %q want %q", dec, plain)
	}
}

func TestDecryptRejectsPlaintext(t *testing.T) {
	b := testBox(t)
	if _, err := b.Decrypt("düz-metin-eski-satır"); err != ErrUnknownFormat {
		t.Fatalf("beklenen ErrUnknownFormat, alınan: %v", err)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	b1 := testBox(t)
	b2 := testBox(t)
	enc, err := b1.Encrypt("gizli")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.Decrypt(enc); err == nil {
		t.Fatal("farklı anahtarla çözme başarılı olmamalıydı")
	}
}

func TestEncryptNonceUniqueness(t *testing.T) {
	b := testBox(t)
	a, err := b.Encrypt("aynı-parola")
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Encrypt("aynı-parola")
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatal("aynı düz metin için aynı şifreli çıktı üretildi (nonce tekrarı)")
	}
}
