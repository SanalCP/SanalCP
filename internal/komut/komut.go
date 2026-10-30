// Package komut: sistem araçlarını (rndc, systemctl, htpasswd, xfs_quota, ...)
// tek tip ve ZAMAN AŞIMLI çalıştırma yardımcıları.
//
// NEDEN VAR: exec.Command ile çağrılan bir sistem aracı takılırsa (kilitli bir
// diskte quota, askıda kalan systemctl, asılı kalan rndc) istek goroutine'i
// süresiz bloklanır; http.Server'ın WriteTimeout'u handler goroutine'ini ve
// child process'i ÖLDÜRMEZ. Buradaki her çağrı bir tavan süreyle sınırlıdır ve
// context iptali child process'e iletilir (exec.CommandContext + SIGKILL).
//
// ÇAĞRI KILAVUZU: mümkünse çağıranın context'ini geçirin — iptal/tetik zaman
// aşımı child'a ulaşsın. Context'in kendi deadline'ı daha erken ise o kazanır
// (WithTimeout tavan ekler, aşağı çekmez). stdout temiz gerekliyse Cikti,
// hata mesajında stderr de isteniyorsa BirlesikCikti kullanın.
package komut

import (
	"context"
	"os/exec"
	"time"
)

// VarsayilanSure: tek bir sistem aracı çağrısı için tavan süre.
// Araçlar (named-checkconf, systemctl reload, htpasswd, ...) saniyelerle
// biten işlerdir; 30s gerçek bir takılmayı yakalamak için cömert bir sınırdır.
const VarsayilanSure = 30 * time.Second

// Calistir: aracı çalıştırır, çıktıyla ilgilenmez.
func Calistir(ctx context.Context, ad string, args ...string) error {
	return calistirZ(ctx, VarsayilanSure, ad, args...)
}

// Cikti: aracı çalıştırır, YALNIZ stdout'u döndürür (stderr hariç — çıktısı
// ayrıştırılacak komutlarda stderr gürültüsü parse'ı bozmasın).
func Cikti(ctx context.Context, ad string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, VarsayilanSure)
	defer cancel()
	return exec.CommandContext(cctx, ad, args...).Output()
}

// BirlesikCikti: stdout+stderr birleşik döndürür (hata mesajı gösterilecek
// komutlarda kullanın: "zone geçersiz: <named-checkzone çıktısı>").
func BirlesikCikti(ctx context.Context, ad string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, VarsayilanSure)
	defer cancel()
	return exec.CommandContext(cctx, ad, args...).CombinedOutput()
}

// calistirZ: Calistir'in tavan-süre parametresiyle çekirdeği (beyaz-kutu test için).
func calistirZ(ctx context.Context, sure time.Duration, ad string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, sure)
	defer cancel()
	return exec.CommandContext(cctx, ad, args...).Run()
}
