package laravel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemizRelTenantDisinaCikamaz(t *testing.T) {
	for _, v := range []string{"../x", "public_html/../../etc", "/etc", "public_html/a b"} {
		if _, err := temizRel(v); err == nil {
			t.Errorf("tehlikeli yol kabul edildi: %q", v)
		}
	}
	if got, err := temizRel("public_html/app"); err != nil || got != "public_html/app" {
		t.Fatalf("geçerli yol: %q %v", got, err)
	}
}

func TestLaravelKesfiAltDiziniBulurSymlinkTakipEtmez(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "public_html", "app")
	for _, d := range []string{"bootstrap"} {
		if e := os.MkdirAll(filepath.Join(root, d), 0755); e != nil {
			t.Fatal(e)
		}
	}
	for _, f := range []string{"artisan", "composer.json", "bootstrap/app.php"} {
		if e := os.WriteFile(filepath.Join(root, f), []byte("x"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "artisan"), []byte("x"), 0644)
	_ = os.WriteFile(filepath.Join(outside, "composer.json"), []byte("x"), 0644)
	_ = os.Mkdir(filepath.Join(outside, "bootstrap"), 0755)
	_ = os.WriteFile(filepath.Join(outside, "bootstrap", "app.php"), []byte("x"), 0644)
	_ = os.Symlink(outside, filepath.Join(home, "public_html", "evil"))
	got := discoverRoots(home)
	if len(got) != 1 || got[0] != "public_html/app" {
		t.Fatalf("keşif sonucu: %#v", got)
	}
}

func TestEnvGizliAnahtarlariAyirtEder(t *testing.T) {
	for _, k := range []string{"APP_KEY", "DB_PASSWORD", "STRIPE_SECRET", "API_TOKEN"} {
		if !secretKeyRE.MatchString(k) {
			t.Errorf("%s gizli sayılmalı", k)
		}
	}
	if secretKeyRE.MatchString("APP_NAME") {
		t.Fatal("APP_NAME gizli sayılmamalı")
	}
}

func TestEnvSetDegeriTekSatiraKilitler(t *testing.T) {
	b := envSet([]byte("APP_NAME=Old\nAPP_ENV=prod\n"), "APP_NAME", `New "Shop"`)
	s := string(b)
	if strings.Count(s, "APP_NAME=") != 1 || !strings.Contains(s, `APP_NAME="New \"Shop\""`) {
		t.Fatalf("beklenmeyen env: %s", s)
	}
	if got := parseEnv(b)["APP_NAME"]; got != `New "Shop"` {
		t.Fatalf("parse sonucu: %q", got)
	}
}

func TestArtisanSerbestKomutKabulEtmez(t *testing.T) {
	if _, ok := artisanAllowed["tinker"]; ok {
		t.Fatal("tinker izin listesinde olmamalı")
	}
	if _, ok := artisanAllowed["migrate-status"]; !ok {
		t.Fatal("salt okunur migrate status eksik")
	}
}

func TestSchedulerCronDigerSatirlariKorurVeTekillesir(t *testing.T) {
	root := "/home/demo/public_html"
	old := "0 2 * * * /usr/bin/backup\n* * * * * /usr/bin/php " + root + "/artisan schedule:run # sanalcp-laravel-scheduler " + root + "\n"
	got := schedulerCron(old, root, true)
	if !strings.Contains(got, "/usr/bin/backup") || strings.Count(got, "sanalcp-laravel-scheduler") != 1 {
		t.Fatalf("cron güvenli güncellenmedi: %s", got)
	}
	if off := schedulerCron(got, root, false); strings.Contains(off, "schedule:run") || !strings.Contains(off, "/usr/bin/backup") {
		t.Fatalf("cron kapatma: %s", off)
	}
}

func TestQueueUnitArgumanlariSabitVeTenantKimliginde(t *testing.T) {
	d := domain{ID: 7, AlanAdi: "example.com", SK: "demo", Home: "/home/demo"}
	p := install{Root: "/home/demo/public_html"}
	u := queueUnit(d, p, "emails", 3, 90)
	for _, want := range []string{"User=demo", "queue:work --queue=emails", "NoNewPrivileges=true", "ProtectSystem=strict"} {
		if !strings.Contains(u, want) {
			t.Errorf("unit içinde %q yok", want)
		}
	}
}
