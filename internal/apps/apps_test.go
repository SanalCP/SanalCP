package apps

import (
	"context"
	"testing"
)

type sahteUygulama struct {
	slug string
}

func (s sahteUygulama) Slug() string                { return s.slug }
func (s sahteUygulama) Ad() string                  { return "Sahte " + s.slug }
func (s sahteUygulama) DBOnEki() string              { return s.slug }
func (s sahteUygulama) MarkerDosya() string          { return "marker.txt" }
func (s sahteUygulama) FormAlanlari() []FormAlan     { return nil }
func (s sahteUygulama) GuncelleDesteklenir() bool    { return false }
func (s sahteUygulama) Kur(ctx context.Context, i KurulumIstek) (KurulumSonuc, error) {
	return KurulumSonuc{}, nil
}
func (s sahteUygulama) Bilgi(ctx context.Context, sk, dizin, url string) (Kurulum, error) {
	return Kurulum{}, nil
}
func (s sahteUygulama) Guncelle(ctx context.Context, sk, dizin string) error { return nil }
func (s sahteUygulama) DBAdiOku(dizin string) (string, bool)                { return "", false }

func TestKaydetBulHepsi(t *testing.T) {
	// Registry paket-seviyesi global olduğu için test isimlerini benzersiz
	// slug'larla izole ediyoruz (paralel testlerde çakışma olmasın diye t.Parallel YOK).
	Kaydet(sahteUygulama{slug: "test-tur-a"})
	Kaydet(sahteUygulama{slug: "test-tur-b"})

	if _, ok := Bul("test-tur-yok"); ok {
		t.Fatal("kayıtlı olmayan tür bulunmamalı")
	}
	u, ok := Bul("test-tur-a")
	if !ok || u.Slug() != "test-tur-a" {
		t.Fatalf("test-tur-a bulunamadı veya yanlış: %+v ok=%v", u, ok)
	}

	var gorulenA, gorulenB bool
	for _, u := range Hepsi() {
		switch u.Slug() {
		case "test-tur-a":
			gorulenA = true
		case "test-tur-b":
			gorulenB = true
		}
	}
	if !gorulenA || !gorulenB {
		t.Fatalf("Hepsi() her iki kayıtlı türü de içermeli (a=%v b=%v)", gorulenA, gorulenB)
	}
}

func TestKaydetIdempotent(t *testing.T) {
	Kaydet(sahteUygulama{slug: "test-tur-c"})
	oncekiUzunluk := len(Hepsi())
	Kaydet(sahteUygulama{slug: "test-tur-c"}) // aynı slug'ı ikinci kez kaydetmek listeyi büyütmemeli
	if len(Hepsi()) != oncekiUzunluk {
		t.Fatalf("aynı slug'ın tekrar kaydı Hepsi() uzunluğunu değiştirmemeli: önce=%d sonra=%d", oncekiUzunluk, len(Hepsi()))
	}
}
