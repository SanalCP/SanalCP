package hesaplar

import "testing"

func TestMySQLGrantNewUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLGrantNewUser(nil, 1, "gecerli_db", "kotu ad", "Parola1234567!"); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
	if err := MySQLGrantNewUser(nil, 1, "kötü-db", "gecerli_user", "Parola1234567!"); err == nil {
		t.Error("tire içeren DB adı reddedilmeliydi")
	}
}

func TestMySQLGrantExistingUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLGrantExistingUser(nil, 1, "gecerli_db", "kotu;user"); err == nil {
		t.Error("noktalı virgüllü kullanıcı adı reddedilmeliydi")
	}
}
