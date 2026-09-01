package transfers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// archiveFile — test arşivini diske yazar; readSmallTarMembers dosya yolu ister.
func archiveFile(t *testing.T, entries ...testEntry) string {
	t.Helper()
	r := archive(t, entries...)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	yol := filepath.Join(t.TempDir(), "yedek.tar.gz")
	if err := os.WriteFile(yol, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestReadSmallTarMembersTekGecistTumUyeleriBulur(t *testing.T) {
	yol := archiveFile(t,
		testEntry{name: "backup-demo/sslcerts/example.com.crt", body: "CERT"},
		testEntry{name: "backup-demo/sslkeys/example.com.key", body: "KEY"},
		testEntry{name: "backup-demo/va/example.com", body: "sales: dis@example.net\n"},
		testEntry{name: "backup-demo/homedir/public_html/index.php", body: "<?php"},
	)
	got, err := readSmallTarMembers(yol, []string{
		"backup-demo/sslcerts/example.com.crt",
		"backup-demo/sslkeys/example.com.key",
		"backup-demo/va/example.com",
		"backup-demo/yok/olan",
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("beklenen 3 üye, gelen %d: %v", len(got), got)
	}
	if string(got["backup-demo/sslcerts/example.com.crt"]) != "CERT" ||
		string(got["backup-demo/sslkeys/example.com.key"]) != "KEY" {
		t.Fatalf("üye içerikleri hatalı: %v", got)
	}
	if _, ok := got["backup-demo/yok/olan"]; ok {
		t.Fatal("var olmayan üye sonuçta görünmemeli")
	}
}

func TestArsivEkleriSSLveAliasTekGecistetoplanir(t *testing.T) {
	yol := archiveFile(t,
		testEntry{name: "backup-demo/homedir/ssl/certs/example.com.crt", body: "CERT"},
		testEntry{name: "backup-demo/homedir/ssl/private/example.com.key", body: "KEY"},
		testEntry{name: "backup-demo/sslcerts/example.com.cabundle", body: "BUNDLE"},
		testEntry{name: "backup-demo/va/example.com", body: "sales: dis@example.net\nbilgi: info\n"},
	)
	inv := Inventory{ArchiveRoot: "backup-demo", PrimaryDomain: "example.com"}
	ekler, err := okuArsivEkleri(yol, inv)
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := ekler.sslCifti()
	if err != nil {
		t.Fatalf("SSL çifti bulunamadı: %v", err)
	}
	// Zincir: yaprak sertifika + CA bundle birleştirilmiş olmalı.
	if !bytes.Contains(cert, []byte("CERT")) || !bytes.Contains(cert, []byte("BUNDLE")) {
		t.Fatalf("sertifika zinciri eksik: %q", cert)
	}
	if string(key) != "KEY" {
		t.Fatalf("özel anahtar hatalı: %q", key)
	}

	aliases := readAliases(ekler, "example.com", "yeni.com")
	if len(aliases) != 2 {
		t.Fatalf("beklenen 2 alias, gelen %d: %+v", len(aliases), aliases)
	}
	if aliases[0].Local != "sales" || aliases[0].Destination != "dis@example.net" {
		t.Fatalf("alias hatalı: %+v", aliases[0])
	}
	// Hedefte '@' yoksa yeni domain eklenmeli.
	if aliases[1].Destination != "info@yeni.com" {
		t.Fatalf("yerel hedef yeni domaine bağlanmalı: %+v", aliases[1])
	}
}

func TestSSLCiftiEksikAnahtardaBulunamadiDoner(t *testing.T) {
	yol := archiveFile(t,
		testEntry{name: "backup-demo/sslcerts/example.com.crt", body: "CERT"},
	)
	ekler, err := okuArsivEkleri(yol, Inventory{ArchiveRoot: "backup-demo", PrimaryDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ekler.sslCifti(); err == nil {
		t.Fatal("eşleşen özel anahtar yokken SSL çifti dönmemeli")
	}
}

func TestReadSmallTarMembersPrefixUyeleriniToplar(t *testing.T) {
	yol := archiveFile(t,
		testEntry{name: "cpmove-demo/sanalcp/domain.json", body: `{"version":1}`},
		testEntry{name: "cpmove-demo/sanalcp/addons/blog.test/security.json", body: `{"hotlink_enabled":0}`},
		testEntry{name: "cpmove-demo/sanalcp/addons/blog.test/cert.pem", body: "CERT"},
		testEntry{name: "cpmove-demo/homedir/public_html/index.php", body: "IGNORE"},
	)
	got, err := readSmallTarMembersMatching(yol,
		[]string{"cpmove-demo/sanalcp/domain.json"},
		[]string{"cpmove-demo/sanalcp/addons/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || string(got["cpmove-demo/sanalcp/addons/blog.test/cert.pem"]) != "CERT" {
		t.Fatalf("prefix üyeleri eksik veya fazla: %v", got)
	}
	if _, ok := got["cpmove-demo/homedir/public_html/index.php"]; ok {
		t.Fatal("prefix dışındaki web dosyası küçük metadata belleğine alındı")
	}
}
