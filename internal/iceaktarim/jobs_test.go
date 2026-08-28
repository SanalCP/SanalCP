package iceaktarim

import "testing"

func TestRollbackScope(t *testing.T) {
	for _, tc := range []struct{ target, want string }{{"public_html", "files"}, {"public_html/shop", "files"}, {"depo/site", "home"}, {"public_html2", "home"}} {
		if got := rollbackScope(tc.target); got != tc.want {
			t.Errorf("rollbackScope(%q)=%q, want %q", tc.target, got, tc.want)
		}
	}
}
