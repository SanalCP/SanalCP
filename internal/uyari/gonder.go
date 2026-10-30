package uyari

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"

	"sanalcp/internal/secretcrypt"
)

var box *secretcrypt.Box

// Init: telegram_token_enc şifreleme kutusu.
func Init(b *secretcrypt.Box) { box = b }

// Ayar: panel_ayarlari'ndaki bildirim kanalları (token çözülmüş halde).
type Ayar struct {
	Acik                          bool
	TelegramToken, TelegramChatID string
	Eposta, Dil                   string
}

func (a Ayar) kanalVar() bool {
	return (a.TelegramToken != "" && a.TelegramChatID != "") || a.Eposta != ""
}

func AyarOku(ctx context.Context, db *sql.DB) (Ayar, error) {
	var a Ayar
	var enc string
	if err := db.QueryRowContext(ctx, `SELECT bildirim_acik, COALESCE(telegram_token_enc,''), telegram_chat_id, bildirim_eposta, varsayilan_dil FROM panel_ayarlari WHERE id=1`).
		Scan(&a.Acik, &enc, &a.TelegramChatID, &a.Eposta, &a.Dil); err != nil {
		return a, fmt.Errorf("bildirim ayarı okunamadı: %w", err)
	}
	if enc != "" {
		if box == nil {
			return a, errors.New("şifreleme kutusu hazır değil")
		}
		tok, err := box.Decrypt(enc)
		if err != nil {
			return a, errors.New("telegram token çözülemedi")
		}
		a.TelegramToken = tok
	}
	return a, nil
}

// Gonderici: üretimde Uretim; testler adresleri httptest'e çevirir.
type Gonderici struct {
	TelegramURL string
	SMTPAddr    string
	HTTP        *http.Client
}

// telegramMaks: Telegram mesaj sınırı 4096 karakter; payı bırakıp kesiyoruz.
const telegramMaks = 4000

// Gonder: ayarlı her kanala gönderir; hatalar token içermez.
func (g Gonderici) Gonder(ctx context.Context, a Ayar, metin string) error {
	var hatalar []error
	if a.TelegramToken != "" && a.TelegramChatID != "" {
		hatalar = append(hatalar, g.telegram(ctx, a, metin))
	}
	if a.Eposta != "" {
		hatalar = append(hatalar, g.eposta(a, metin))
	}
	return errors.Join(hatalar...)
}

func (g Gonderici) telegram(ctx context.Context, a Ayar, metin string) error {
	if r := []rune(metin); len(r) > telegramMaks {
		metin = string(r[:telegramMaks]) + "…"
	}
	form := url.Values{"chat_id": {a.TelegramChatID}, "text": {metin}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.TelegramURL+"/bot"+a.TelegramToken+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("telegram: istek oluşturulamadı")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.HTTP.Do(req)
	if err != nil {
		// url.Error mesajı token'lı URL'yi içerir; yalnız iç hatayı döndür.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("telegram: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (g Gonderici) eposta(a Ayar, metin string) error {
	host, _ := os.Hostname()
	kimden := "sanalcp@" + host
	konu := "[SanalCP] Uyarı"
	if a.Dil == "en" {
		konu = "[SanalCP] Alert"
	}
	msg := "From: " + kimden + "\r\nTo: " + a.Eposta + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", konu) +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + metin + "\r\n"
	if err := smtp.SendMail(g.SMTPAddr, nil, kimden, []string{a.Eposta}, []byte(msg)); err != nil {
		return fmt.Errorf("e-posta: %v", err)
	}
	return nil
}
