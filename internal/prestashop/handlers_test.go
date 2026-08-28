package prestashop

import "testing"

func TestTemizRel(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"", "public_html"}, {"/public_html/shop/", "public_html/shop"}} {
		got, e := temizRel(tc.in)
		if e != nil || got != tc.want {
			t.Errorf("temizRel(%q)=%q,%v", tc.in, got, e)
		}
	}
	for _, bad := range []string{"../etc", "public_html/a/../../etc", "home/site", "public_html/a b"} {
		if _, e := temizRel(bad); e == nil {
			t.Errorf("geçersiz yol kabul edildi: %q", bad)
		}
	}
}

func TestParseDBConfig(t *testing.T) {
	modern := []byte(`'database_name'=>'shop_db','database_user'=>'shop_user','database_password'=>'secret','database_prefix'=>'abc_'`)
	m := parseDBConfig(modern)
	if m["database_name"] != "shop_db" || m["database_prefix"] != "abc_" {
		t.Fatalf("modern config: %#v", m)
	}
	legacy := []byte(`define('_DB_NAME_', 'old_db'); define('_DB_USER_', 'old_user'); define('_DB_PREFIX_', 'ps_');`)
	m = parseDBConfig(legacy)
	if m["database_name"] != "old_db" || m["database_user"] != "old_user" {
		t.Fatalf("legacy config: %#v", m)
	}
}
