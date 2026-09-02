package transfers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"sanalcp/internal/dns"
	"sanalcp/internal/mail"
	"sanalcp/internal/provisioner"
)

type nativeDomainMeta struct {
	Version    int    `json:"version"`
	Domain     string `json:"domain"`
	SourceIPv4 string `json:"source_ipv4"`
	DefaultDB  string `json:"default_db"`
}

func nativeDatabaseSources(sources []string, raw []byte) []string {
	out := append([]string(nil), sources...)
	var meta nativeDomainMeta
	if json.Unmarshal(raw, &meta) != nil || meta.DefaultDB == "" {
		return out
	}
	for i, name := range out {
		if name == meta.DefaultDB {
			copy(out[1:i+1], out[0:i])
			out[0] = name
			break
		}
	}
	return out
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

type nativeSecurity struct {
	HotlinkEnabled int    `json:"hotlink_enabled"`
	HotlinkAllowed string `json:"hotlink_allowed"`
	IPAccessMode   string `json:"ip_access_mode"`
	WWWRedirect    int    `json:"www_redirect"`
	WAFEnabled     *int   `json:"waf_enabled"`
	WAFMode        string `json:"waf_mode"`
	WAFParanoia    int    `json:"waf_paranoia"`
}
type nativeRedirect struct {
	TargetURL string `json:"target_url"`
	Code      int    `json:"code"`
}
type nativeIPRule struct {
	CIDR string `json:"cidr"`
}
type nativeNginx struct {
	XContentType   int    `json:"x_content_type"`
	XXSS           int    `json:"x_xss"`
	Referrer       int    `json:"referrer"`
	Permissions    int    `json:"permissions"`
	CSPUpgrade     int    `json:"csp_upgrade"`
	HSTS           int    `json:"hsts"`
	HSTSMaxAge     int    `json:"hsts_max_age"`
	HSTSSubdomains int    `json:"hsts_subdomains"`
	HSTSPreload    int    `json:"hsts_preload"`
	HTTP3          int    `json:"http3"`
	CacheProfile   string `json:"cache_profile"`
}

type nativeRate struct {
	Profile        string `json:"profile"`
	Requests       int    `json:"requests_per_minute"`
	Burst          int    `json:"burst"`
	BlockBots      int    `json:"block_bots"`
	IPExceptions   string `json:"ip_exceptions"`
	PathExceptions string `json:"path_exceptions"`
}
type nativeSpam struct {
	Enabled int     `json:"enabled"`
	Grey    float64 `json:"greylist_score"`
	Header  float64 `json:"add_header_score"`
	Reject  float64 `json:"reject_score"`
}
type nativeAuto struct {
	LocalPart string `json:"local_part"`
	Enabled   int    `json:"enabled"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Interval  int    `json:"interval_days"`
}
type nativeFilter struct {
	LocalPart   string `json:"local_part"`
	Name        string `json:"name"`
	MatchField  string `json:"match_field"`
	MatchValue  string `json:"match_value"`
	ActionType  string `json:"action_type"`
	ActionValue string `json:"action_value"`
	Priority    int    `json:"priority"`
	Enabled     int    `json:"enabled"`
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
	if err := h.importNativePortableSettings(ctx, ekler, domainID); err != nil {
		return nil, err
	}
	return preserved, nil
}

func optionalJSON[T any](raw []byte) (*T, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
func bit(v int) bool { return v == 0 || v == 1 }

func (h *Handlers) importNativePortableSettings(ctx context.Context, e arsivEkler, domainID int64) error {
	sec, err := optionalJSON[nativeSecurity](e.uyeler[e.nativeSecurity])
	if err != nil {
		return fmt.Errorf("güvenlik ayarları: %w", err)
	}
	redir, err := optionalJSON[nativeRedirect](e.uyeler[e.nativeRedirect])
	if err != nil {
		return fmt.Errorf("yönlendirme ayarı: %w", err)
	}
	rules, err := decodeJSONLines[nativeIPRule](e.uyeler[e.nativeIPRules])
	if err != nil {
		return fmt.Errorf("IP kuralları: %w", err)
	}
	ng, err := optionalJSON[nativeNginx](e.uyeler[e.nativeNginx])
	if err != nil {
		return fmt.Errorf("nginx ayarları: %w", err)
	}
	rate, err := optionalJSON[nativeRate](e.uyeler[e.nativeRate])
	if err != nil {
		return fmt.Errorf("hız limiti: %w", err)
	}
	spam, err := optionalJSON[nativeSpam](e.uyeler[e.nativeSpam])
	if err != nil {
		return fmt.Errorf("spam ayarı: %w", err)
	}
	autos, err := decodeJSONLines[nativeAuto](e.uyeler[e.nativeAuto])
	if err != nil {
		return fmt.Errorf("otomatik yanıtlayıcılar: %w", err)
	}
	filters, err := decodeJSONLines[nativeFilter](e.uyeler[e.nativeFilters])
	if err != nil {
		return fmt.Errorf("posta filtreleri: %w", err)
	}

	if sec != nil && (!bit(sec.HotlinkEnabled) || !bit(sec.WWWRedirect) || len(sec.HotlinkAllowed) > 4000 || !oneOf(sec.IPAccessMode, "kapali", "engelle", "izin_ver") || (sec.WAFEnabled != nil && !bit(*sec.WAFEnabled)) || !oneOf(sec.WAFMode, "", "on", "detect", "off") || sec.WAFParanoia < 0 || sec.WAFParanoia > 4) {
		return errors.New("geçersiz güvenlik ayarı")
	}
	if redir != nil {
		u, er := url.ParseRequestURI(redir.TargetURL)
		if er != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(redir.TargetURL) > 2048 || (redir.Code != 301 && redir.Code != 302) {
			return errors.New("geçersiz domain yönlendirmesi")
		}
	}
	for _, r := range rules {
		if !validIPOrCIDR(r.CIDR) {
			return fmt.Errorf("geçersiz IP/CIDR: %s", r.CIDR)
		}
	}
	if ng != nil && (!bits(ng.XContentType, ng.XXSS, ng.Referrer, ng.Permissions, ng.CSPUpgrade, ng.HSTS, ng.HSTSSubdomains, ng.HSTSPreload, ng.HTTP3) || ng.HSTSMaxAge < 0 || ng.HSTSMaxAge > 63072000 || !oneOf(ng.CacheProfile, "kapali", "genel", "wordpress", "prestashop", "ozel")) {
		return errors.New("geçersiz nginx ayarı")
	}
	if rate != nil && (!oneOf(rate.Profile, "kapali", "dengeli", "siki", "ozel") || rate.Requests < 1 || rate.Requests > 60000 || rate.Burst < 0 || rate.Burst > 10000 || !bit(rate.BlockBots) || !validExceptionLines(rate.IPExceptions, true) || !validExceptionLines(rate.PathExceptions, false)) {
		return errors.New("geçersiz hız limiti ayarı")
	}
	if spam != nil && (!bit(spam.Enabled) || spam.Grey < 0 || spam.Reject > 50 || spam.Grey > spam.Header || spam.Header > spam.Reject) {
		return errors.New("geçersiz spam ayarı")
	}
	for _, a := range autos {
		if !localPartRE.MatchString(a.LocalPart) || !bit(a.Enabled) || strings.TrimSpace(a.Subject) == "" || strings.TrimSpace(a.Body) == "" || len(a.Subject) > 255 || len(a.Body) > 10000 || a.Interval < 1 || a.Interval > 30 {
			return errors.New("geçersiz otomatik yanıtlayıcı")
		}
	}
	for _, f := range filters {
		if !validNativeFilter(f) {
			return errors.New("geçersiz posta filtresi")
		}
	}

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if sec != nil {
		var wafEnabled any
		if sec.WAFEnabled != nil {
			wafEnabled = *sec.WAFEnabled
		}
		_, err = tx.ExecContext(ctx, `UPDATE domains SET hotlink_aktif=?,hotlink_izinli=?,ip_erisim_modu=?,www_yonlendir=?,waf_enabled=?,waf_mode=NULLIF(?,''),waf_paranoia=NULLIF(?,0) WHERE id=?`, sec.HotlinkEnabled, sec.HotlinkAllowed, sec.IPAccessMode, sec.WWWRedirect, wafEnabled, sec.WAFMode, sec.WAFParanoia, domainID)
		if err != nil {
			return err
		}
	}
	if redir != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO domain_redirects(domain_id,hedef_url,kod) VALUES(?,?,?) ON DUPLICATE KEY UPDATE hedef_url=VALUES(hedef_url),kod=VALUES(kod)`, domainID, redir.TargetURL, redir.Code)
		if err != nil {
			return err
		}
	}
	_, hasIPRules := e.uyeler[e.nativeIPRules]
	if hasIPRules {
		if _, err = tx.ExecContext(ctx, `DELETE FROM domain_ip_kurallari WHERE domain_id=?`, domainID); err != nil {
			return err
		}
		for _, r := range rules {
			if _, err = tx.ExecContext(ctx, `INSERT INTO domain_ip_kurallari(domain_id,ip_cidr) VALUES(?,?)`, domainID, r.CIDR); err != nil {
				return err
			}
		}
	}
	if ng != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO nginx_settings(domain_id,hdr_x_content_type,hdr_x_xss,hdr_referrer,hdr_permissions,hdr_csp_upgrade,hdr_hsts,hsts_max_age,hsts_subdomains,hsts_preload,ek_direktifler,http3,cache_profili) VALUES(?,?,?,?,?,?,?,?,?,?, '',?,?) ON DUPLICATE KEY UPDATE hdr_x_content_type=VALUES(hdr_x_content_type),hdr_x_xss=VALUES(hdr_x_xss),hdr_referrer=VALUES(hdr_referrer),hdr_permissions=VALUES(hdr_permissions),hdr_csp_upgrade=VALUES(hdr_csp_upgrade),hdr_hsts=VALUES(hdr_hsts),hsts_max_age=VALUES(hsts_max_age),hsts_subdomains=VALUES(hsts_subdomains),hsts_preload=VALUES(hsts_preload),http3=VALUES(http3),cache_profili=VALUES(cache_profili)`, domainID, ng.XContentType, ng.XXSS, ng.Referrer, ng.Permissions, ng.CSPUpgrade, ng.HSTS, ng.HSTSMaxAge, ng.HSTSSubdomains, ng.HSTSPreload, ng.HTTP3, ng.CacheProfile)
		if err != nil {
			return err
		}
	}
	if rate != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO domain_rate_limits(domain_id,profil,istek_dakika,burst,bot_engelle,ip_istisnalari,yol_istisnalari) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE profil=VALUES(profil),istek_dakika=VALUES(istek_dakika),burst=VALUES(burst),bot_engelle=VALUES(bot_engelle),ip_istisnalari=VALUES(ip_istisnalari),yol_istisnalari=VALUES(yol_istisnalari)`, domainID, rate.Profile, rate.Requests, rate.Burst, rate.BlockBots, rate.IPExceptions, rate.PathExceptions)
		if err != nil {
			return err
		}
	}
	if spam != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO mail_spam_settings(domain_id,enabled,greylist_score,add_header_score,reject_score) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE enabled=VALUES(enabled),greylist_score=VALUES(greylist_score),add_header_score=VALUES(add_header_score),reject_score=VALUES(reject_score)`, domainID, spam.Enabled, spam.Grey, spam.Header, spam.Reject)
		if err != nil {
			return err
		}
	}
	mailIDs := map[string]int64{}
	rows, er := tx.QueryContext(ctx, `SELECT local_part,id FROM mailboxes WHERE domain_id=?`, domainID)
	if er != nil {
		return er
	}
	for rows.Next() {
		var l string
		var id int64
		if er = rows.Scan(&l, &id); er != nil {
			rows.Close()
			return er
		}
		mailIDs[l] = id
	}
	if er = rows.Err(); er != nil {
		rows.Close()
		return er
	}
	if er = rows.Close(); er != nil {
		return er
	}
	_, hasAutos := e.uyeler[e.nativeAuto]
	_, hasFilters := e.uyeler[e.nativeFilters]
	if hasAutos {
		if _, err = tx.ExecContext(ctx, `DELETE FROM mail_autoresponders WHERE domain_id=?`, domainID); err != nil {
			return err
		}
	}
	if hasFilters {
		if _, err = tx.ExecContext(ctx, `DELETE FROM mail_filters WHERE domain_id=?`, domainID); err != nil {
			return err
		}
	}
	touched := map[int64]bool{}
	for _, a := range autos {
		id, ok := mailIDs[a.LocalPart]
		if !ok {
			return fmt.Errorf("yanıtlayıcı posta kutusu bulunamadı: %s", a.LocalPart)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mail_autoresponders(mailbox_id,domain_id,enabled,subject_text,body_text,interval_days) VALUES(?,?,?,?,?,?)`, id, domainID, a.Enabled, a.Subject, a.Body, a.Interval)
		if err != nil {
			return err
		}
		touched[id] = true
	}
	for _, f := range filters {
		id, ok := mailIDs[f.LocalPart]
		if !ok {
			return fmt.Errorf("filtre posta kutusu bulunamadı: %s", f.LocalPart)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mail_filters(mailbox_id,domain_id,name,match_field,match_value,action_type,action_value,priority_n,enabled) VALUES(?,?,?,?,?,?,?,?,?)`, id, domainID, f.Name, f.MatchField, f.MatchValue, f.ActionType, f.ActionValue, f.Priority, f.Enabled)
		if err != nil {
			return err
		}
		touched[id] = true
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if rate != nil {
		if err = provisioner.RateLimitGlobalYaz(h.DB); err != nil {
			return err
		}
	}
	if sec != nil || redir != nil || hasIPRules || ng != nil || rate != nil {
		if err = provisioner.RerenderVhost(h.DB, domainID); err != nil {
			return err
		}
	}
	if spam != nil {
		if err = mail.ApplyRspamdSettings(h.DB); err != nil {
			return err
		}
	}
	for id := range touched {
		if err = mail.ApplyMailboxSieve(ctx, h.DB, id); err != nil {
			return err
		}
	}
	return nil
}

func oneOf(s string, vals ...string) bool {
	for _, v := range vals {
		if s == v {
			return true
		}
	}
	return false
}
func bits(v ...int) bool {
	for _, x := range v {
		if !bit(x) {
			return false
		}
	}
	return true
}
func validIPOrCIDR(s string) bool {
	s = strings.TrimSpace(s)
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, e := net.ParseCIDR(s)
	return e == nil
}
func validExceptionLines(s string, ip bool) bool {
	if len(s) > 20000 {
		return false
	}
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if ip {
			if !validIPOrCIDR(x) {
				return false
			}
		} else if !strings.HasPrefix(x, "/") || strings.ContainsAny(x, " \t\"'{};") {
			return false
		}
	}
	return true
}
func validNativeFilter(f nativeFilter) bool {
	if !localPartRE.MatchString(f.LocalPart) || !bit(f.Enabled) || strings.TrimSpace(f.Name) == "" || len(f.Name) > 128 || strings.TrimSpace(f.MatchValue) == "" || len(f.MatchValue) > 255 || !oneOf(f.MatchField, "from", "to", "subject") || f.Priority < 0 || f.Priority > 100000 {
		return false
	}
	switch f.ActionType {
	case "move":
		return len(f.ActionValue) > 0 && len(f.ActionValue) <= 64 && !strings.ContainsAny(f.ActionValue, "\r\n\"{};")
	case "redirect":
		p := strings.Split(f.ActionValue, "@")
		return len(p) == 2 && localPartRE.MatchString(strings.ToLower(p[0])) && domainKesifGecerli(strings.ToLower(p[1]))
	case "discard":
		return f.ActionValue == ""
	}
	return false
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
