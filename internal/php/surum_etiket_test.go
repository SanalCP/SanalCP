package php

import (
	"encoding/json"
	"strings"
	"testing"
)

// Domain oluşturma ekranındaki sürüm listesi SADE olmalı: "PHP 8.3".
// Etiketler ("AppStream · OPcache", "Remi modular — geliştirme/test/legacy")
// yanıltıcıydı ve kaldırıldı; geri sızmasınlar.
func TestVersionsAciklamaBos(t *testing.T) {
	s := Surum{Surum: "8.1", PoolDir: "/x", SockDir: "/y", Service: "php81-php-fpm"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, yasak := range []string{"OPcache", "AppStream", "Remi", "legacy"} {
		if strings.Contains(got, yasak) {
			t.Errorf("sürüm yanıtında %q görünmemeli: %s", yasak, got)
		}
	}
	if !strings.Contains(got, `"aciklama":""`) {
		t.Errorf("aciklama boş olmalı: %s", got)
	}
}
