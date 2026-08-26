package wordpress

import "testing"

func TestAdapterTemelBilgiler(t *testing.T) {
	a := Adapter{}
	if a.Slug() != "wordpress" {
		t.Fatalf("Slug() = %q, beklenen wordpress", a.Slug())
	}
	if a.DBOnEki() != "wp" {
		t.Fatalf("DBOnEki() = %q, beklenen wp (mevcut wp_/wpu_ DB adlandırma deseniyle uyumlu olmalı)", a.DBOnEki())
	}
	if a.MarkerDosya() != "wp-config.php" {
		t.Fatalf("MarkerDosya() = %q, beklenen wp-config.php", a.MarkerDosya())
	}
	if a.GuncelleDesteklenir() != true {
		t.Fatal("WordPress güncelleme desteklemeli (wp core update mevcut)")
	}
	alanlar := a.FormAlanlari()
	beklenenAnahtarlar := map[string]bool{"site_basligi": false, "admin_kullanici": false, "admin_email": false}
	for _, fa := range alanlar {
		if _, var_ := beklenenAnahtarlar[fa.Anahtar]; var_ {
			beklenenAnahtarlar[fa.Anahtar] = true
		}
	}
	for k, bulundu := range beklenenAnahtarlar {
		if !bulundu {
			t.Errorf("form alanlarında %q eksik", k)
		}
	}
}

func TestAdapterDBAdiOkuGecersizDosya(t *testing.T) {
	a := Adapter{}
	if _, bulundu := a.DBAdiOku(t.TempDir()); bulundu {
		t.Fatal("wp-config.php olmayan dizinde bulundu=false olmalı")
	}
}
