package config

import (
	"crypto/sha256"
	"fmt"
	"net"
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
	// TrustedProxyCIDRs: X-Forwarded-For/X-Real-IP başlıklarına yalnızca bu
	// CIDR'lardan doğrudan bağlanan taraflar için güvenilir (bkz. httpx.ClientIP).
	// Varsayılan yalnızca loopback (nginx her zaman 127.0.0.1/::1 üzerinden panele
	// bağlanır, bkz. assets/nginx/_panel.conf) — panel yanlışlıkla dışarıya açılırsa
	// (ör. PANEL_LISTEN=0.0.0.0) dışarıdan gelen bağlantının RemoteAddr'ı loopback
	// olamayacağı için XFF/X-Real-IP otomatik olarak yok sayılır (fail-closed).
	TrustedProxyCIDRs []*net.IPNet
	// AVMaxConcurrent: aynı anda en fazla kaç clamscan/freshclam çalışabilir.
	// ClamAV imza DB'si ~1.5 GB RAM tutar; N tane paralel clamscan = ~1.5N GB.
	// 3.8 GB'lık kutuda default 1 zorunlu, daha büyük bellekli kutularda 2-3
	// ayarlanabilir. PANEL_AV_MAX_CONCURRENT ile override edilir.
	AVMaxConcurrent int
}

func defaultTrustedProxyCIDRs() []*net.IPNet {
	var out []*net.IPNet
	for _, s := range []string{"127.0.0.1/32", "::1/128"} {
		_, cidr, err := net.ParseCIDR(s)
		if err == nil {
			out = append(out, cidr)
		}
	}
	return out
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
		AVMaxConcurrent: envInt("PANEL_AV_MAX_CONCURRENT", 1),
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

	if raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			_, cidr, err := net.ParseCIDR(part)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS geçersiz CIDR içeriyor (%q): %w", part, err)
			}
			c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, cidr)
		}
	} else {
		c.TrustedProxyCIDRs = defaultTrustedProxyCIDRs()
	}

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
