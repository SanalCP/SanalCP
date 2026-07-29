// Package kilit: kaynak kimliğine göre süreç-içi (in-process) kilit sağlar.
// Tek amaç: aynı bayi üzerinde eşzamanlı çalışan "askıya al" (toplu) ile "yeni
// hosting oluştur" istekleri arasındaki yarış durumunu önlemek — aksi hâlde
// askı zinciri çalışırken tam o anda oluşturulan bir domain zincirin dışında
// kalıp askıdan muaf canlı kalabilir.
package kilit

import "sync"

var (
	mu      sync.Mutex
	kayitli = map[int64]*sync.Mutex{}
)

// Bayi: verilen bayi kullanıcı id'si için her zaman aynı mutex'i döner.
func Bayi(id int64) *sync.Mutex {
	mu.Lock()
	defer mu.Unlock()
	m, ok := kayitli[id]
	if !ok {
		m = &sync.Mutex{}
		kayitli[id] = m
	}
	return m
}
