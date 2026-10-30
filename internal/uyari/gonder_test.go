package uyari

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
