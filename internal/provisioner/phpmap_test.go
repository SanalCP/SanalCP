package provisioner

import (
	"strings"
	"testing"
)

// 🔴 İki tablodan biri güncellenip diğeri unutulursa sessiz bir split-brain
// oluşur: o sürüm bir ailede yönetilir, diğerinde pool relative-path'e yazılır
// ve SetPHP/Deprovision o sürümü hiç temizleyemez. Kaynak dosyadaki uyarının
// testi budur.
func TestPHPTablolariAyniSurumleriIcerir(t *testing.T) {
	for surum := range phpMapRHEL {
		if _, ok := phpMapDebian[surum]; !ok {
			t.Errorf("PHP %s RHEL tablosunda var, Debian tablosunda YOK", surum)
		}
	}
	for surum := range phpMapDebian {
		if _, ok := phpMapRHEL[surum]; !ok {
			t.Errorf("PHP %s Debian tablosunda var, RHEL tablosunda YOK", surum)
		}
	}
}

// Hiçbir alan boş bırakılmamalı: boş PoolDir, pool'un göreli yola yazılması
// demektir (panelin çalışma dizinine), yani sessizce yanlış yere.
func TestPHPTablolariBosAlanIcermez(t *testing.T) {
	for ad, tablo := range map[string]map[string]phpAyar{
		"RHEL": phpMapRHEL, "Debian": phpMapDebian,
	} {
		for surum, ay := range tablo {
			if ay.PoolDir == "" || ay.SockDir == "" || ay.Service == "" || ay.FpmBin == "" {
				t.Errorf("%s/%s eksik alan: %+v", ad, surum, ay)
			}
			if !strings.HasPrefix(ay.PoolDir, "/") || !strings.HasPrefix(ay.SockDir, "/") {
				t.Errorf("%s/%s mutlak yol değil: %+v", ad, surum, ay)
			}
		}
	}
}

// Debian tablosu deb.sury.org sözleşmesini izlemeli. Buradaki bir yazım hatası
// (ör. php83-fpm) servisin hiç bulunamamasına ve sitenin açılmamasına yol açar.
func TestPHPDebianTablosuSuryDuzenineUyar(t *testing.T) {
	for surum, ay := range phpMapDebian {
		if got, want := ay.PoolDir, "/etc/php/"+surum+"/fpm/pool.d"; got != want {
			t.Errorf("%s PoolDir: got %q want %q", surum, got, want)
		}
		if got, want := ay.Service, "php"+surum+"-fpm"; got != want {
			t.Errorf("%s Service: got %q want %q", surum, got, want)
		}
		if got, want := ay.FpmBin, "/usr/sbin/php-fpm"+surum; got != want {
			t.Errorf("%s FpmBin: got %q want %q", surum, got, want)
		}
		// sury tüm sürümlerde soketleri /run/php altında tutar.
		if ay.SockDir != "/run/php" {
			t.Errorf("%s SockDir: got %q want /run/php", surum, ay.SockDir)
		}
	}
}

// Soket adı SÜRÜME değil KİRACIYA göre verilir (SockDir/<sk>.sock). Debian'da
// tüm sürümler aynı SockDir'i paylaştığı için bu davranışın korunması şart:
// aksi halde iki sürüm aynı kiracı için aynı sokete yazmaya çalışırdı.
// Havuz dosyaları sürüme özel dizinlerde kaldığı için temizlik doğru işler.
func TestPHPDebianHavuzDizinleriSurumeOzel(t *testing.T) {
	gorulen := map[string]string{}
	for surum, ay := range phpMapDebian {
		if onceki, varsa := gorulen[ay.PoolDir]; varsa {
			t.Errorf("PoolDir %q iki sürümde ortak (%s ve %s) — sürümler arası pool temizliği bozulur",
				ay.PoolDir, onceki, surum)
		}
		gorulen[ay.PoolDir] = surum
	}
}
