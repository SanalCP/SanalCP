package transfers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sanalcp/internal/dns"
)

type nativeDomainMeta struct {
	Version    int    `json:"version"`
	Domain     string `json:"domain"`
	SourceIPv4 string `json:"source_ipv4"`
}

type nativeDNSRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Active   int    `json:"active"`
}

type nativeMailbox struct {
	LocalPart    string `json:"local_part"`
	PasswordHash string `json:"password_hash"`
	QuotaBytes   uint64 `json:"quota_bytes"`
	Status       string `json:"status"`
}

// importNativeMetadata yalnız native SanalCP üyeleri mevcutsa çalışır. cPanel,
// Plesk ve DirectAdmin arşivlerinin davranışını değiştirmez.
func (h *Handlers) importNativeMetadata(ctx context.Context, ekler arsivEkler, domainID int64) (map[string]bool, error) {
	preserved := map[string]bool{}
	domainRaw, native := ekler.uyeler[ekler.nativeDomain]
	if !native {
		return preserved, nil
	}
	var meta nativeDomainMeta
	if json.Unmarshal(domainRaw, &meta) != nil || meta.Version != 1 || !domainKesifGecerli(strings.ToLower(meta.Domain)) {
		return nil, errors.New("native manifest geçersiz veya desteklenmiyor")
	}

	var targetDomain, targetIPv4 string
	if err := h.DB.QueryRowContext(ctx, `SELECT alan_adi,ipv4 FROM domains WHERE id=?`, domainID).Scan(&targetDomain, &targetIPv4); err != nil {
		return nil, err
	}

	dnsRows, err := decodeJSONLines[nativeDNSRecord](ekler.uyeler[ekler.nativeDNS])
	if err != nil {
		return nil, fmt.Errorf("DNS metadata: %w", err)
	}
	if len(dnsRows) > 0 {
		tx, err := h.DB.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `DELETE FROM dns_records WHERE domain_id=?`, domainID); err != nil {
			return nil, err
		}
		for _, rec := range dnsRows {
			rec.Type = strings.ToUpper(strings.TrimSpace(rec.Type))
			if !nativeDNSSafe(rec) {
				return nil, fmt.Errorf("geçersiz DNS kaydı: %s %s", rec.Name, rec.Type)
			}
			if rec.Type == "A" && meta.SourceIPv4 != "" && strings.TrimSpace(rec.Value) == meta.SourceIPv4 && targetIPv4 != "" {
				rec.Value = targetIPv4
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO dns_records(domain_id,ad,tip,deger,ttl,oncelik,aktif) VALUES(?,?,?,?,?,?,?)`, domainID, rec.Name, rec.Type, rec.Value, rec.TTL, rec.Priority, rec.Active); err != nil {
				return nil, err
			}
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		if err = dns.WriteZone(ctx, h.DB, domainID); err != nil {
			return nil, err
		}
	}

	mailRows, err := decodeJSONLines[nativeMailbox](ekler.uyeler[ekler.nativeMailbox])
	if err != nil {
		return nil, fmt.Errorf("posta metadata: %w", err)
	}
	for _, box := range mailRows {
		local := strings.ToLower(strings.TrimSpace(box.LocalPart))
		if !localPartRE.MatchString(local) || len(box.PasswordHash) < 10 || len(box.PasswordHash) > 255 {
			return nil, fmt.Errorf("geçersiz posta kutusu metadata: %q", local)
		}
		if box.Status != "active" && box.Status != "suspended" {
			return nil, fmt.Errorf("geçersiz posta kutusu durumu")
		}
		res, err := h.DB.ExecContext(ctx, `UPDATE mailboxes SET password_hash=?,quota_bytes=?,status=? WHERE domain_id=? AND local_part=?`, box.PasswordHash, box.QuotaBytes, box.Status, domainID, local)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return nil, fmt.Errorf("posta kutusu hedefte bulunamadı: %s", local)
		}
		preserved[local+"@"+targetDomain] = true
	}
	return preserved, nil
}

func nativeDNSSafe(r nativeDNSRecord) bool {
	validType := false
	for _, typ := range dns.GecerliTipler {
		if r.Type == typ {
			validType = true
			break
		}
	}
	return validType && r.Name != "" && len(r.Name) <= 253 && r.Value != "" && len(r.Value) <= 65535 && r.TTL >= 60 && r.TTL <= 604800 && r.Priority >= 0 && r.Priority <= 65535 && (r.Active == 0 || r.Active == 1)
}

func decodeJSONLines[T any](raw []byte) ([]T, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []T{}, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), int(maxMetadataBytes))
	var out []T
	for scanner.Scan() {
		var item T
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
