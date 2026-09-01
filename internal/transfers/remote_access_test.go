package transfers

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestHostKeyCozParmakIziniCikarir(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	line := "example.com " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + "\n"
	lines, fps, err := hostKeyCoz([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(fps) != 1 || fps[0] != ssh.FingerprintSHA256(key) {
		t.Fatalf("beklenmeyen çözüm: lines=%v fps=%v", lines, fps)
	}
}

func TestHostKeyCozGecersiziReddeder(t *testing.T) {
	if _, _, err := hostKeyCoz([]byte("host ssh-ed25519 bozuk\n")); err == nil {
		t.Fatal("geçersiz host key kabul edildi")
	}
}
