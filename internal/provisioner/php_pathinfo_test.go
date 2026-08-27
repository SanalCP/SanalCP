package provisioner

import (
	"strings"
	"testing"
)

func TestVhostPHPPathInfoWebDAVDestegi(t *testing.T) {
	sablon := vhostTmpl.Tree.Root.String()
	if got := strings.Count(sablon, `location ~ \.php(?:$|/)`); got != 2 {
		t.Fatalf("SSL ve HTTP PHP PATH_INFO location'ları bekleniyordu, adet=%d", got)
	}
	if got := strings.Count(sablon, `try_files $fastcgi_script_name =404;`); got != 2 {
		t.Fatalf("PATH_INFO script varlık koruması iki blokta bulunmalı, adet=%d", got)
	}
}
