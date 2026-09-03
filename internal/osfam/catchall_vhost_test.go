package osfam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Catch-all vhost'larda `try_files $uri /index.html;` (son parametre URI) ölümcül:
// /var/www/_default80/index.html yoksa nginx sonsuz iç yönlendirmeye girer ve
// HTTP 500 döner. Sertifikası henüz olmayan her domain bu vhost'a düştüğü için
// o 500, aktarım sağlık kontrolünde sitenin kendi hatası sanılıyordu.
func TestCatchAllVhostlariIcYonlendirmeDongusuneGirmez(t *testing.T) {
	kok, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, ad := range []string{"_default80.conf", "_default443.conf"} {
		b, err := os.ReadFile(filepath.Join(kok, "assets", "nginx", ad))
		if err != nil {
			t.Skipf("%s okunamadı: %v", ad, err)
		}
		icerik := string(b)
		if !strings.Contains(icerik, "try_files $uri /index.html =404;") {
			t.Errorf("%s: try_files son parametresi =404 olmalı", ad)
		}
		if strings.Contains(icerik, "try_files $uri /index.html;") {
			t.Errorf("%s: URI ile biten try_files iç yönlendirme döngüsü üretir", ad)
		}
	}
}

// Kurulum ve güncelleme, catch-all belge kökünü kendisi yaratmalı; aksi hâlde
// =404 sayesinde 500 gitse de her bilinmeyen host boş 404 alır.
func TestKurulumVeGuncellemeCatchAllKokunuOlusturur(t *testing.T) {
	kok, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, yol := range []string{"sanalcp-install.sh", filepath.Join("assets", "ops", "sanalcp-update")} {
		b, err := os.ReadFile(filepath.Join(kok, yol))
		if err != nil {
			t.Skipf("%s okunamadı: %v", yol, err)
		}
		if !strings.Contains(string(b), "/var/www/_default80/index.html") {
			t.Errorf("%s: catch-all belge kökü oluşturulmuyor", yol)
		}
	}
}
