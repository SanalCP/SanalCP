// Package planfeatures defines customer management grants; runtime services are unaffected.
package planfeatures

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type Module string

const (
	DNS       Module = "dns"
	Mail      Module = "mail"
	Databases Module = "databases"
	Files     Module = "files"
	FTP       Module = "ftp"
	Cron      Module = "cron"
	Backups   Module = "backups"
	Git       Module = "git"
)

type Features struct {
	DNS       bool `json:"dns"`
	Mail      bool `json:"mail"`
	Databases bool `json:"databases"`
	Files     bool `json:"files"`
	FTP       bool `json:"ftp"`
	Cron      bool `json:"cron"`
	Backups   bool `json:"backups"`
	Git       bool `json:"git"`
}

func All() Features { return Features{true, true, true, true, true, true, true, true} }

func (f Features) Allows(module Module) bool {
	switch module {
	case DNS:
		return f.DNS
	case Mail:
		return f.Mail
	case Databases:
		return f.Databases
	case Files:
		return f.Files
	case FTP:
		return f.FTP
	case Cron:
		return f.Cron
	case Backups:
		return f.Backups
	case Git:
		return f.Git
	default:
		return false
	}
}

// UnmarshalJSON accepts exactly the eight boolean grants. A missing value is not
// equivalent to false: partial settings and corrupt stored values are errors.
func (f *Features) UnmarshalJSON(data []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || values == nil || len(values) != 8 {
		return errors.New("features: sekiz boolean modül alanı zorunlu")
	}
	var parsed Features
	for _, field := range [...]struct {
		key    string
		target *bool
	}{
		{"dns", &parsed.DNS}, {"mail", &parsed.Mail}, {"databases", &parsed.Databases},
		{"files", &parsed.Files}, {"ftp", &parsed.FTP}, {"cron", &parsed.Cron},
		{"backups", &parsed.Backups}, {"git", &parsed.Git},
	} {
		raw, ok := values[field.key]
		raw = bytes.TrimSpace(raw)
		if !ok || (!bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false"))) {
			return fmt.Errorf("features: %s boolean olmalı; yalnız sekiz bilinen alan kabul edilir", field.key)
		}
		*field.target = bytes.Equal(raw, []byte("true"))
	}
	*f = parsed
	return nil
}

// Stored NULL alone denotes a legacy plan with no feature settings. Empty,
// partial, JSON null or otherwise malformed settings must never become grants.
func Stored(value sql.NullString) (Features, error) {
	if !value.Valid {
		return All(), nil
	}
	var f Features
	err := json.Unmarshal([]byte(value.String), &f)
	return f, err
}

// Resolve reads the canonical hosting account plan afresh. Addon rows carry a
// creation-time plan copy, so using that copy would restore disabled management
// access after the parent's plan changes. A broken parent/plan chain fails shut.
func Resolve(ctx context.Context, db *sql.DB, domainID int64) (Features, error) {
	if db == nil || domainID <= 0 {
		return Features{}, errors.New("plan bilgisi okunamadı")
	}
	var seen [32]int64
	for depth := range seen {
		for _, previous := range seen[:depth] {
			if previous == domainID {
				return Features{}, errors.New("döngüsel domain hesabı")
			}
		}
		seen[depth] = domainID
		var parent, planID, foundPlan sql.NullInt64
		var stored sql.NullString
		err := db.QueryRowContext(ctx, `SELECT d.ana_domain_id, d.plan_id, p.id, p.features
			FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id WHERE d.id=?`, domainID).
			Scan(&parent, &planID, &foundPlan, &stored)
		if err != nil {
			return Features{}, err
		}
		if parent.Valid {
			if parent.Int64 <= 0 {
				return Features{}, errors.New("geçersiz ana domain")
			}
			domainID = parent.Int64
			continue
		}
		if !planID.Valid {
			return All(), nil
		}
		if !foundPlan.Valid || foundPlan.Int64 != planID.Int64 {
			return Features{}, errors.New("plan bulunamadı")
		}
		return Stored(stored)
	}
	return Features{}, errors.New("domain hesabı zinciri çok uzun")
}
