package cron

import "testing"

func TestKomutlariMaskele(t *testing.T) {
	komut := "mysqldump -uroot -pGIZLI db > /tmp/x.sql"

	got := komutlariMaskele(true, []Gorev{{Dakika: "0", Saat: "3", Komut: komut}})
	if got[0].Komut == komut || got[0].Saat != "3" {
		t.Fatalf("demo modunda komut maskelenmedi ya da zamanlama bozuldu: %+v", got[0])
	}

	got = komutlariMaskele(false, []Gorev{{Komut: komut}})
	if got[0].Komut != komut {
		t.Fatalf("demo kapalıyken komut değişti: %q", got[0].Komut)
	}
}
