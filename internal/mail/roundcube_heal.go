package mail

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"sanalcp/internal/provisioner"
)

// roundcubeConfig: Roundcube ana yapılandırması (paket değişkeni — testler
// geçici bir dosyaya yönlendirir).
var roundcubeConfig = "/opt/roundcube/config/config.inc.php"

// rcSMTPYamasi: eksikse dosyanın SONUNA eklenen blok.
//
// Dosya yalnızca `$config[...] = ...;` atamalarından oluşur ve kapanış `?>`
// etiketi yoktur; sona eklenen atamalar öncekileri geçerli biçimde EZER.
const rcSMTPYamasi = `
// --- SanalCP onarımı: Roundcube 1.5+ seçenek adları + submission STARTTLS ---
// Eski şablon ` + "`smtp_server`/`smtp_port`" + ` yazıyordu; Roundcube 1.7 bu adları
// YOK SAYAR ve varsayılan olan DÜZ localhost:587'ye bağlanır. Postfix submission
// portunu smtpd_tls_security_level=encrypt ile sunduğu için AUTH'u yalnız
// STARTTLS SONRASI duyurur → istemci AUTH göremez ve "Kimlik doğrulanamadı" der.
$config['imap_host'] = 'localhost:143';
$config['smtp_host'] = 'tls://localhost:587';
$config['smtp_user'] = '%u';
$config['smtp_pass'] = '%p';
// Loopback'te sertifika ADI uyuşmaz (sertifika sunucu adına, bağlantı
// localhost'a); trafik makineden çıkmadığı için ad doğrulaması gevşetilir,
// şifreleme korunur — zaten AUTH'un ön koşuludur.
$config['smtp_conn_options'] = [
    'ssl' => [
        'verify_peer'       => false,
        'verify_peer_name'  => false,
        'allow_self_signed' => true,
    ],
];
`

// HealRoundcubeSMTP: webmail'in e-posta GÖNDEREMEMESİNE yol açan eksik
// `smtp_host` ayarını yerinde onarır. Idempotenttir; ayar zaten varsa dokunmaz.
//
// 🔴 NEDEN PANEL AÇILIŞINDA (sanalcp-update içinde DEĞİL):
// sanalcp-update kendini de günceller, ama çalışan kopya ESKİ script'tir.
// Onarım oraya konursa ancak BİR SONRAKİ güncellemede devreye girer — canlıda
// tam olarak bu yaşandı: yeni script diske yazıldı, config yamalanmadı.
// Panel her yeniden başlatmada bu fonksiyonu çağırdığı için onarım
// güncellemenin hemen ardından uygulanır.
func HealRoundcubeSMTP(ctx context.Context) {
	icerik, err := os.ReadFile(roundcubeConfig)
	if err != nil {
		return // Roundcube kurulu değil — sessiz geç.
	}
	if strings.Contains(string(icerik), "smtp_host") {
		return // zaten onarılmış / doğru şablonla yazılmış
	}

	f, err := os.OpenFile(roundcubeConfig, os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		log.Printf("roundcube smtp onarımı: dosya açılamadı: %v", err)
		return
	}
	// Dosya newline ile bitmiyorsa satırlar birleşmesin.
	on := ""
	if n := len(icerik); n > 0 && icerik[n-1] != '\n' {
		on = "\n"
	}
	if _, err := f.WriteString(on + rcSMTPYamasi); err != nil {
		f.Close()
		log.Printf("roundcube smtp onarımı: yazılamadı: %v", err)
		return
	}
	if err := f.Close(); err != nil {
		log.Printf("roundcube smtp onarımı: kapatılamadı: %v", err)
		return
	}

	// php-fpm opcache'i eski config'i tutabilir; reload en iyi çabadır.
	rctx, iptal := context.WithTimeout(ctx, 15*time.Second)
	defer iptal()
	fpm := provisioner.SistemPHPServis() // RHEL "php-fpm" · Debian "php8.3-fpm"
	if out, err := exec.CommandContext(rctx, "systemctl", "reload", fpm).CombinedOutput(); err != nil {
		_, _ = exec.CommandContext(rctx, "systemctl", "restart", fpm).CombinedOutput()
		_ = out
	}
	log.Printf("roundcube smtp onarımı uygulandı (%s) — webmail giden e-posta kimlik doğrulaması düzeltildi", roundcubeConfig)
}
