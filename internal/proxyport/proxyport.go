// Package proxyport: reverse-proxy sitelerinin yönlendirilebileceği yerel
// (127.0.0.1) port politikası.
//
// 🔴 GÜVENLİK: reverse-proxy hedefi yalnız 127.0.0.1 olabilir, ama port
// eskiden yalnız 8080/8443/10080 için reddediliyordu. Müşteri rolündeki bir
// kullanıcı sitesini 127.0.0.1'deki HERHANGİ bir servise yönlendirip
// internete açabiliyordu: panel CLI API'si ve kimlik doğrulamasız /metrics
// (8090), başka bir tenant'ın Node/Python uygulaması (o sitenin nginx
// korumalarını atlayarak), redis/rspamd gibi sistem servisleri.
//
// Kural:
//   - Paneli oluşturan portlar HERKES için yasaktır (Ayrilmis).
//   - Admin dışındaki roller yalnız (a) şu an kimsenin dinlemediği ya da
//     (b) yalnız KENDİ tenant kullanıcısının dinlediği bir porta yönlendirebilir;
//     port başka bir tenant'ın reverse-proxy sitesine de ayrılmış olmamalıdır.
//   - Admin, sistem servislerini (ör. bir izleme paneli) bir alan adıyla
//     yayınlamak isteyebilir; onun için yalnız Ayrilmis listesi uygulanır.
package proxyport

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Ayrilmis: panelin kendi dinlediği portlar — hiçbir rol bunlara yönlendiremez.
var Ayrilmis = map[int]string{
	8080:  "panel API",
	8090:  "panel CLI API",
	8443:  "panel arayüzü",
	10080: "panel iç servisi",
}

// procNetDosyalari: test için değiştirilebilir.
var procNetDosyalari = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// AyrilmisMi: port panelin kendisine mi ait?
func AyrilmisMi(port int) bool {
	_, ok := Ayrilmis[port]
	return ok
}

// DinleyenUIDler: verilen TCP portunu dinleyen (LISTEN) soketlerin sahip
// kullanıcıları. Dinleyen yoksa boş döner.
func DinleyenUIDler(port int) ([]int, error) {
	var uidler []int
	gorulen := map[int]bool{}
	for _, yol := range procNetDosyalari {
		f, err := os.Open(yol)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		tara := bufio.NewScanner(f)
		ilk := true
		for tara.Scan() {
			if ilk { // başlık satırı
				ilk = false
				continue
			}
			alan := strings.Fields(tara.Text())
			if len(alan) < 8 || alan[3] != "0A" { // 0A = TCP_LISTEN
				continue
			}
			i := strings.LastIndexByte(alan[1], ':')
			if i < 0 {
				continue
			}
			p, err := strconv.ParseUint(alan[1][i+1:], 16, 16)
			if err != nil || int(p) != port {
				continue
			}
			uid, err := strconv.Atoi(alan[7])
			if err != nil {
				continue
			}
			if !gorulen[uid] {
				gorulen[uid] = true
				uidler = append(uidler, uid)
			}
		}
		err = tara.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return uidler, nil
}

// TenantIcinDogrula: admin olmayan bir rolün sk tenant'ı adına port'a
// yönlendirme yapıp yapamayacağını denetler. tenantUID < 0 ise tenant henüz
// yoktur (yeni domain) ve port boş olmalıdır. haricDomainID, kendi satırını
// "başka site" saymamak için verilir (yeni domainde 0).
func TenantIcinDogrula(ctx context.Context, db *sql.DB, port int, sk string, tenantUID int, haricDomainID int64) error {
	if ad, ok := Ayrilmis[port]; ok {
		return fmt.Errorf("bu port SanalCP tarafından ayrılmıştır (%s)", ad)
	}
	if db != nil {
		var n int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM domains
			 WHERE COALESCE(web_backend,'php-fpm')='reverse-proxy' AND proxy_port=?
			   AND sistem_kullanici<>? AND id<>?`, port, sk, haricDomainID).Scan(&n); err != nil {
			return errors.New("port uygunluğu doğrulanamadı")
		}
		if n > 0 {
			return errors.New("bu port başka bir hesabın sitesine ayrılmış")
		}
	}
	uidler, err := DinleyenUIDler(port)
	if err != nil {
		return errors.New("port uygunluğu doğrulanamadı")
	}
	for _, uid := range uidler {
		if tenantUID < 0 || uid != tenantUID {
			return errors.New("bu port sunucuda başka bir hizmet tarafından kullanılıyor; yalnız boş bir porta ya da kendi uygulamanızın portuna yönlendirebilirsiniz")
		}
	}
	return nil
}
