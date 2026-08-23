package provisioner

import "os"

// acmeStagingServer: Let's Encrypt STAGING dizin URL'i. Staging cert'leri
// tarayıcıda güvenilir değildir ama rate-limit'e tabi DEĞİLDİR — CI ve manuel
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
