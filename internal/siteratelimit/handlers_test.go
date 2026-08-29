package siteratelimit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIPIstisnalariNormalize(t *testing.T) {
	got, err := ipler([]string{"203.0.113.7", "192.0.2.99/24", "2001:db8::1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"203.0.113.7", "192.0.2.0/24", "2001:db8::1"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestYolIstisnasiTehlikeliGirdiyiReddeder(t *testing.T) {
	if _, err := yollarDogrula([]string{"/ok/*", "/x; return 200"}); err == nil {
		t.Fatal("tehlikeli yol kabul edildi")
	}
}

// Boş liste JSON'da "null" değil "[]" olmalı: Rate Limit sayfası ip_istisnalari
// ve yol_istisnalari üzerinde doğrudan .join() çağırıyor, null gelirse sayfa
// "Cannot read properties of null (reading 'join')" ile çöküyor. Rate limit
// tanımlanmamış bir domain (varsayılan durum) tam olarak bu yanıtı üretiyordu.
func TestBosListelerNullDegilBosDiziKodlanir(t *testing.T) {
	a := Ayar{Profil: "kapali", IstekDakika: 120, Burst: 30}
	a.IPIstisnalari = satirlar("")
	a.YolIstisnalari = satirlar("")

	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "null") {
		t.Errorf("yanıtta null var: %s", got)
	}
	if !strings.Contains(got, `"ip_istisnalari":[]`) || !strings.Contains(got, `"yol_istisnalari":[]`) {
		t.Errorf("boş dizi bekleniyordu: %s", got)
	}
}

func TestIplerVeYollarBosGirdideNullDondurmez(t *testing.T) {
	ips, err := ipler(nil)
	if err != nil || ips == nil {
		t.Errorf("ipler(nil)=%v, err=%v — boş dilim bekleniyordu", ips, err)
	}
	yollar, err := yollarDogrula(nil)
	if err != nil || yollar == nil {
		t.Errorf("yollarDogrula(nil)=%v, err=%v — boş dilim bekleniyordu", yollar, err)
	}
}
