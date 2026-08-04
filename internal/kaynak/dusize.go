package kaynak

// Disk kullanımı ölçümü: `du` bir dizin ağacının TAMAMINI gezer. Tenant home'u
// yüz binlerce dosya içerebilir; bu yüzden ölçüm:
//
//   - ÜST SINIRLI çalışır (context timeout) — yavaş/çok dosyalı bir hesap
//     istek işleyen goroutine'i süresiz bloklamasın,
//   - ÖNBELLEKLENİR — "Kaynak Kullanımı" sayfası art arda yenilendiğinde her
//     seferinde tam ağaç taraması yapılmasın. Panel root çalıştığı için bu
//     tarama tenant'ın cgroup I/O limitine takılmaz; önbellek olmadan müşteri
//     sayfayı yenileyerek sunucunun disk I/O'sunu boğabilirdi.
//
// Önbellek süresi dolmuş bir kayıt varken ölçüm başarısız olursa (timeout)
// BAYAT DEĞER döner — sıfır göstermek kullanıcı için yanıltıcı olurdu.

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	duZamanAsimi = 20 * time.Second
	duTazelik    = 60 * time.Second
)

type duKayit struct {
	mb     int64
	zaman  time.Time
	olcldi bool
}

var (
	duMu    sync.Mutex
	duCache = map[string]duKayit{}
)

// duMB: dizinin boyutunu MB olarak döner (önbellekli + zaman aşımlı).
func duMB(yol string) int64 {
	duMu.Lock()
	k, varMi := duCache[yol]
	duMu.Unlock()
	if varMi && time.Since(k.zaman) < duTazelik {
		return k.mb
	}

	ctx, cancel := context.WithTimeout(context.Background(), duZamanAsimi)
	defer cancel()
	out, err := exec.CommandContext(ctx, "du", "-sm", yol).Output()
	if err != nil {
		if varMi {
			return k.mb // bayat ama sıfırdan iyi
		}
		return 0
	}
	alanlar := strings.Fields(string(out))
	if len(alanlar) == 0 {
		if varMi {
			return k.mb
		}
		return 0
	}
	n, _ := strconv.ParseInt(alanlar[0], 10, 64)

	duMu.Lock()
	duCache[yol] = duKayit{mb: n, zaman: time.Now(), olcldi: true}
	// Sınırsız büyümeyi önle: eskimiş kayıtları buda.
	if len(duCache) > 512 {
		for y, kk := range duCache {
			if time.Since(kk.zaman) > 10*duTazelik {
				delete(duCache, y)
			}
		}
	}
	duMu.Unlock()
	return n
}
