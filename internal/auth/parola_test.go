package auth

import "testing"

func TestKullaniciRootMu(t *testing.T) {
	for _, ad := range []string{"root", "ROOT", " root ", "Root"} {
		if !KullaniciRootMu(ad) {
			t.Errorf("%q root sayılmalıydı", ad)
		}
	}
	// "root" varyasyonu gibi görünen ama farklı olan adlar sistem parolasına
	// yönlendirilmemeli.
	for _, ad := range []string{"root2", "rooot", "bayi", "", "toor"} {
		if KullaniciRootMu(ad) {
			t.Errorf("%q root sayılmamalıydı", ad)
		}
	}
}

func TestParolaHashleVeDogrula(t *testing.T) {
	const p = "cokGuvenliParola1"
	h, err := ParolaHashle(p)
	if err != nil {
		t.Fatalf("hash üretilemedi: %v", err)
	}
	if h == p {
		t.Fatal("parola düz metin saklanmış")
	}
	if !ParolaEslesiyorMu(h, p) {
		t.Error("doğru parola eşleşmeliydi")
	}
	if ParolaEslesiyorMu(h, "yanlisParola1") {
		t.Error("yanlış parola eşleşmemeliydi")
	}

	// Aynı parola iki kez hash'lendiğinde farklı çıktı vermeli (salt).
	h2, _ := ParolaHashle(p)
	if h == h2 {
		t.Error("bcrypt salt kullanmıyor görünüyor")
	}
}

func TestParolaKisaReddedilir(t *testing.T) {
	if _, err := ParolaHashle("kisa"); err == nil {
		t.Error("8 karakterden kısa parola reddedilmeliydi")
	}
}

func TestBosHashGirisEngellenir(t *testing.T) {
	// Parolası hiç atanmamış hesap (password_hash=''): boş parolayla da,
	// herhangi bir parolayla da giriş yapılamamalı.
	if ParolaEslesiyorMu("", "") {
		t.Error("boş hash + boş parola kabul edildi")
	}
	if ParolaEslesiyorMu("", "herhangiBirSey") {
		t.Error("boş hash herhangi bir parolayı kabul etti")
	}
}
