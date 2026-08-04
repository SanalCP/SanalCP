package mail

import (
	"context"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Dovecot yapılandırma yolları (paket değişkeni — testler geçici dizine yönlendirir).
var (
	dovecotAuthConf    = "/etc/dovecot/conf.d/10-auth.conf"
	dovecotSanalCPConf = "/etc/dovecot/conf.d/10-sanalcp-mail.conf"
)

// rePamInclude: stok Dovecot'un PAM passdb'sini dahil eden AKTİF satır.
// Zaten yorumlanmışsa (`#!include`) eşleşmez → idempotent.
var rePamInclude = regexp.MustCompile(`(?m)^!include auth-system\.conf\.ext\s*$`)

const authCacheBloku = `
# --- SanalCP: kimlik doğrulama önbelleği ---
# Roundcube her HTTP isteğinde YENİ bir IMAP oturumu açar; önbellek olmadan
# her istek yeniden passdb sorgusu demektir.
auth_cache_size = 10M
auth_cache_ttl = 1 hours
auth_cache_negative_ttl = 1 mins
`

// HealDovecotAuth: sanal posta kutusu girişlerini yavaşlatan (ve sistem
// hesaplarını IMAP'e açan) stok PAM passdb'sini devre dışı bırakır ve kimlik
// doğrulama önbelleğini açar.
//
// 🔴 SORUN: Dovecot conf.d dosyalarını ALFABETİK yükler. Stok 10-auth.conf,
// bizim 10-sanalcp-mail.conf'umuzdan ÖNCE gelir ve `!include auth-system.conf.ext`
// ile `passdb { driver = pam }` kaydeder. passdb'ler SIRAYLA denendiği için her
// sanal kutu girişi ÖNCE PAM'a sorulur; kullanıcı sistemde olmadığından
// pam_unix başarısızlık gecikmesi uygular ve giriş ~1.3–2 saniye sürer.
// Canlıda ölçülen: başarılı giriş 1354–2135 ms, başarısız 3.7–8.1 s.
//
// Ayrıca güvenlik: PAM aktifken IMAP üzerinden `root` dahil SİSTEM hesaplarına
// parola denemesi yapılabiliyor. SanalCP'de tüm kutular sanaldır (SQL passdb),
// PAM'ın hiçbir işlevi yoktur.
//
// GÜVENLİK KORUMASI: yalnız SanalCP'nin kendi drop-in'i varsa müdahale edilir.
// Aksi hâlde panel, başka bir amaçla (sistem kullanıcılarına IMAP) kurulmuş bir
// Dovecot'u sessizce bozabilirdi.
func HealDovecotAuth(ctx context.Context) {
	if _, err := os.Stat(dovecotSanalCPConf); err != nil {
		return // SanalCP mail kurulumu yapılmamış — dokunma.
	}

	degisti := false
	if pamKapat() {
		degisti = true
	}
	if authCacheEkle() {
		degisti = true
	}
	if !degisti {
		return
	}

	rctx, iptal := context.WithTimeout(ctx, 20*time.Second)
	defer iptal()
	if out, err := exec.CommandContext(rctx, "systemctl", "reload", "dovecot").CombinedOutput(); err != nil {
		if out2, err2 := exec.CommandContext(rctx, "systemctl", "restart", "dovecot").CombinedOutput(); err2 != nil {
			log.Printf("dovecot auth onarımı: yeniden yüklenemedi: %v: %s", err2, strings.TrimSpace(string(out2)))
			return
		}
		_ = out
	}
	log.Printf("dovecot auth onarımı uygulandı — sanal kutu girişindeki PAM gecikmesi kaldırıldı")
}

// pamKapat: stok PAM include'unu yorum satırına çevirir. Değişiklik yaptıysa true.
func pamKapat() bool {
	b, err := os.ReadFile(dovecotAuthConf)
	if err != nil {
		return false
	}
	if !rePamInclude.Match(b) {
		return false // zaten kapalı
	}
	yeni := rePamInclude.ReplaceAll(b,
		[]byte("#!include auth-system.conf.ext  # SanalCP: kutular sanaldır (SQL passdb); "+
			"PAM her girişe ~2sn gecikme ekliyor ve sistem hesaplarını IMAP'e açıyordu"))
	fi, err := os.Stat(dovecotAuthConf)
	mod := os.FileMode(0o644)
	if err == nil {
		mod = fi.Mode().Perm()
	}
	if err := os.WriteFile(dovecotAuthConf, yeni, mod); err != nil {
		log.Printf("dovecot auth onarımı: %s yazılamadı: %v", dovecotAuthConf, err)
		return false
	}
	return true
}

// authCacheEkle: SanalCP drop-in'ine auth önbelleği ayarlarını ekler.
func authCacheEkle() bool {
	b, err := os.ReadFile(dovecotSanalCPConf)
	if err != nil {
		return false
	}
	if strings.Contains(string(b), "auth_cache_size") {
		return false
	}
	f, err := os.OpenFile(dovecotSanalCPConf, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()
	on := ""
	if n := len(b); n > 0 && b[n-1] != '\n' {
		on = "\n"
	}
	if _, err := f.WriteString(on + authCacheBloku); err != nil {
		log.Printf("dovecot auth onarımı: önbellek eklenemedi: %v", err)
		return false
	}
	return true
}
