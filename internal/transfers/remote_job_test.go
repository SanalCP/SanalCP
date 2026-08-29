package transfers

import (
	"bytes"
	"strings"
	"testing"
)

func TestSinirliYazici(t *testing.T) {
	var b bytes.Buffer
	w := &sinirliYazici{w: &b, kalan: 4}
	if n, err := w.Write([]byte("1234")); err != nil || n != 4 {
		t.Fatalf("ilk yazma n=%d err=%v", n, err)
	}
	if _, err := w.Write([]byte("5")); err == nil {
		t.Fatal("sınır aşımı kabul edildi")
	}
}

func TestUzakHesapSaglayiciyaGoreDogrulanir(t *testing.T) {
	if !uzakHesapGecerli("plesk", "example.com", "example.com") {
		t.Fatal("Plesk site kimliği reddedildi")
	}
	if uzakHesapGecerli("plesk", "other.example", "example.com") {
		t.Fatal("Plesk'te başka site kimliği kabul edildi")
	}
	if !uzakHesapGecerli("directadmin", "demo_user", "example.com") {
		t.Fatal("DirectAdmin kullanıcı adı reddedildi")
	}
}

func TestTumSaglayicilarAkisArsiviUretir(t *testing.T) {
	for _, p := range []string{"cpanel", "plesk", "directadmin"} {
		hesap := "demo"
		if p == "plesk" {
			hesap = "example.com"
		}
		cmd, err := uzakPaketKomutu(remoteStartReq{Provider: p, Hesap: hesap, Domain: "example.com"})
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if !strings.Contains(cmd, "tar") || !strings.Contains(cmd, "cpmove-") {
			t.Errorf("%s ortak arşiv sözleşmesini üretmedi", p)
		}
	}
}

func TestHTTPSaglikSiniri(t *testing.T) {
	for _, v := range []int{200, 301, 404} {
		if !httpSaglikli(v) {
			t.Errorf("%d sağlıklı sayılmalı", v)
		}
	}
	for _, v := range []int{0, 199, 500, 503} {
		if httpSaglikli(v) {
			t.Errorf("%d sağlıksız sayılmalı", v)
		}
	}
}

func TestUzakPaketHesabiYalnizGuvenliUnixAdi(t *testing.T) {
	for _, s := range []string{"root;id", "x$(id)", "../root", "UPPER"} {
		if uzakUserRe.MatchString(s) {
			t.Errorf("tehlikeli hesap kabul edildi: %q", s)
		}
	}
}
