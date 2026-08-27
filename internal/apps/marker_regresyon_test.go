package apps_test

import (
	"testing"

	"sanalcp/internal/opencart"
	"sanalcp/internal/phpbb"
)

// Her iki sürücü de eskiden genel config.php marker'ını kullanıyordu. Herhangi
// bir özel PHP sitesi iki uygulama birden kuruluymuş gibi listeleniyordu.
func TestOpenCartVePhpBBMarkerCakismiyor(t *testing.T) {
	openMarker := (opencart.Surucu{}).MarkerDosya()
	phpbbMarker := (phpbb.Surucu{}).MarkerDosya()
	if openMarker == "config.php" || phpbbMarker == "config.php" {
		t.Fatalf("genel config.php marker olarak kullanılamaz: opencart=%q phpbb=%q", openMarker, phpbbMarker)
	}
	if openMarker == phpbbMarker {
		t.Fatalf("uygulama marker'ları çakışıyor: %q", openMarker)
	}
}
