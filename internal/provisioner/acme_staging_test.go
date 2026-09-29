package provisioner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ACME_STAGING=1 --issue çağrısına LE staging sunucusunu eklemeli; aksi halde
// (unset veya başka bir değer) prod API'ye dokunulmamalı — varsayılan davranış
// değişmemeli, bayrak yalnız açıkça istendiğinde devreye girmeli.
func TestAcmeServerArgs(t *testing.T) {
	durumlar := []struct {
		ad       string
		degisken string
		beklenen []string
	}{
		{"set edilmemiş", "", nil},
		{"1", "1", []string{"--server", acmeStagingServer}},
		{"0", "0", nil},
		{"true", "true", nil}, // yalnız tam "1" tetikler, başka değer yanlışlıkla staging'e düşürmez
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			if d.degisken == "" {
				t.Setenv("ACME_STAGING", "")
			} else {
				t.Setenv("ACME_STAGING", d.degisken)
			}
			got := AcmeServerArgs()
			if !reflect.DeepEqual(got, d.beklenen) {
				t.Errorf("AcmeServerArgs() = %v, beklenen %v", got, d.beklenen)
			}
		})
	}
}

func TestIssueWithPreflight(t *testing.T) {
	for _, tc := range []struct {
		name        string
		failStaging bool
		manual      bool
		wantCalls   int
	}{
		{"başarılı ön deneme", false, false, 2},
		{"staging başarısızsa üretim yok", true, false, 1},
		{"manuel staging", false, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "acme.sh")
			logPath := filepath.Join(dir, "calls")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_ACME_LOG\"\n" +
				"case \"$*\" in *acme-staging-v02* ) [ \"$TEST_FAIL_STAGING\" = 1 ] && exit 1;; esac\nexit 0\n"
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TEST_ACME_LOG", logPath)
			if tc.failStaging {
				t.Setenv("TEST_FAIL_STAGING", "1")
			} else {
				t.Setenv("TEST_FAIL_STAGING", "0")
			}
			if tc.manual {
				t.Setenv("ACME_STAGING", "1")
			} else {
				t.Setenv("ACME_STAGING", "")
			}

			out, err := issueWithPreflight(context.Background(), bin,
				[]string{"--issue", "--webroot", "/var/www/_acme", "-d", "ornek.com"})
			if (err != nil) != tc.failStaging {
				t.Fatalf("hata = %v, çıktı = %s", err, out)
			}
			if tc.failStaging && !strings.Contains(string(out), "staging ön denemesi") {
				t.Fatalf("staging hata sebebi eksik: %s", out)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			calls := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(calls) != tc.wantCalls {
				t.Fatalf("çağrı sayısı %d, beklenen %d: %s", len(calls), tc.wantCalls, data)
			}
			if !strings.Contains(calls[0], "--server "+acmeStagingServer) {
				t.Fatalf("ilk çağrı staging değil: %s", calls[0])
			}
			if !tc.manual {
				for _, flag := range []string{"--home ", "--config-home ", "--cert-home "} {
					if !strings.Contains(calls[0], flag) {
						t.Errorf("staging çağrısında %s yok: %s", flag, calls[0])
					}
				}
				if strings.Contains(calls[0], "/root/.acme.sh/") {
					t.Errorf("staging canlı depoya işaret ediyor: %s", calls[0])
				}
			}
			if tc.wantCalls == 2 && !strings.Contains(calls[1], "--server letsencrypt") {
				t.Errorf("üretim çağrısı LE sunucusuna sabitlenmedi: %s", calls[1])
			}
		})
	}
}
