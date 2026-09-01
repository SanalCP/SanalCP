package transfers

import "testing"

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
