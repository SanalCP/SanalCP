package nginxset

import "testing"

func TestCacheProfilleri(t *testing.T) {
	testler := []struct {
		p    string
		acik bool
		dk   int
	}{{"kapali", false, 60}, {"genel", true, 15}, {"wordpress", true, 60}, {"prestashop", true, 30}}
	for _, x := range testler {
		s := Defaults()
		s.CacheProfili = x.p
		if err := profilUygula(&s); err != nil {
			t.Fatalf("%s: %v", x.p, err)
		}
		if s.FastCgiCache != x.acik || s.FastCgiCacheDakika != x.dk {
			t.Fatalf("%s => %#v", x.p, s)
		}
	}
	s := Defaults()
	s.CacheProfili = "bilinmeyen"
	if profilUygula(&s) == nil {
		t.Fatal("bilinmeyen profil kabul edildi")
	}
}
