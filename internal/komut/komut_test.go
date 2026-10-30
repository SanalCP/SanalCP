package komut

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCalistirBasarili(t *testing.T) {
	if err := Calistir(context.Background(), "true"); err != nil {
		t.Fatalf("true çalıştırılamadı: %v", err)
	}
}

func TestCalistirHataGecirir(t *testing.T) {
	if err := Calistir(context.Background(), "false"); err == nil {
		t.Fatal("false hata döndürmeliydi")
	}
}

// TestCalistirTakilanAraciOldurur: asılı kalan bir araç tavan süre dolunca
// KESİLMELİ. Bu test yoksa takılan bir sistem aracı istek goroutine'ini
// süresiz bloklayabilir (paketin varlık sebebi).
func TestCalistirTakilanAraciOldurur(t *testing.T) {
	baslangic := time.Now()
	err := calistirZ(context.Background(), 50*time.Millisecond, "sleep", "5")
	if err == nil {
		t.Fatal("sleep 5, 50ms tavanda kesilmeliydi")
	}
	if gecen := time.Since(baslangic); gecen > 2*time.Second {
		t.Fatalf("tavan süresi uygulanmadı: %v geçti", gecen)
	}
}

// TestCalistirContextIptali: çağıran context iptal edilirse child beklenmez.
func TestCalistirContextIptali(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := calistirZ(ctx, VarsayilanSure, "sleep", "5"); err == nil {
		t.Fatal("iptal edilmiş context'te sleep hata döndürmeliydi")
	}
}

func TestCiktiYalnizStdout(t *testing.T) {
	out, err := Cikti(context.Background(), "echo", "merhaba")
	if err != nil {
		t.Fatalf("echo: %v", err)
	}
	if strings.TrimSpace(string(out)) != "merhaba" {
		t.Fatalf("stdout bekleniyordu, gelen: %q", out)
	}
}

func TestBirlesikCiktiStderrDahil(t *testing.T) {
	// stderr'e yazan ama başarılı biten komut: birleşik çıktı stderr'i de içermeli.
	out, err := BirlesikCikti(context.Background(), "sh", "-c", "echo yan-kanal >&2")
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	if !strings.Contains(string(out), "yan-kanal") {
		t.Fatalf("birleşik çıktı stderr içermeli, gelen: %q", out)
	}
}
