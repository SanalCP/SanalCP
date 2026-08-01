package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr    string
	CLIListenAddr string
	DBDsn         string
	JWTSecret     []byte
	JWTLifetime   int // saniye
	Env           string
	// SecretKey: db_pass_plain gibi panel metadata'sındaki müşteri parolalarını
	// AES-256-GCM ile şifrelemek için kullanılan anahtar (bkz. internal/secretcrypt).
	// PANEL_SECRET_KEY'in SHA-256'sı — ham env değeri herhangi bir uzunlukta
	// olabilir, burada her zaman tam 32 bayta indirgenir.
	SecretKey [32]byte
}

func Load() (*Config, error) {
	dsn := strings.TrimSpace(os.Getenv("PANEL_DB_DSN"))
	if dsn == "" {
		return nil, fmt.Errorf("PANEL_DB_DSN zorunlu (varsayılan/zayıf kimlik bilgisiyle sessizce çalışmayı önlemek için sabit kod içinde varsayılan yok)")
	}
	c := &Config{
		ListenAddr:    envOr("PANEL_LISTEN", ":8080"),
		CLIListenAddr: envOr("PANEL_CLI_LISTEN", "127.0.0.1:8090"),
		DBDsn:         dsn,
		Env:           envOr("PANEL_ENV", "production"),
		JWTLifetime:   envInt("PANEL_JWT_LIFETIME_SEC", 8*3600),
	}
	secret := strings.TrimSpace(os.Getenv("PANEL_JWT_SECRET"))
	if len(secret) < 32 {
		return nil, fmt.Errorf("PANEL_JWT_SECRET en az 32 karakter olmalı (mevcut: %d)", len(secret))
	}
	c.JWTSecret = []byte(secret)

	secretKey := strings.TrimSpace(os.Getenv("PANEL_SECRET_KEY"))
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("PANEL_SECRET_KEY en az 32 karakter olmalı (mevcut: %d)", len(secretKey))
	}
	c.SecretKey = sha256.Sum256([]byte(secretKey))
	return c, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
