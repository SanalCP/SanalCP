package mail

import (
	"strings"
	"testing"
)

func TestSieveEscaping(t *testing.T) {
	if got := sieveQuote("a\"b\\c"); got != `"a\"b\\c"` {
		t.Fatalf("sieveQuote = %s", got)
	}
	if got := sieveMultiline("bir\n.nokta\r\nson"); got != "bir\n..nokta\nson" {
		t.Fatalf("sieveMultiline = %q", got)
	}
}

// Sieve quoted-string'de satır sonu kaçışı yoktur — `\n` yazmak "n" harfine
// çözülür ve eşleşme değerini sessizce bozardı. Boşluğa çevrilmeli.
func TestSieveQuoteSatirSonu(t *testing.T) {
	got := sieveQuote("bir\r\niki\nüç\rdört")
	if want := `"bir iki üç dört"`; got != want {
		t.Fatalf("sieveQuote = %s, beklenen %s", got, want)
	}
	if strings.Contains(got, `\n`) {
		t.Fatalf("çıktıda geçersiz \\n kaçışı kaldı: %s", got)
	}
}

func TestValidateFilter(t *testing.T) {
	valid := []MailFilter{
		{Name: "konu", MatchField: "subject", MatchValue: "fatura", ActionType: "move", ActionValue: "Faturalar"},
		{Name: "yonlendir", MatchField: "from", MatchValue: "@firma.com", ActionType: "redirect", ActionValue: "arsiv@example.com"},
		{Name: "sil", MatchField: "to", MatchValue: "eski@", ActionType: "discard"},
	}
	for _, filter := range valid {
		if err := validateFilter(filter); err != nil {
			t.Errorf("geçerli filtre reddedildi: %v", err)
		}
	}
	invalid := []MailFilter{
		{Name: "", MatchField: "subject", MatchValue: "x", ActionType: "discard"},
		{Name: "x", MatchField: "body", MatchValue: "x", ActionType: "discard"},
		{Name: "x", MatchField: "from", MatchValue: "x", ActionType: "move", ActionValue: "../kaçış"},
		{Name: "x", MatchField: "from", MatchValue: "x", ActionType: "redirect", ActionValue: "geçersiz"},
	}
	for _, filter := range invalid {
		if err := validateFilter(filter); err == nil {
			t.Errorf("geçersiz filtre kabul edildi: %+v", filter)
		}
	}
}
