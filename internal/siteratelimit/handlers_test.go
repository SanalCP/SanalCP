package siteratelimit

import "testing"

func TestIPIstisnalariNormalize(t *testing.T) {
	got, err := ipler([]string{"203.0.113.7", "192.0.2.99/24", "2001:db8::1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"203.0.113.7", "192.0.2.0/24", "2001:db8::1"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestYolIstisnasiTehlikeliGirdiyiReddeder(t *testing.T) {
	if _, err := yollarDogrula([]string{"/ok/*", "/x; return 200"}); err == nil {
		t.Fatal("tehlikeli yol kabul edildi")
	}
}
