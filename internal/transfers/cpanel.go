// Package transfers implements provider-neutral hosting account discovery.
// The first adapter understands cPanel full-account (cpmove) tar archives.
package transfers

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	maxArchiveEntries = 1_000_000
	maxExpandedBytes  = int64(100 << 30) // inventory guard; no data is extracted here
	maxMetadataBytes  = int64(2 << 20)
)

var domainRE = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,62}\.)+[a-z]{2,63}$`)

var (
	ErrNotCPanel       = errors.New("arşiv bir cPanel tam hesap yedeği olarak tanınmadı")
	ErrUnsafeArchive   = errors.New("güvenlik: arşiv güvenli olmayan bir üye içeriyor")
	ErrArchiveTooLarge = errors.New("güvenlik: arşiv envanter sınırını aşıyor")
)

type Inventory struct {
	Provider      string   `json:"provider"`
	Username      string   `json:"username"`
	PrimaryDomain string   `json:"primary_domain"`
	ArchiveRoot   string   `json:"archive_root"`
	EntryCount    int      `json:"entry_count"`
	ExpandedBytes int64    `json:"expanded_bytes"`
	WebFiles      int      `json:"web_files"`
	WebBytes      int64    `json:"web_bytes"`
	Databases     []string `json:"databases"`
	DNSZones      []string `json:"dns_zones"`
	MailFiles     int      `json:"mail_files"`
	CronPresent   bool     `json:"cron_present"`
	SSLCerts      int      `json:"ssl_certs"`
	Warnings      []string `json:"warnings"`
}

// AnalyzeCPanel reads a gzip-compressed cPanel full backup without extracting it.
func AnalyzeCPanel(src io.Reader) (Inventory, error) {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return Inventory{}, fmt.Errorf("gzip açılamadı: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	inv := Inventory{Provider: "cpanel", Databases: []string{}, DNSZones: []string{}, Warnings: []string{}}
	dbSet := map[string]bool{}
	dnsSet := map[string]bool{}
	seenCPanel := false

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Inventory{}, fmt.Errorf("tar okunamadı: %w", err)
		}
		inv.EntryCount++
		if inv.EntryCount > maxArchiveEntries || h.Size < 0 || inv.ExpandedBytes > maxExpandedBytes-h.Size {
			return Inventory{}, ErrArchiveTooLarge
		}
		inv.ExpandedBytes += h.Size
		if unsafeMember(h) {
			return Inventory{}, fmt.Errorf("%w: %q", ErrUnsafeArchive, h.Name)
		}

		clean := cleanMember(h.Name)
		root, rel := splitArchiveRoot(clean)
		if inv.ArchiveRoot == "" && root != "" {
			inv.ArchiveRoot = root
		}
		if strings.HasPrefix(rel, "cp/") || rel == "cp" || strings.HasPrefix(rel, "homedir/") {
			seenCPanel = true
		}

		switch {
		case strings.HasPrefix(rel, "homedir/public_html/") && h.Typeflag == tar.TypeReg:
			inv.WebFiles++
			inv.WebBytes += h.Size
		case strings.HasPrefix(rel, "homedir/mail/") && h.Typeflag == tar.TypeReg:
			inv.MailFiles++
		case rel == "cron" || strings.HasPrefix(rel, "cron/"):
			inv.CronPresent = true
		case strings.HasPrefix(rel, "sslcerts/") && h.Typeflag == tar.TypeReg:
			inv.SSLCerts++
		case strings.HasPrefix(rel, "mysql/") && strings.HasSuffix(strings.ToLower(rel), ".sql"):
			name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
			if name != "" && !dbSet[name] {
				dbSet[name] = true
				inv.Databases = append(inv.Databases, name)
			}
		case strings.HasPrefix(rel, "dnszones/") && h.Typeflag == tar.TypeReg:
			name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
			if domainRE.MatchString(name) && !dnsSet[name] {
				dnsSet[name] = true
				inv.DNSZones = append(inv.DNSZones, strings.ToLower(name))
			}
		}

		if h.Typeflag == tar.TypeReg && h.Size <= maxMetadataBytes {
			switch {
			case rel == "cp/backup_user" || rel == "cp/username":
				if b, e := io.ReadAll(io.LimitReader(tr, maxMetadataBytes)); e == nil {
					inv.Username = strings.TrimSpace(string(b))
				}
			case strings.HasPrefix(rel, "cp/userdata/") && strings.HasSuffix(rel, "/main"):
				if b, e := io.ReadAll(io.LimitReader(tr, maxMetadataBytes)); e == nil {
					parseMainMetadata(&inv, string(b), rel)
				}
			}
		}
	}
	if !seenCPanel {
		return Inventory{}, ErrNotCPanel
	}
	if inv.PrimaryDomain == "" && len(inv.DNSZones) > 0 {
		inv.PrimaryDomain = inv.DNSZones[0]
		inv.Warnings = append(inv.Warnings, "Ana domain metadata yerine DNS zone bilgisinden belirlendi.")
	}
	if inv.PrimaryDomain == "" {
		inv.Warnings = append(inv.Warnings, "Ana domain otomatik belirlenemedi; içe aktarmadan önce seçilmelidir.")
	}
	if inv.WebFiles == 0 {
		inv.Warnings = append(inv.Warnings, "public_html altında web dosyası bulunamadı.")
	}
	sort.Strings(inv.Databases)
	sort.Strings(inv.DNSZones)
	return inv, nil
}

func unsafeMember(h *tar.Header) bool {
	n := strings.ReplaceAll(h.Name, "\\", "/")
	if strings.HasPrefix(n, "/") {
		return true
	}
	for _, p := range strings.Split(n, "/") {
		if p == ".." {
			return true
		}
	}
	switch h.Typeflag {
	case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		return true
	}
	return false
}

func cleanMember(name string) string {
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
}

func splitArchiveRoot(name string) (root, rel string) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 && (strings.HasPrefix(parts[0], "backup-") || strings.HasPrefix(parts[0], "cpmove-")) {
		return parts[0], parts[1]
	}
	return "", name
}

func parseMainMetadata(inv *Inventory, body, rel string) {
	parts := strings.Split(rel, "/")
	if inv.Username == "" && len(parts) >= 4 {
		inv.Username = parts[2]
	}
	for _, line := range strings.Split(body, "\n") {
		p := strings.SplitN(line, ":", 2)
		if len(p) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(p[0]))
		value := strings.Trim(strings.TrimSpace(p[1]), `"'`)
		if (key == "main_domain" || key == "domain") && domainRE.MatchString(value) {
			inv.PrimaryDomain = strings.ToLower(value)
			return
		}
	}
}
