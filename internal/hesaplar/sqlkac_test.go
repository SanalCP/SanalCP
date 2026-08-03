package hesaplar

import "testing"

// sqlKac, MySQLCreateDB/MySQLChangePassword gibi fonksiyonlarda CREATE/ALTER USER
// ... IDENTIFIED BY '%s' kalıbına gömülen parolaları kaçışlıyor (MariaDB'nin
// IDENTIFIED BY ifadesi ? placeholder'ını desteklemiyor — gerçek sunucuda
// doğrulandı, bkz. commit mesajı). Kaçış yanlış olursa ya syntax hatası
// (zararsız ama can sıkıcı) ya da EN KÖTÜ durumda statement enjeksiyonu olur.
func TestSqlKac(t *testing.T) {
	vakalar := []struct{ girdi, beklenen string }{
		{`basit`, `basit`},
		{`tek'tirnak`, `tek\'tirnak`},
		{`ters\bolu`, `ters\\bolu`},
		{`'; DROP TABLE domains; --`, `\'; DROP TABLE domains; --`},
		{`\'`, `\\\'`},
	}
	for _, v := range vakalar {
		if got := sqlKac(v.girdi); got != v.beklenen {
			t.Errorf("sqlKac(%q) = %q, beklenen %q", v.girdi, got, v.beklenen)
		}
	}
}

// TestSqlKacUretilenIfadeGecerli: kaçışlanmış değer bir SQL string-literal'ine
// gömülünce, literal ORİJİNAL sınırında kapanıyor mu (ekstra tırnak açığa
// çıkmıyor mu)? Basit bir parse simülasyonu — gerçek MariaDB'ye karşı bu tam
// senaryo (özel karakterli parolayla CREATE USER + o parolayla gerçek giriş)
// gerçek sunucuda ayrıca doğrulandı.
func TestSqlKacUretilenIfadeGecerli(t *testing.T) {
	tehlikeli := `x', 'y'); DROP DATABASE panel; -- `
	kacisli := sqlKac(tehlikeli)
	// Kaçışlı değerde çıplak (kaçışsız) tek tırnak KALMAMALI.
	acik := false
	kacis := false
	for _, r := range kacisli {
		if kacis {
			kacis = false
			continue
		}
		if r == '\\' {
			kacis = true
			continue
		}
		if r == '\'' {
			acik = true
		}
	}
	if acik {
		t.Fatalf("sqlKac(%q) = %q içinde kaçışsız tek tırnak kaldı", tehlikeli, kacisli)
	}
}
