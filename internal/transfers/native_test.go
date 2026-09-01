package transfers

import (
	"encoding/json"
	"testing"
)

func TestDecodeJSONLines(t *testing.T) {
	raw := []byte("{\"name\":\"@\",\"type\":\"A\",\"value\":\"192.0.2.1\",\"ttl\":300,\"priority\":0,\"active\":1}\n" +
		"{\"name\":\"www\",\"type\":\"CNAME\",\"value\":\"example.com\",\"ttl\":3600,\"priority\":0,\"active\":1}\n")
	rows, err := decodeJSONLines[nativeDNSRecord](raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "@" || rows[1].Type != "CNAME" {
		t.Fatalf("beklenmeyen JSONL: %+v", rows)
	}
}

func TestNativeDNSSafe(t *testing.T) {
	valid := nativeDNSRecord{Name: "@", Type: "A", Value: "192.0.2.1", TTL: 300, Active: 1}
	if !nativeDNSSafe(valid) {
		t.Fatal("geçerli DNS kaydı reddedildi")
	}
	valid.Type = "SHELL"
	if nativeDNSSafe(valid) {
		t.Fatal("desteklenmeyen DNS tipi kabul edildi")
	}
}

func TestDecodeJSONLinesBozukSatiriReddeder(t *testing.T) {
	if _, err := decodeJSONLines[nativeMailbox]([]byte("{bozuk}\n")); err == nil {
		t.Fatal("bozuk native metadata kabul edildi")
	}
}

func TestNativeNginxJSONAlanlari(t *testing.T) {
	var got nativeNginx
	err := json.Unmarshal([]byte(`{"x_content_type":1,"x_xss":0,"referrer":1,"permissions":0,"csp_upgrade":1,"hsts":1,"hsts_max_age":31536000,"hsts_subdomains":1,"hsts_preload":0,"http3":1,"cache_profile":"wordpress"}`), &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.XContentType != 1 || got.XXSS != 0 || got.HTTP3 != 1 || got.CacheProfile != "wordpress" {
		t.Fatalf("nginx metadata alanları çözülemedi: %+v", got)
	}
}

func TestNativeTasimaGirdiDogrulama(t *testing.T) {
	if !validIPOrCIDR("192.0.2.0/24") || validIPOrCIDR("192.0.2.999") {
		t.Fatal("IP/CIDR doğrulaması beklenmeyen sonuç verdi")
	}
	if !validExceptionLines("/wp-login.php\n/xmlrpc.php", false) || validExceptionLines("/ok; return 200", false) {
		t.Fatal("yol istisnası doğrulaması beklenmeyen sonuç verdi")
	}
	good := nativeFilter{LocalPart: "admin", Name: "Sipariş", MatchField: "subject", MatchValue: "order", ActionType: "move", ActionValue: "Orders", Priority: 100, Enabled: 1}
	if !validNativeFilter(good) {
		t.Fatal("geçerli posta filtresi reddedildi")
	}
	good.ActionValue = `Bad"; discard;`
	if validNativeFilter(good) {
		t.Fatal("tehlikeli Sieve klasör değeri kabul edildi")
	}
}

func TestNativeChildMetadataHelpers(t *testing.T) {
	for _, v := range []string{"7.4", "8.3", "8.10"} {
		if !validPHPVersion(v) {
			t.Fatalf("geçerli PHP sürümü reddedildi: %s", v)
		}
	}
	for _, v := range []string{"", "83", "8.x", "8.3;id"} {
		if validPHPVersion(v) {
			t.Fatalf("geçersiz PHP sürümü kabul edildi: %s", v)
		}
	}
	addons := []nativeAddonDomain{{Domain: "site.test", Parked: 0}, {Domain: "park.test", Parked: 1}}
	got := addonNames(addons)
	if len(got) != 1 || got[0] != "site.test" {
		t.Fatalf("parked domain için gereksiz docroot seçildi: %v", got)
	}
}

func TestNativeAddonDNSJSON(t *testing.T) {
	rows, err := decodeJSONLines[nativeAddonDNSRecord]([]byte(`{"domain":"blog.test","name":"@","type":"A","value":"192.0.2.4","ttl":300,"priority":0,"active":1}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Domain != "blog.test" || rows[0].Type != "A" || rows[0].Value != "192.0.2.4" {
		t.Fatalf("addon DNS metadata çözülemedi: %+v", rows)
	}
}
