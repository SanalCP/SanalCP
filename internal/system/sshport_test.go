package system

import (
	"reflect"
	"testing"
)

// Port satırı sshd -T çıktısından okunur. Gerçek çıktı küçük harfli ("port 22")
// ve başka satırlarda da "port" geçen anahtarlar bulunur (ör. "x11displayoffset").
func TestRePortSatiri(t *testing.T) {
	ornek := `port 2222
addressfamily any
listenaddress 0.0.0.0:2222
permitrootlogin no
x11displayoffset 10
gatewayports no
`
	var bulunan []string
	for _, m := range rePortSatiri.FindAllStringSubmatch(ornek, -1) {
		bulunan = append(bulunan, m[1])
	}
	if !reflect.DeepEqual(bulunan, []string{"2222"}) {
		t.Errorf("yalnız 'port' satırı okunmalıydı: %v", bulunan)
	}
}

// Birden fazla Port direktifi olabilir (geçiş dönemi: hem 22 hem yeni port).
// Bu durumda 22 hâlâ açıktır ve uyarı GÖSTERİLMELİDİR.
func TestRePortSatiriCoklu(t *testing.T) {
	ornek := "port 22\nport 2222\n"
	var portlar []string
	for _, m := range rePortSatiri.FindAllStringSubmatch(ornek, -1) {
		portlar = append(portlar, m[1])
	}
	if len(portlar) != 2 {
		t.Fatalf("2 port bekleniyordu: %v", portlar)
	}
}

// "listenaddress 0.0.0.0:22" gibi satırlar port sanılmamalı.
func TestRePortSatiriListenAddressiAlmaz(t *testing.T) {
	if rePortSatiri.MatchString("listenaddress 0.0.0.0:22\n") {
		t.Error("listenaddress satırı port olarak okundu")
	}
}

func TestBenzersizSirali(t *testing.T) {
	if got := benzersizSirali([]int{2222, 22, 2222, 22}); !reflect.DeepEqual(got, []int{22, 2222}) {
		t.Errorf("tekrarsız+sıralı olmalı: %v", got)
	}
	if got := benzersizSirali(nil); got != nil {
		t.Errorf("boş girdide nil beklenir: %v", got)
	}
}

func TestSSListenPortAyristirma(t *testing.T) {
	satir := `LISTEN 0 128 0.0.0.0:2222 0.0.0.0:* users:(("sshd",pid=1,fd=3))`
	m := reSSListenPort.FindStringSubmatch(satir)
	if m == nil || m[1] != "2222" {
		t.Errorf("ss satırından port çıkarılamadı: %v", m)
	}
}
