package provisioner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// acmeStagingServer: Let's Encrypt STAGING dizin URL'i. Staging cert'leri
// tarayıcıda güvenilir değildir; üretimden ayrı, daha geniş limitleri vardır. CI ve manuel
// testlerde gerçek CA'yı yakmadan --issue akışını uçtan uca doğrulamak için var.
const acmeStagingServer = "https://acme-staging-v02.api.letsencrypt.org/directory"

// AcmeServerArgs: ACME_STAGING=1 ortam değişkeni set edilmişse acme.sh'e
// staging sunucusunu kullandıran bayrakları döner, aksi halde nil (varsayılan:
// gerçek LE prod API). Yalnız --issue çağrılarına eklenmeli — --install-cert
// zaten yerel store'dan kopyalar, sunucuya gitmez.
func AcmeServerArgs() []string {
	if os.Getenv("ACME_STAGING") == "1" {
		return []string{"--server", acmeStagingServer}
	}
	return nil
}

// issueWithPreflight doğrulamayı ayrı bir acme.sh deposunda yapar. Aynı depo
// kullanılırsa staging sertifikası üretim sertifikasının domain kaydını ve
// otomatik yenileme ayarlarını değiştirebilir. Geçici depo silinmeden önce
// staging sertifikası hiçbir zaman --install-cert ile canlıya taşınmaz.
func issueWithPreflight(ctx context.Context, acmeBin string, issueArgs []string) ([]byte, error) {
	if os.Getenv("ACME_STAGING") == "1" {
		// Eski manuel/CI staging modu: üretim isteği yapılmaz.
		args := append(append([]string{}, issueArgs...), AcmeServerArgs()...)
		return exec.CommandContext(ctx, acmeBin, args...).CombinedOutput()
	}

	tmp, err := os.MkdirTemp("", "sanalcp-acme-preflight-")
	if err != nil {
		return nil, fmt.Errorf("staging çalışma dizini oluşturulamadı: %w", err)
	}
	defer os.RemoveAll(tmp)
	args := append(append([]string{}, issueArgs...),
		"--server", acmeStagingServer,
		"--home", tmp,
		"--config-home", filepath.Join(tmp, "config"),
		"--cert-home", filepath.Join(tmp, "certs"))
	stagingCtx, stagingCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer stagingCancel()
	if out, err := exec.CommandContext(stagingCtx, acmeBin, args...).CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(out))
		if len(message) > 500 {
			message = message[len(message)-500:]
		}
		return []byte("Let's Encrypt staging ön denemesi başarısız: " + message), fmt.Errorf("staging ön denemesi: %w", err)
	}

	args = append(append([]string{}, issueArgs...), "--server", "letsencrypt")
	productionCtx, productionCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer productionCancel()
	return exec.CommandContext(productionCtx, acmeBin, args...).CombinedOutput()
}
