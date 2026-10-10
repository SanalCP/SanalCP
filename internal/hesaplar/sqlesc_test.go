package hesaplar

import "testing"

func TestMySQLStringLiteralTemel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "''"},
		{"abc", "'abc'"},
		{"o'rap", `'o\'rap'`},
		{`a\b`, `'a\\b'`},
		{"a\nb", `'a\nb'`},
		{"a\rb", `'a\rb'`},
		{"a\x00b", `'a\0b'`},
		{"a\x1ab", `'a\Zb'`},
		{`a"b`, `'a\"b'`},
		// birleşik
		{`x'"\` + "\n\r\x00\x1a", `'x\'\"\\\n\r\0\Z'`},
	}
	for _, tc := range tests {
		if got := MySQLStringLiteral(tc.in); got != tc.want {
			t.Errorf("MySQLStringLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMySQLStringLiteralTirnakliCikti(t *testing.T) {
	got := MySQLStringLiteral("pass'word")
	if len(got) < 2 || got[0] != '\'' || got[len(got)-1] != '\'' {
		t.Fatalf("literal tırnaklarla sarılı olmalı: %q", got)
	}
}

func TestMySQLIdent(t *testing.T) {
	if got := MySQLIdent("wp_options"); got != "`wp_options`" {
		t.Errorf("MySQLIdent = %q", got)
	}
	// backtick kaçışı
	if got := MySQLIdent("a`b"); got != "`a``b`" {
		t.Errorf("MySQLIdent backtick = %q", got)
	}
}

func TestMySQLUserHost(t *testing.T) {
	got := MySQLUserHost("c_abc", "localhost")
	if got != "'c_abc'@'localhost'" {
		t.Errorf("MySQLUserHost = %q", got)
	}
	got = MySQLUserHost("u'x", "h\"y")
	if got != `'u\'x'@'h\"y'` {
		t.Errorf("MySQLUserHost escape = %q", got)
	}
}

// sqlKac artık kullanılmamalı; yeni kod MySQLStringLiteral kullanır.
// Eski davranışın kapsadığı iki karakteri doğrulayan regresyon testi.
func TestMySQLStringLiteralSqlKacUyumu(t *testing.T) {
	in := `a'b\c`
	want := `'a\'b\\c'`
	if got := MySQLStringLiteral(in); got != want {
		t.Errorf("uyum = %q, want %q", got, want)
	}
}
