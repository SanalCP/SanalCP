package provisioner

import (
	"log"
	"os"
	"os/exec"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/nginxconf"
)

// healProxyAnahtarOnStartup: nginx → backend proxy anahtarını hazırlar ve
// httpx'e bildirir (bkz. internal/nginxconf/proxyanahtar.go). Map dosyası
// yeni yazıldıysa ve panel vhost'u zaten anahtarı kullanıyorsa nginx yeniden
// yüklenir; kullanmıyorsa ardından gelen HealPanelVhostOnStartup yükler.
func healProxyAnahtarOnStartup() {
	anahtar, degisti, err := nginxconf.ProxyAnahtariHazirla()
	if err != nil {
		log.Printf("proxy anahtarı hazırlanamadı — loopback istemci-IP başlıklarına güvenilmeyecek: %v", err)
		return
	}
	httpx.SetProxyAnahtari(anahtar)
	if !degisti {
		return
	}
	mevcut, err := os.ReadFile(panelVhostPath)
	if err != nil || !strings.Contains(string(mevcut), nginxconf.ProxyAnahtarBaslik) {
		return
	}
	if out, e := exec.Command("nginx", "-t").CombinedOutput(); e != nil {
		log.Printf("proxy anahtarı: nginx -t başarısız: %s", strings.TrimSpace(string(out)))
		return
	}
	if out, e := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); e != nil {
		log.Printf("proxy anahtarı: nginx reload başarısız: %s", strings.TrimSpace(string(out)))
	}
}

// proxyAnahtarGuveniniBelirle: panel vhost'u heal'den SONRA hâlâ anahtar
// göndermiyorsa (yönetici dosyayı elle düzenlemiş, heal üzerine yazmamış),
// backend eski davranışa döner. Aksi hâlde nginx'ten gelen her istek
// 127.0.0.1 görünür: IP izin listesi açıksa yönetici panelin dışında kalır,
// giriş sınırı da tüm istemcileri tek kovada toplar. Bu geçiş modu güvenli
// DEĞİLDİR; yönetici conf'u birleştirene kadar her açılışta uyarı basılır.
func proxyAnahtarGuveniniBelirle() {
	mevcut, err := os.ReadFile(panelVhostPath)
	if err != nil {
		return // panel vhost yok — bu host'ta nginx önünde panel yok
	}
	if strings.Contains(string(mevcut), nginxconf.ProxyAnahtarBaslik) {
		httpx.SetLoopbackAnahtarsizGuven(false)
		return
	}
	httpx.SetLoopbackAnahtarsizGuven(true)
	log.Printf("🔴 GÜVENLİK UYARISI: %s elle düzenlenmiş ve %s başlığını göndermiyor. "+
		"Panel geçiş modunda: loopback'ten gelen X-Forwarded-For anahtarsız kabul ediliyor, "+
		"yani sunucudaki tenant süreçleri panel IP izin listesini ve giriş sınırını atlatabilir. "+
		"Dosyayı internal/nginxconf/_panel.conf ile birleştirip paneli yeniden başlatın.",
		panelVhostPath, nginxconf.ProxyAnahtarBaslik)
}
