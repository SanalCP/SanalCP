package siteratelimit

import (
	"reflect"
	"testing"
)

func TestOlayJSONEtiketleriAyrikAlanlariKorur(t *testing.T) {
	typ, ok := reflect.TypeOf(olay{}).FieldByName("Zaman")
	if !ok || typ.Tag.Get("json") != "zaman" {
		t.Fatalf("Zaman JSON etiketi %q, beklenen %q", typ.Tag.Get("json"), "zaman")
	}
	for _, alan := range []struct {
		ad  string
		bek string
	}{
		{"IP", "ip"},
		{"Yol", "yol"},
	} {
		f, ok := reflect.TypeOf(olay{}).FieldByName(alan.ad)
		if !ok {
			t.Fatalf("%s alanı bulunamadı", alan.ad)
		}
		if got := f.Tag.Get("json"); got != alan.bek {
			t.Errorf("%s JSON etiketi %q, beklenen %q", alan.ad, got, alan.bek)
		}
	}
}
