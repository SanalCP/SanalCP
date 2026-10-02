package nginxconf

// 🔴 GÜVENLİK: panel backend'i (127.0.0.1:8080) loopback'ten gelen istekte
// X-Forwarded-For'a güveniyordu. Loopback yalnız nginx'e ait değildir: aynı
// sunucudaki her tenant süreci (PHP, Node/Python uygulaması, cron, SSH jail)
// 127.0.0.1:8080'e doğrudan bağlanıp X-Forwarded-For'a istediği IP'yi
// yazabiliyor, böylece panel IP izin listesini ve IP başına giriş sınırını
// atlatabiliyordu.
//
// Çözüm: nginx backend'e her istekte yalnız root'un okuyabildiği bir anahtar
// gönderir (X-SanalCP-Proxy). Backend, loopback'ten gelen istemci-IP
// başlıklarına YALNIZ bu anahtar doğruysa güvenir (bkz. httpx.ClientIP).
// Anahtar nginx'e 0600 root:root bir conf.d dosyasıyla verilir; nginx
// master süreci yapılandırmayı root olarak okur, tenant'lar okuyamaz.
//
// Aynı dosya, 443 panel alan adı vhost'undan (_panel_domain.conf) gelen
// isteğin GERÇEK istemci IP'sini _panel.conf'a taşımak için de kullanılır:
// 443 bloğu da anahtarı ve istemci IP'sini gönderir; _panel.conf yalnız
// anahtar doğruysa o IP'yi kabul eder, aksi hâlde $remote_addr kullanır.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// ProxyAnahtarYol: anahtarın kalıcı kopyası (panel env'iyle aynı dizin).
	ProxyAnahtarYol = "/etc/sanalcp/proxy-anahtar"
	// ProxyAnahtarConfYol: nginx'in http seviyesinde yüklediği map dosyası.
	// conf.d/*.conf glob'u hem Debian hem RHEL nginx.conf'unda http{} içindedir.
	ProxyAnahtarConfYol = "/etc/nginx/conf.d/_sanalcp_proxy_anahtar.conf"
	// ProxyAnahtarBaslik: nginx → backend kimlik başlığı.
	ProxyAnahtarBaslik = "X-SanalCP-Proxy"
)

var proxyAnahtarRE = regexp.MustCompile(`^[a-f0-9]{32}$`)

// ProxyAnahtarConf: verilen anahtar için nginx map dosyasının içeriği.
//
// $sanalcp_proxy_anahtari  → backend'e ve 443→8443 atlamasına gönderilen değer.
// $sanalcp_istemci_ip      → _panel.conf'un backend'e ilettiği istemci IP'si:
// istek 443 panel alan adı bloğundan (anahtarla) geldiyse onun bildirdiği IP,
// değilse bağlantının kendi adresi.
func ProxyAnahtarConf(anahtar string) string {
	return "# SanalCP — panel proxy anahtarı (panel tarafından üretilir; elle düzenlemeyin).\n" +
		"# 0600 root:root olmalı: tenant'lar bu değeri okuyamamalı.\n" +
		"map \"\" $sanalcp_proxy_anahtari {\n" +
		"    default \"" + anahtar + "\";\n" +
		"}\n" +
		"map $http_x_sanalcp_proxy $sanalcp_istemci_ip {\n" +
		"    \"" + anahtar + "\" $http_x_sanalcp_istemci;\n" +
		"    default $remote_addr;\n" +
		"}\n"
}

// ProxyAnahtariHazirla: anahtarı okur (yoksa üretir) ve nginx map dosyasını
// güncel tutar. Dönen bool, nginx dosyasının değişip değişmediğidir (çağıran
// gerekirse nginx'i yeniden yükler). Panel bu sunucuda kurulu değilse
// (/etc/nginx/conf.d yok) "" döner.
func ProxyAnahtariHazirla() (string, bool, error) {
	return proxyAnahtariHazirla(ProxyAnahtarYol, ProxyAnahtarConfYol)
}

func proxyAnahtariHazirla(anahtarYol, confYol string) (string, bool, error) {
	if st, err := os.Stat(filepath.Dir(confYol)); err != nil || !st.IsDir() {
		return "", false, nil
	}
	anahtar := ""
	if b, err := os.ReadFile(anahtarYol); err == nil {
		anahtar = strings.TrimSpace(string(b))
		if !proxyAnahtarRE.MatchString(anahtar) {
			return "", false, fmt.Errorf("%s bozuk (32 onaltılık karakter bekleniyordu)", anahtarYol)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		ham := make([]byte, 16) // 128 bit; 64 karakterlik anahtar nginx map_hash_bucket_size (64) sınırını aşar
		if _, err := rand.Read(ham); err != nil {
			return "", false, err
		}
		anahtar = hex.EncodeToString(ham)
		if err := os.MkdirAll(filepath.Dir(anahtarYol), 0o700); err != nil {
			return "", false, err
		}
		if err := atomikYaz(anahtarYol, []byte(anahtar+"\n")); err != nil {
			return "", false, err
		}
	} else {
		return "", false, err
	}

	istenen := []byte(ProxyAnahtarConf(anahtar))
	mevcut, err := os.ReadFile(confYol)
	if err == nil && bytes.Equal(mevcut, istenen) {
		// İçerik doğru; izinler yine de daraltılır (elle gevşetilmiş olabilir).
		_ = os.Chmod(confYol, 0o600)
		return anahtar, false, nil
	}
	if err := atomikYaz(confYol, istenen); err != nil {
		return "", false, err
	}
	return anahtar, true, nil
}

// atomikYaz: 0600 geçici dosya + rename. Yarım yazılmış bir map dosyası
// nginx -t'yi kırar; anahtarın bir an bile geniş izinle durmaması gerekir.
func atomikYaz(yol string, veri []byte) error {
	f, err := os.CreateTemp(filepath.Dir(yol), ".sanalcp-anahtar-*")
	if err != nil {
		return err
	}
	ad := f.Name()
	defer os.Remove(ad)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(veri); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(ad, yol)
}
