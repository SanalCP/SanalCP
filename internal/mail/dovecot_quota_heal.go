package mail

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Posta kutusu kotasının MEVCUT kurulumlara onarımı.
//
// 🔴 NEDEN PANEL AÇILIŞINDA: sanalcp-mail-setup yeni kurulumda kota yapılandırmasını
// zaten yazar, ama güncelleme yalnız yeni panel binary'sini + migration'ları
// getirir; kurulu bir sunucuda setup yeniden çalışmaz. Aynı ders HealDovecotAuth
// için de öğrenildi.
//
// Onarım iki parçadır ve İKİSİ BİRLİKTE uygulanır ya da hiçbiri:
//  1. userdb sorgusuna kutunun limiti eklenir (2.3: dovecot-sql.conf.ext
//     `quota_rule`; 2.4: 10-sanalcp-mail.conf `quota_storage_size`).
//  2. Kota eklentisini açan YENİ drop-in (95-sanalcp-quota.conf) yazılır —
//     stok dosyalara dokunulmaz.
//
// Güvenlik ağı: yapılandırma `doveconf -n` ile doğrulanır; geçmezse iki dosya da
// eski haline döner ve Dovecot'a dokunulmaz. Başarıda `reload` (restart değil)
// yapılır: bağlı IMAP oturumları kopmaz.

// Kota drop-in'inin sürüme göre içeriği. assets/mail/dovecot/95-sanalcp-quota*.tmpl
// dosyalarıyla BİREBİR aynı olmalıdır (kurulum betiği şablonu, panel bu sabiti
// yazar; sapmayı TestKotaDropinSabitleriSablonlaAyni yakalar).
const dovecotKota23 = `# SanalCP — posta kutusu kotası (kutu başına depolama sınırı) · Dovecot 2.3.
#
# Limit BU dosyada değil userdb'den gelir: dovecot-sql.conf.ext içindeki
# user_query her kutunun mailboxes.quota_bytes değerini 'quota_rule' alanı
# olarak döndürür ('*:bytes=<bayt>'; 0 = sınırsız). Kotayı panelden değiştirmek
# bu yüzden Dovecot'u yeniden başlatmayı gerektirmez.
#
# Kullanım Maildir++ 'maildirsize' dosyasında tutulur: panel kutu listesinde
# kullanımı dizin gezmeden buradan okur.
#
# 🔴 NEDEN 'mail_plugins' HER PROTOKOLE AYRI YAZILIYOR, global DEĞİL:
# 10-sanalcp-mail.conf, 'protocol lmtp { mail_plugins = $mail_plugins sieve }'
# bloğunu bu dosyadan ÖNCE açar. Sonradan yazılan global 'mail_plugins', daha
# önce açılmış protokol bloğunun içini DEĞİŞTİRMEZ (doveconf: "Global setting
# mail_plugins won't change the setting inside an earlier filter") — LMTP'de
# kota eklentisi hiç yüklenmez, teslimat kotayı sessizce aşardı. Blokları
# burada tekrar açmak mevcut değerin ($mail_plugins) SONUNA ekler.
protocol imap {
  mail_plugins = $mail_plugins quota imap_quota
}
protocol lmtp {
  mail_plugins = $mail_plugins quota
  # Kota doluysa alıcıyı RCPT aşamasında reddet: teslimat sessizce kabul
  # edilip sonra düşmez, gönderene "552 5.2.2 Quota exceeded" döner.
  lmtp_rcpt_check_quota = yes
}
# 'doveadm quota get/recalc' için.
protocol doveadm {
  mail_plugins = $mail_plugins quota
}
plugin {
  quota = maildir:User quota
  # Tolerans yok: limit, panelde gösterilen değerle birebir aynıdır.
  # (Varsayılan %10 pay, küçük kutularda "limit aşıldı" göstergesini yanıltırdı.)
  quota_grace = 0
}
`

const dovecotKota24 = `# SanalCP — posta kutusu kotası (kutu başına depolama sınırı) · **Dovecot 2.4 sözdizimi**.
#
# Limit BU dosyada değil userdb'den gelir: 10-sanalcp-mail.conf içindeki
# userdb sorgusu her kutunun mailboxes.quota_bytes değerini 'quota_storage_size'
# alanı olarak döndürür ('<bayt>B'; 0B = sınırsız). Kotayı panelden değiştirmek
# bu yüzden Dovecot'u yeniden başlatmayı gerektirmez.
#
# 🔴 SÜRÜCÜ 'maildir' (2.4'te önerilen 'count' DEĞİL): Maildir++ 'maildirsize'
# dosyası kullanımı kutu kökünde tutar; panel kutu listesinde kullanımı dizin
# gezmeden ve Dovecot'u çağırmadan buradan okur. 2.4'te dosyanın başlığındaki
# limit yok sayılır, limit yalnız userdb'den gelir.
mail_plugins {
  quota = yes
}
protocol imap {
  mail_plugins {
    imap_quota = yes
  }
}
protocol lmtp {
  # Kota doluysa alıcıyı RCPT aşamasında reddet: teslimat sessizce kabul
  # edilip sonra düşmez, gönderene "552 5.2.2 Quota exceeded" döner.
  lmtp_rcpt_check_quota = yes
}
quota "User quota" {
  driver = maildir
  # Tolerans yok: limit, panelde gösterilen değerle birebir aynıdır.
  # (2.4 varsayılanı 10 MB pay — küçük kutularda limiti anlamsızlaştırırdı.)
  storage_grace = 0
}
`

var (
	dovecotKotaConf = "/etc/dovecot/conf.d/95-sanalcp-quota.conf"
	dovecotSQLConf  = "/etc/dovecot/dovecot-sql.conf.ext"
)

// dovecotKomut: doveconf'u çalıştırır (paket değişkeni — testler Dovecot'suz çalışır).
var dovecotKomut = func(ctx context.Context, arglar ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "doveconf", arglar...).CombinedOutput()
}

// dovecotYenile: `systemctl reload dovecot` (paket değişkeni — testler değiştirir).
var dovecotYenile = func(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "systemctl", "reload", "dovecot").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	reMailDriver = regexp.MustCompile(`(?m)^\s*mail_driver\s*=`)
	reMailLoc    = regexp.MustCompile(`(?m)^\s*mail_location\s*=`)
	// 2.3: tek satırlık user_query (kutu tablosundan okuyan).
	reUserQuery23 = regexp.MustCompile(`(?m)^(user_query\s*=\s*SELECT\b.*\S)(\s+FROM\s+mailboxes\s+m\s+JOIN\b.*)$`)
	// 2.4: userdb sql içindeki sorgu — passdb sorgusundan `AS home` ile ayrılır.
	reUserQuery24 = regexp.MustCompile(`(?m)^(\s*query\s*=\s*SELECT\b.*\bAS\s+home\b.*\S)(\s+FROM\s+mailboxes\s+m\s+JOIN\b.*)$`)
)

type dovecotSurum int

const (
	dovecotBilinmiyor dovecotSurum = iota
	dovecot23
	dovecot24
)

// dovecotSurumunuBul: kurulu SanalCP drop-in'inin sözdiziminden sürümü çıkarır
// (şablon testleri iki dilin karışmadığını sabitler).
func dovecotSurumunuBul(anaConf string) dovecotSurum {
	switch {
	case reMailDriver.MatchString(anaConf):
		return dovecot24
	case reMailLoc.MatchString(anaConf):
		return dovecot23
	}
	return dovecotBilinmiyor
}

// kotaSorgusuEkle: userdb sorgusuna kota alanını ekler.
//   - degisti: içerik değişti (yazılmalı)
//   - tamam: sorgu artık kota alanını taşıyor (zaten vardı ya da eklendi);
//     false → sorgu tanınmadı (elle özelleştirilmiş), dokunulmadı.
func kotaSorgusuEkle(s dovecotSurum, icerik string) (yeni string, degisti, tamam bool) {
	re, alan, ek := reUserQuery23, "quota_rule", ", CONCAT('*:bytes=',m.quota_bytes) AS quota_rule"
	if s == dovecot24 {
		re, alan, ek = reUserQuery24, "quota_storage_size", ", CONCAT(m.quota_bytes,'B') AS quota_storage_size"
	}
	m := re.FindStringSubmatchIndex(icerik)
	if m == nil {
		return icerik, false, false
	}
	if strings.Contains(icerik[m[0]:m[1]], alan) {
		return icerik, false, true
	}
	// m[3] = grup 1 sonu: alan "FROM"dan hemen önce eklenir.
	return icerik[:m[3]] + ek + icerik[m[3]:], true, true
}

// kotaEklentileriVar: Dovecot'un kota eklentileri diskte mi? Eksik eklenti
// yapılandırmada GÖRÜNMEZ (doveconf geçer) ama her IMAP oturumu açılışta
// düşer; bu yüzden yazmadan önce bakılır.
func kotaEklentileriVar(ctx context.Context) bool {
	out, err := dovecotKomut(ctx, "-h", "mail_plugin_dir")
	if err != nil {
		return false
	}
	dizin := strings.TrimSpace(string(out))
	if dizin == "" {
		return false
	}
	for _, kalip := range []string{"lib??_quota_plugin.so", "lib??_imap_quota_plugin.so"} {
		if es, _ := filepath.Glob(filepath.Join(dizin, kalip)); len(es) == 0 {
			return false
		}
	}
	return true
}

// yazAtomik: aynı dizinde geçici dosya + rename; mevcut dosyanın izin ve
// sahibini korur (ör. DB parolası içeren root:dovecot 0640).
func yazAtomik(yol string, veri []byte, varsayilanMod os.FileMode) error {
	mod := varsayilanMod
	uid, gid := -1, -1
	if fi, err := os.Stat(yol); err == nil {
		mod = fi.Mode().Perm()
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(st.Uid), int(st.Gid)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(yol), ".sanalcp-kota-*")
	if err != nil {
		return err
	}
	tmpAd := tmp.Name()
	defer os.Remove(tmpAd)
	if _, err := tmp.Write(veri); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpAd, mod); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Chown(tmpAd, uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmpAd, yol)
}

// HealDovecotQuota: bkz. dosya başlığı. İdempotent; asla fatal değil.
func HealDovecotQuota(ctx context.Context) {
	ana, err := os.ReadFile(dovecotSanalCPConf)
	if err != nil {
		return // SanalCP mail kurulumu yok — dokunma.
	}
	surum := dovecotSurumunuBul(string(ana))
	if surum == dovecotBilinmiyor {
		log.Printf("dovecot kota onarımı: %s sürümü tanınmadı, atlandı", dovecotSanalCPConf)
		return
	}

	sqlYol, dropin := dovecotSQLConf, dovecotKota23
	if surum == dovecot24 {
		sqlYol, dropin = dovecotSanalCPConf, dovecotKota24
	}
	sqlEski, err := os.ReadFile(sqlYol)
	if err != nil {
		log.Printf("dovecot kota onarımı: %s okunamadı: %v", sqlYol, err)
		return
	}
	sqlYeni, sqlDegisti, tamam := kotaSorgusuEkle(surum, string(sqlEski))
	if !tamam {
		log.Printf("dovecot kota onarımı: %s içindeki userdb sorgusu tanınmadı (elle özelleştirilmiş?) — kota uygulanmadı", sqlYol)
		return
	}
	_, dropinErr := os.Stat(dovecotKotaConf)
	dropinVar := dropinErr == nil
	if !sqlDegisti && dropinVar {
		return // zaten uygulanmış
	}

	rctx, iptal := context.WithTimeout(ctx, 40*time.Second)
	defer iptal()
	if !kotaEklentileriVar(rctx) {
		log.Printf("dovecot kota onarımı: kota eklentileri (quota/imap_quota) bulunamadı — kota uygulanmadı")
		return
	}

	geriAl := func() {
		if sqlDegisti {
			if err := yazAtomik(sqlYol, sqlEski, 0o640); err != nil {
				log.Printf("dovecot kota onarımı: %s eski haline döndürülemedi: %v", sqlYol, err)
			}
		}
		if !dropinVar {
			_ = os.Remove(dovecotKotaConf)
		}
	}
	if sqlDegisti {
		if err := yazAtomik(sqlYol, []byte(sqlYeni), 0o640); err != nil {
			log.Printf("dovecot kota onarımı: %s yazılamadı: %v", sqlYol, err)
			return
		}
	}
	if !dropinVar {
		if err := yazAtomik(dovecotKotaConf, []byte(dropin), 0o644); err != nil {
			log.Printf("dovecot kota onarımı: %s yazılamadı: %v", dovecotKotaConf, err)
			geriAl()
			return
		}
	}

	if out, err := dovecotKomut(rctx, "-n"); err != nil {
		ilk, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		log.Printf("dovecot kota onarımı: yapılandırma doğrulanamadı, geri alındı: %v: %s", err, ilk)
		geriAl()
		return
	}
	if err := dovecotYenile(rctx); err != nil {
		// Yapılandırma geçerli; servis çalışmıyorsa bir sonraki başlatma kotayı yükler.
		log.Printf("dovecot kota onarımı: reload başarısız (yapılandırma geçerli, korundu): %v", err)
		return
	}
	log.Printf("dovecot kota onarımı uygulandı — kutu kotası artık Dovecot tarafından uygulanıyor")
}
