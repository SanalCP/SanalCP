package kaynak

import (
	"context"
	"sanalcp/internal/diskusage"
)

// Ortak ölçüm servisi istekleri birleştirir, önbellekler ve tüm uçların
// toplam du eşzamanlılığını sınırlar. Hata durumunda önceki değer korunur.
func duMB(ctx context.Context, yol string) int64 {
	n, _ := diskusage.Measure(ctx, yol, false)
	return (n + (1 << 20) - 1) / (1 << 20)
}
