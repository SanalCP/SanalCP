package provisioner

import (
	"reflect"
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
