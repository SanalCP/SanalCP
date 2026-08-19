package osfam

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 🔴 `komut | grep -q desen` + `set -o pipefail` = SESSİZ YANLIŞ-OLUMSUZ.
//
// grep -q eşleşmeyi bulduğu ANDA çıkar; bu boruyu kapatır, üretici SIGPIPE alır
// ve boru hattı 141 döner — eşleşme BAŞARILI olsa bile. Üretici ne kadar çok
// yazarsa risk o kadar büyük, yani aynı kod bir dağıtımda geçip diğerinde
// düşebiliyor (çıktı uzunluğu değişince yarış çevriliyor).
//
// Bu kod tabanında ÜÇ KEZ yaşandı:
//   1. `quotaon | grep` → kota kapalı sanıldı (internal/kaynaklimit/kota_ext4.go)
//   2. `ss -lntp | grep -q ...:8891` → OpenDKIM dinlemiyor sanıldı (kabul testi)
//   3. `apt-cache policy | grep -q sury.org` → Ubuntu 24.04 kurulumu adım 1'de
//      öldü ve DOĞRU olan sources.list dosyasını suçladı
//
// Doğrusu: çıktıyı değişkene al, bash'in `=~`/`case`'i ile eşleştir; ya da
// sanalcp-ortak.sh:cikti_esler yardımcısını kullan.
//
// İstisna: üretici bir shell builtin'i (echo/printf) ya da anında bitip çıkan
// küçük bir komut (head -c N) ise SIGPIPE oluşmaz. Bu satırlar `# sigpipe-ok`
// yorumuyla işaretlenmelidir — böylece karar gözden geçirmede BİLİNÇLİ olur.
func TestPipefailBetiklerindeGrepQBoruHattiYok(t *testing.T) {
	kok, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	hedefler := []string{
		"sanalcp-install.sh",
		"scripts/debian-kabul-testi.sh",
		"assets/ops/sanalcp-ortak.sh",
		"assets/ops/sanalcp-mail-setup.sh",
		"assets/ops/sanalcp-redis-setup.sh",
		"assets/ops/sanalcp-optimize.sh",
		"assets/ops/sanalcp-wp-redis.sh",
		"assets/ops/sanalcp-ftp-setup",
		"assets/ops/sanalcp-repair",
		"assets/ops/sanalcp-restore",
		"assets/ops/sanalcp-update",
		"assets/ops/sanalcp-waf-setup",
		"assets/ops/sanalcp-jail",
		"assets/ops/sanalcp-backup-all",
		"assets/ops/sanalcp-db-backup",
	}
	// `| grep -q` (veya -qE/-qF/-qw...) biçimindeki her boru hattı.
	tehlikeli := regexp.MustCompile(`\|\s*grep\s+-[a-zA-Z]*q`)

	for _, rel := range hedefler {
		yol := filepath.Join(kok, rel)
		ham, err := os.ReadFile(yol)
		if err != nil {
			// Dosya yoksa atla: hedef listesi zamanla değişir, test kırılmasın.
			continue
		}
		icerik := string(ham)
		// Yalnız pipefail ile koşan betikler ilgilendiriyor.
		if !strings.Contains(icerik, "pipefail") {
			continue
		}
		for i, satir := range strings.Split(icerik, "\n") {
			if !tehlikeli.MatchString(satir) {
				continue
			}
			// Yorum satırları (bu tuzağı ANLATAN metinler dahil) kod değil.
			if strings.HasPrefix(strings.TrimSpace(satir), "#") {
				continue
			}
			// Bilinçli olarak güvenli işaretlenmiş satırlar serbest.
			if strings.Contains(satir, "sigpipe-ok") {
				continue
			}
			t.Errorf("%s:%d — pipefail altında `| grep -q`: eşleşmede SIGPIPE (141) döner, "+
				"kontrol sessizce yanlış-olumsuz olur. Çıktıyı değişkene al (cikti_esler) "+
				"ya da üretici gerçekten builtin/anında bitiyorsa satırı `# sigpipe-ok` ile işaretle.\n  %s",
				rel, i+1, strings.TrimSpace(satir))
		}
	}
}
