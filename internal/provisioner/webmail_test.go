package provisioner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Roundcube kurulu değilse blok ÜRETİLMEMELİ — aksi hâlde her vhost'a 404
// veren ölü bir yol eklenirdi.
func TestWebmailBlokuRoundcubeYoksaBos(t *testing.T) {
	eski := roundcubeKokTest
	roundcubeKokTest = filepath.Join(t.TempDir(), "olmayan")
	defer func() { roundcubeKokTest = eski }()

	if got := webmailBlokuYol(roundcubeKokTest); got != "" {
		t.Errorf("kurulu değilken blok üretildi: %q", got)
	}
}

func TestWebmailBlokuKuruluysaUretir(t *testing.T) {
	dizin := t.TempDir()
	if err := os.MkdirAll(dizin, 0o755); err != nil {
		t.Fatal(err)
	}
	got := webmailBlokuYol(dizin)
	if got == "" {
		t.Fatal("kuruluyken blok üretilmedi")
	}
	for _, beklenen := range []string{
		"location ^~ /webmail/",
		"roundcube.sock",
		"location = /webmail { return 301 /webmail/; }",
		"(config|temp|logs|SQL|bin|tests)",
	} {
		if !strings.Contains(got, beklenen) {
			t.Errorf("blokta %q yok", beklenen)
		}
	}
	// static.php eşleşmesi, uzantı-bazlı statik bloktan ÖNCE gelmeli; aksi
	// hâlde nginx onu var olmayan bir .css/.js dosyası sanıp 404 verir.
	if strings.Index(got, "static\\.php") > strings.Index(got, "jpg|jpeg|gif") {
		t.Error("static.php bloğu statik uzantı bloğundan SONRA geliyor")
	}
}
