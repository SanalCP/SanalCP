package hesaplar

import (
	"context"
	"testing"
)

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

func TestMySQLRevokeUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLRevokeUser(nil, "kötü-db", "gecerli_user", false); err == nil {
		t.Error("tire içeren DB adı reddedilmeliydi")
	}
	if err := MySQLRevokeUser(nil, "gecerli_db", "kotu user", true); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
}

func TestMySQLRenameDBGecersizKimlikleriReddeder(t *testing.T) {
	ctx := context.Background()
	if err := MySQLRenameDB(ctx, nil, 1, "kötü-db", "yeni_db", []string{"kullanici1"}); err == nil {
		t.Error("tire içeren eski DB adı reddedilmeliydi")
	}
	if err := MySQLRenameDB(ctx, nil, 1, "eski_db", "kötü-db", []string{"kullanici1"}); err == nil {
		t.Error("tire içeren yeni DB adı reddedilmeliydi")
	}
	if err := MySQLRenameDB(ctx, nil, 1, "eski_db", "yeni_db", []string{"kotu user"}); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
}
