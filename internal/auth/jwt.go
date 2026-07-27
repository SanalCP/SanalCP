package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"rol"`
	Version  uint64 `json:"ver"`
	jwt.RegisteredClaims
}

func Issue(secret []byte, lifetimeSec int, uid int64, username, role string, version uint64) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:   uid,
		Username: username,
		Role:     role,
		Version:  version,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(lifetimeSec) * time.Second)),
			Issuer:    "sanalpanel",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(secret)
}

func Parse(secret []byte, raw string) (*Claims, error) {
	if raw == "" {
		return nil, errors.New("boş token")
	}
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("beklenmeyen alg")
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, errors.New("geçersiz token")
	}
	if c.Issuer != "sanalpanel" || c.Role == "" {
		return nil, errors.New("admin token değil")
	}
	return c, nil
}
