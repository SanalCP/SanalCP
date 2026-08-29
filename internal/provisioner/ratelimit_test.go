package provisioner

import (
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
)

func TestBuildRateLimit(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m.ExpectQuery("SELECT profil,burst,bot_engelle").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"profil", "burst", "bot"}).AddRow("ozel", 0, 1))
	got := buildRateLimit(db, 42)
	for _, want := range []string{"$sanal_bot_block_42", "zone=sanal_rl_42 nodelay", "limit_req_status 429"} {
		if !strings.Contains(got, want) {
			t.Errorf("çıktıda %q yok: %s", want, got)
		}
	}
}

func TestBuildRateLimit_ProfilKapaliBotAcik(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m.ExpectQuery("SELECT profil,burst,bot_engelle").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"profil", "burst", "bot"}).AddRow("kapali", 30, 1))
	got := buildRateLimit(db, 7)
	if !strings.Contains(got, "$sanal_bot_block_7") {
		t.Fatal("bot engeli üretilmedi")
	}
	if strings.Contains(got, "limit_req zone") {
		t.Fatal("kapalı profil hız limiti üretti")
	}
}
