package middleware

import "testing"

func TestPanelIPIstisna(t *testing.T) {
	ham := "192.0.2.0/24\n2001:db8::7"
	if !panelIPIstisna("192.0.2.44", ham) || !panelIPIstisna("2001:db8::7", ham) {
		t.Fatal("istisna eşleşmedi")
	}
	if panelIPIstisna("198.51.100.1", ham) {
		t.Fatal("liste dışı IP eşleşti")
	}
}
