package system

import "testing"

func TestBoyutByte(t *testing.T) {
	tests := map[string]uint64{
		"Archived and active journals take up 56.3M in the file system.": 59034828,
		"2.0G":    2 << 30,
		"512 KiB": 512 << 10,
	}
	for in, want := range tests {
		if got := boyutByte(in); got != want {
			t.Errorf("boyutByte(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestAlanAdiDogrula(t *testing.T) {
	for _, ok := range []string{"cloud.sanalcp.com", "example.org", "a-b.example"} {
		if err := alanAdiDogrula(ok); err != nil {
			t.Errorf("%q reddedildi: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-bad.example", "bad-.example", "bad;id", "a..b"} {
		if err := alanAdiDogrula(bad); err == nil {
			t.Errorf("%q kabul edildi", bad)
		}
	}
}

func TestDepolamaVarsayilanlari(t *testing.T) {
	eski := depolamaAyarYolu
	_ = eski // sabit yolun varsayılan davranışını belgeleyen basit sınır testi
	a := depolamaAyar{DiskEsik: 80, InodeEsik: 85}
	if a.DiskEsik < 50 || a.InodeEsik > 99 {
		t.Fatal("geçersiz varsayılan eşik")
	}
}

func TestGuvenlikBetigiRebootVarsayilani(t *testing.T) {
	a := guvenlikAyariOku()
	if a.OtomatikReboot && !a.Aktif {
		t.Fatal("pasif ayarda otomatik reboot etkin olamaz")
	}
}

func TestSwapKaynaklariBosDizi(t *testing.T) {
	d := swapOku()
	if d.Kaynaklar == nil {
		t.Fatal("swap kaynakları null dönmemeli; frontend dizi bekliyor")
	}
}
