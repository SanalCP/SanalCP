package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestGecerliWebhookImzasi(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	imza := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !gecerliWebhookImzasi(secret, body, imza) {
		t.Fatal("doğru webhook imzası reddedildi")
	}
	if gecerliWebhookImzasi(secret, []byte(`{"ref":"degisti"}`), imza) {
		t.Fatal("değiştirilmiş gövde kabul edildi")
	}
	if gecerliWebhookImzasi(secret, body, "sha256=bozuk") {
		t.Fatal("bozuk imza kabul edildi")
	}
	if gecerliWebhookImzasi([]byte("kisa"), body, imza) {
		t.Fatal("kısa secret kabul edildi")
	}
}
