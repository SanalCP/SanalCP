package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gecikiConfig(t *testing.T, icerik string) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), "config.inc.php")
	if err := os.WriteFile(yol, []byte(icerik), 0o640); err != nil {
		t.Fatal(err)
	}
	eski := roundcubeConfig
	roundcubeConfig = yol
	t.Cleanup(func() { roundcubeConfig = eski })
	return yol
}

// Eski şablonla yazılmış config onarılmalı: smtp_host eklenmiş olmalı ve
// eski satırlar KORUNMALI (onlar zaten yok sayılıyor, silmek gereksiz risk).
func TestHealRoundcubeSMTPEksikOlaniEkler(t *testing.T) {
	eski := "<?php\n$config = [];\n$config['smtp_server']  = 'localhost';\n$config['smtp_port']    = 587;\n"
	yol := gecikiConfig(t, eski)

	HealRoundcubeSMTP(context.Background())

	b, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, beklenen := range []string{
		"$config['smtp_host'] = 'tls://localhost:587';",
		"$config['imap_host'] = 'localhost:143';",
		"verify_peer_name",
	} {
		if !strings.Contains(s, beklenen) {
			t.Errorf("%q eklenmedi:\n%s", beklenen, s)
		}
	}
	if !strings.HasPrefix(s, eski) {
		t.Error("mevcut içerik korunmadı")
	}
	// Yama, eski atamalardan SONRA gelmeli ki onları ezebilsin.
	if strings.Index(s, "smtp_host") < strings.Index(s, "smtp_server") {
		t.Error("yama eski atamaların önüne yazıldı — ezme çalışmaz")
	}
}

// Idempotent: ikinci çağrı hiçbir şey eklememeli.
func TestHealRoundcubeSMTPIdempotent(t *testing.T) {
	yol := gecikiConfig(t, "<?php\n$config['smtp_server'] = 'localhost';\n")

	HealRoundcubeSMTP(context.Background())
	ilk, _ := os.ReadFile(yol)
	HealRoundcubeSMTP(context.Background())
	ikinci, _ := os.ReadFile(yol)

	if string(ilk) != string(ikinci) {
		t.Errorf("ikinci çağrı dosyayı değiştirdi (%d → %d bayt)", len(ilk), len(ikinci))
	}
	if n := strings.Count(string(ikinci), "smtp_host"); n != 1 {
		t.Errorf("smtp_host %d kez yazıldı, 1 olmalıydı", n)
	}
}

// Doğru şablonla kurulmuş yeni sistemlere DOKUNULMAMALI.
func TestHealRoundcubeSMTPDogruConfigeDokunmaz(t *testing.T) {
	dogru := "<?php\n$config['smtp_host'] = 'tls://localhost:587';\n"
	yol := gecikiConfig(t, dogru)

	HealRoundcubeSMTP(context.Background())

	b, _ := os.ReadFile(yol)
	if string(b) != dogru {
		t.Errorf("zaten doğru olan config değiştirildi:\n%s", b)
	}
}

// Roundcube kurulu değilse sessizce geçmeli (panel açılışını bozmamalı).
func TestHealRoundcubeSMTPDosyaYoksaSessiz(t *testing.T) {
	eski := roundcubeConfig
	roundcubeConfig = filepath.Join(t.TempDir(), "olmayan", "config.inc.php")
	defer func() { roundcubeConfig = eski }()

	HealRoundcubeSMTP(context.Background()) // panik/hata olmamalı
}

// Dosya newline ile bitmiyorsa son satır yamayla birleşmemeli.
func TestHealRoundcubeSMTPSonSatirBirlesmez(t *testing.T) {
	yol := gecikiConfig(t, "<?php\n$config['smtp_server'] = 'localhost';")

	HealRoundcubeSMTP(context.Background())

	b, _ := os.ReadFile(yol)
	if strings.Contains(string(b), "'localhost';//") || strings.Contains(string(b), "'localhost';\n// ---") == false {
		if !strings.Contains(string(b), "'localhost';\n") {
			t.Errorf("son satır yamayla birleşti:\n%s", b)
		}
	}
}
