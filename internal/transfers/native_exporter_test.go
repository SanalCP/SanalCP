package transfers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeExporterInventoryProtokolu(t *testing.T) {
	bin := t.TempDir()
	yaz := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	yaz("id", `echo 0`)
	yaz("curl", `echo '{"surum":"0.9.99"}'`)
	yaz("mysql", `echo 'SITE=example.com|c_example|8.3'`)

	script, err := filepath.Abs("../../assets/ops/sanalcp-transfer-export")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, "inventory")
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exporter: %v: %s", err, out)
	}
	got := string(out)
	for _, want := range []string{"PROVIDER=sanalcp", "VERSION=0.9.99", "SITE=example.com|c_example|8.3"} {
		if !strings.Contains(got, want) {
			t.Errorf("çıktıda %q yok: %s", want, got)
		}
	}
}
