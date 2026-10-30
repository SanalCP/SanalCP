package uyari

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramGonderir(t *testing.T) {
	var yol, chat, metin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		yol, chat, metin = r.URL.Path, r.PostForm.Get("chat_id"), r.PostForm.Get("text")
	}))
	defer srv.Close()
	g := Gonderici{TelegramURL: srv.URL, HTTP: srv.Client()}
	if err := g.Gonder(context.Background(), Ayar{TelegramToken: "TOK", TelegramChatID: "42"}, "merhaba"); err != nil {
		t.Fatal(err)
	}
	if yol != "/botTOK/sendMessage" || chat != "42" || metin != "merhaba" {
		t.Fatalf("yol=%q chat=%q metin=%q", yol, chat, metin)
	}
}

func TestTelegramHataTokenSizlar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized TOK", http.StatusUnauthorized)
	}))
	defer srv.Close()
	g := Gonderici{TelegramURL: srv.URL, HTTP: srv.Client()}
	err := g.Gonder(context.Background(), Ayar{TelegramToken: "TOK", TelegramChatID: "42"}, "x")
	if err == nil || strings.Contains(err.Error(), "TOK") {
		t.Fatalf("hata bekleniyordu, token içermemeli: %v", err)
	}
	// Bağlantı hatası (url.Error URL'yi, dolayısıyla token'ı içerir) da sızmamalı.
	g.TelegramURL = "http://127.0.0.1:1"
	err = g.Gonder(context.Background(), Ayar{TelegramToken: "TOK", TelegramChatID: "42"}, "x")
	if err == nil || strings.Contains(err.Error(), "TOK") {
		t.Fatalf("bağlantı hatası token sızdırdı: %v", err)
	}
}

func TestKanalYoksaHicbirSeyYapmaz(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
	defer srv.Close()
	g := Gonderici{TelegramURL: srv.URL, SMTPAddr: "127.0.0.1:1", HTTP: srv.Client()}
	if err := g.Gonder(context.Background(), Ayar{}, "x"); err != nil || n.Load() != 0 {
		t.Fatalf("err=%v istek=%d", err, n.Load())
	}
}

// sahteSMTP: STARTTLS sunan ama sertifikası doğrulanamayacak yerel MTA taklidi.
func sahteSMTP(t *testing.T, cevapVer bool) (addr string, komutlar chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	komutlar = make(chan string, 64)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if !cevapVer {
			time.Sleep(5 * time.Second)
			return
		}
		r := bufio.NewReader(c)
		fmt.Fprint(c, "220 test\r\n")
		veri := false
		for {
			satir, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if veri {
				if satir == ".\r\n" {
					veri = false
					fmt.Fprint(c, "250 ok\r\n")
				}
				continue
			}
			komutlar <- strings.ToUpper(strings.Fields(satir + " x")[0])
			switch {
			case strings.HasPrefix(strings.ToUpper(satir), "EHLO"):
				fmt.Fprint(c, "250-test\r\n250 STARTTLS\r\n")
			case strings.HasPrefix(strings.ToUpper(satir), "STARTTLS"):
				fmt.Fprint(c, "454 TLS yok\r\n")
			case strings.HasPrefix(strings.ToUpper(satir), "DATA"):
				veri = true
				fmt.Fprint(c, "354 go\r\n")
			case strings.HasPrefix(strings.ToUpper(satir), "QUIT"):
				fmt.Fprint(c, "221 bye\r\n")
				return
			default:
				fmt.Fprint(c, "250 ok\r\n")
			}
		}
	}()
	return ln.Addr().String(), komutlar
}

func TestEpostaYerelMTAdaSTARTTLSDenemez(t *testing.T) {
	addr, komutlar := sahteSMTP(t, true)
	g := Gonderici{SMTPAddr: addr}
	if err := g.Gonder(context.Background(), Ayar{Eposta: "a@b.com"}, "merhaba"); err != nil {
		t.Fatalf("yerel MTA'ya gönderim başarısız: %v", err)
	}
	close(komutlar)
	for k := range komutlar {
		if k == "STARTTLS" {
			t.Fatal("loopback MTA'da STARTTLS denendi (self-signed sertifika x509 hatası verir)")
		}
	}
}

func TestEpostaYanitsizMTAdaTakilmaz(t *testing.T) {
	eski := smtpZamanAsimi
	smtpZamanAsimi = 200 * time.Millisecond
	t.Cleanup(func() { smtpZamanAsimi = eski })
	addr, _ := sahteSMTP(t, false)
	basla := time.Now()
	err := Gonderici{SMTPAddr: addr}.Gonder(context.Background(), Ayar{Eposta: "a@b.com"}, "x")
	if err == nil || time.Since(basla) > 2*time.Second {
		t.Fatalf("zaman aşımı bekleniyordu: err=%v süre=%v", err, time.Since(basla))
	}
}
