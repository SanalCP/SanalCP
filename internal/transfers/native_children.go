package transfers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"strings"

	"sanalcp/internal/adlar"
	"sanalcp/internal/altalangoc"
	"sanalcp/internal/archivex"
	"sanalcp/internal/dns"
	"sanalcp/internal/provisioner"
)

type nativeSubdomain struct {
	Label      string `json:"label"`
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version"`
}

type nativeAddonDomain struct {
	Domain     string `json:"domain"`
	Parked     int    `json:"parked"`
	PHPVersion string `json:"php_version"`
}

type nativeAddonDNSRecord struct {
	Domain string `json:"domain"`
	nativeDNSRecord
}

func (r *nativeAddonDNSRecord) UnmarshalJSON(b []byte) error {
	type wire struct {
		Domain   string `json:"domain"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Value    string `json:"value"`
		TTL      int    `json:"ttl"`
		Priority int    `json:"priority"`
		Active   int    `json:"active"`
	}
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	r.Domain = w.Domain
	r.nativeDNSRecord = nativeDNSRecord{Name: w.Name, Type: w.Type, Value: w.Value, TTL: w.TTL, Priority: w.Priority, Active: w.Active}
	return nil
}

func (h *Handlers) importNativeChildDomains(r *http.Request, archivePath string, e arsivEkler, inv Inventory, parentID int64, sk string) (int, int, error) {
	if _, native := e.uyeler[e.nativeDomain]; !native {
		return 0, 0, nil
	}
	subs, err := decodeJSONLines[nativeSubdomain](e.uyeler[e.nativeSubs])
	if err != nil {
		return 0, 0, fmt.Errorf("alt alan metadata: %w", err)
	}
	addons, err := decodeJSONLines[nativeAddonDomain](e.uyeler[e.nativeAddons])
	if err != nil {
		return 0, 0, fmt.Errorf("ek alan metadata: %w", err)
	}
	if len(subs) > 0 && h.Domains == nil {
		return 0, 0, errors.New("domain sağlayıcısı hazır değil")
	}
	if len(addons) > 0 && h.Addon == nil {
		return 0, 0, errors.New("ek alan sağlayıcısı hazır değil")
	}
	seen := map[string]bool{strings.ToLower(inv.PrimaryDomain): true}
	for i := range subs {
		subs[i].Label = strings.ToLower(strings.TrimSpace(subs[i].Label))
		subs[i].Domain = strings.ToLower(strings.TrimSpace(subs[i].Domain))
		if !adlar.EtiketGecerli(subs[i].Label) || subs[i].Domain != subs[i].Label+"."+strings.ToLower(inv.PrimaryDomain) || !validPHPVersion(subs[i].PHPVersion) || seen[subs[i].Domain] {
			return 0, 0, fmt.Errorf("geçersiz alt alan metadata: %q", subs[i].Domain)
		}
		seen[subs[i].Domain] = true
	}
	for i := range addons {
		addons[i].Domain = strings.ToLower(strings.TrimSpace(addons[i].Domain))
		if provisioner.ValidateDomain(addons[i].Domain) != nil || !bit(addons[i].Parked) || !validPHPVersion(addons[i].PHPVersion) || seen[addons[i].Domain] {
			return 0, 0, fmt.Errorf("geçersiz ek alan metadata: %q", addons[i].Domain)
		}
		seen[addons[i].Domain] = true
	}

	// Alt alan adları hedefte BAĞIMSIZ domain olarak açılır — panelde alt alan
	// adı sistemi kaldırıldı (CloudPanel modeli). Kaynak panel eski sürüm
	// olabileceği için arşiv hâlâ subdomains.jsonl taşıyor; aktarımın kayıpsız
	// kalması için bunlar atlanmaz, yükseltilir.
	//
	// Sahiplik ana domainden devralınır: aktarılan alt alan adı, ana domainin
	// müşterisinde görünmeli.
	subSK := make(map[string]string, len(subs))
	if len(subs) > 0 {
		customerID, sahipBayi := altalangoc.Sahiplik(r.Context(), h.DB, parentID)
		for i := range subs {
			_, yeniSK, err := altalangoc.TenantOlustur(r.Context(), h.DB,
				subs[i].Domain, subs[i].PHPVersion, h.Domains.IPv4, customerID, sahipBayi)
			if err != nil {
				return i, 0, fmt.Errorf("%s: %w", subs[i].Domain, err)
			}
			subSK[subs[i].Domain] = yeniSK
		}
	}
	for i := range addons {
		body := []byte(fmt.Sprintf(`{"alan_adi":%q,"parked":%t,"php_surum":%q}`, addons[i].Domain, addons[i].Parked == 1, addons[i].PHPVersion))
		req := domainRequest(r, http.MethodPost, "/ek", parentID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Addon.Olustur(rr, req)
		if rr.Code < 200 || rr.Code >= 300 {
			return len(subs), i, fmt.Errorf("%s: %s", addons[i].Domain, strings.TrimSpace(rr.Body.String()))
		}
	}
	// Alt alan adı dosyaları ana tenant'ın ~/subdomains'ine DEĞİL, her birinin
	// kendi yeni tenant'ının public_html'ine açılır.
	if err := restoreNativeSubdomainTrees(archivePath, inv.ArchiveRoot, subSK); err != nil {
		return len(subs), len(addons), fmt.Errorf("alt alan dosyaları: %w", err)
	}
	if err := restoreNativeChildTrees(archivePath, inv.ArchiveRoot, sk, "domains", addonNames(addons)); err != nil {
		return len(subs), len(addons), fmt.Errorf("ek alan dosyaları: %w", err)
	}
	if err := h.importNativeAddonMetadata(r.Context(), e, inv, parentID, addons); err != nil {
		return len(subs), len(addons), err
	}
	return len(subs), len(addons), nil
}

func (h *Handlers) importNativeAddonMetadata(ctx context.Context, e arsivEkler, inv Inventory, parentID int64, addons []nativeAddonDomain) error {
	if len(addons) == 0 {
		return nil
	}
	ids := map[string]int64{}
	rows, err := h.DB.QueryContext(ctx, `SELECT alan_adi,id FROM domains WHERE ana_domain_id=?`, parentID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var domain string
		var id int64
		if err = rows.Scan(&domain, &id); err != nil {
			rows.Close()
			return err
		}
		ids[domain] = id
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, a := range addons {
		if ids[a.Domain] <= 0 {
			return fmt.Errorf("hedef ek alan bulunamadı: %s", a.Domain)
		}
	}

	dnsRows, err := decodeJSONLines[nativeAddonDNSRecord](e.uyeler[e.nativeAddonDNS])
	if err != nil {
		return fmt.Errorf("ek alan DNS metadata: %w", err)
	}
	grouped := map[string][]nativeDNSRecord{}
	var sourceIP, targetIP string
	var meta nativeDomainMeta
	_ = json.Unmarshal(e.uyeler[e.nativeDomain], &meta)
	sourceIP = meta.SourceIPv4
	_ = h.DB.QueryRowContext(ctx, `SELECT ipv4 FROM domains WHERE id=?`, parentID).Scan(&targetIP)
	for _, row := range dnsRows {
		row.Domain = strings.ToLower(strings.TrimSpace(row.Domain))
		row.Type = strings.ToUpper(strings.TrimSpace(row.Type))
		if ids[row.Domain] <= 0 || !nativeDNSSafe(row.nativeDNSRecord) {
			return fmt.Errorf("geçersiz ek alan DNS kaydı: %s", row.Domain)
		}
		if row.Type == "A" && sourceIP != "" && strings.TrimSpace(row.Value) == sourceIP && targetIP != "" {
			row.Value = targetIP
		}
		grouped[row.Domain] = append(grouped[row.Domain], row.nativeDNSRecord)
	}
	_, hasDNS := e.uyeler[e.nativeAddonDNS]
	if hasDNS {
		for _, a := range addons {
			tx, er := h.DB.BeginTx(ctx, nil)
			if er != nil {
				return er
			}
			if _, er = tx.ExecContext(ctx, `DELETE FROM dns_records WHERE domain_id=?`, ids[a.Domain]); er != nil {
				tx.Rollback()
				return er
			}
			for _, rec := range grouped[a.Domain] {
				if _, er = tx.ExecContext(ctx, `INSERT INTO dns_records(domain_id,ad,tip,deger,ttl,oncelik,aktif) VALUES(?,?,?,?,?,?,?)`, ids[a.Domain], rec.Name, rec.Type, rec.Value, rec.TTL, rec.Priority, rec.Active); er != nil {
					tx.Rollback()
					return er
				}
			}
			if er = tx.Commit(); er != nil {
				return er
			}
			if er = dns.WriteZone(ctx, h.DB, ids[a.Domain]); er != nil {
				return er
			}
		}
	}
	for _, a := range addons {
		base := e.nativeAddonBase(a.Domain)
		child := arsivEkler{nativeSecurity: base + "/security.json", nativeRedirect: base + "/redirect.json", nativeIPRules: base + "/ip_rules.jsonl", nativeNginx: base + "/nginx.json", nativeRate: base + "/rate_limit.json", uyeler: e.uyeler}
		if err = h.importNativePortableSettings(ctx, child, ids[a.Domain]); err != nil {
			return fmt.Errorf("%s ayarları: %w", a.Domain, err)
		}
		cert, key := e.uyeler[base+"/cert.pem"], e.uyeler[base+"/key.pem"]
		if len(cert) > 0 || len(key) > 0 {
			if len(cert) == 0 || len(key) == 0 {
				return fmt.Errorf("%s SSL çifti eksik", a.Domain)
			}
			certPath, keyPath, expires, er := provisioner.InstallImportedSSL(a.Domain, cert, key)
			if er != nil {
				return fmt.Errorf("%s SSL: %w", a.Domain, er)
			}
			if _, er = h.DB.ExecContext(ctx, `UPDATE domains SET ssl_aktif=1,ssl_kaynak='imported',cert_path=?,key_path=?,ssl_bitis=? WHERE id=?`, certPath, keyPath, expires, ids[a.Domain]); er != nil {
				return er
			}
			if er = provisioner.RerenderVhost(h.DB, ids[a.Domain]); er != nil {
				return er
			}
		}
	}
	return nil
}

func (e arsivEkler) nativeAddonBase(domain string) string {
	return path.Dir(e.nativeAddons) + "/addons/" + domain
}

func validPHPVersion(s string) bool {
	if len(s) < 3 || len(s) > 8 {
		return false
	}
	for i, r := range s {
		if i == 1 {
			if r != '.' {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// restoreNativeSubdomainTrees: arşivdeki her alt alan adı ağacını KENDİ yeni
// tenant'ının public_html'ine açar.
//
// restoreNativeChildTrees'ten farkı hedefin ad başına değişmesi: üye yolu
// root/homedir/subdomains/<ad>/… olduğundan 4 bileşen atılır ve içerik doğrudan
// /home/<yeniSK>/public_html altına düşer. Arşiv ad başına bir kez açılır —
// tek tar çağrısı bütün adları alamaz, çünkü her adın hedefi farklı.
func restoreNativeSubdomainTrees(archivePath, root string, adSK map[string]string) error {
	if len(adSK) == 0 {
		return nil
	}
	if root == "" {
		return errors.New("güvensiz kaynak")
	}
	if err := archivex.Tara(archivePath, archivex.TurTarGz); err != nil {
		return err
	}
	for ad, sk := range adSK {
		if !adlar.SKGecerli(sk) || provisioner.ValidateDomain(ad) != nil {
			return errors.New("güvensiz alt alan hedefi: " + ad)
		}
		target := "/home/" + sk + "/public_html"
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		cmd := exec.Command("runuser", "-u", sk, "--",
			"tar", "-xz", "-f", "-", "-C", target, "--strip-components=4",
			root+"/homedir/subdomains/"+ad)
		cmd.Stdin = f
		out, err := cmd.CombinedOutput()
		f.Close()
		if err != nil {
			// "Not found" = kaynakta o ağaç yok; boş bir alt alan adı aktarımı
			// düşürmemeli (restoreNativeChildTrees ile aynı tolerans).
			if strings.Contains(string(out), "Not found") {
				continue
			}
			return fmt.Errorf("tar %s: %s", ad, strings.TrimSpace(string(out)))
		}
		_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	}
	return nil
}
func addonNames(in []nativeAddonDomain) []string {
	out := []string{}
	for _, x := range in {
		if x.Parked == 0 {
			out = append(out, x.Domain)
		}
	}
	return out
}

func restoreNativeChildTrees(archivePath, root, sk, kind string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	if !adlar.SKGecerli(sk) || root == "" || (kind != "subdomains" && kind != "domains") {
		return errors.New("güvensiz hedef")
	}
	if err := archivex.Tara(archivePath, archivex.TurTarGz); err != nil {
		return err
	}
	target := "/home/" + sk + "/" + kind
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	args := []string{"-u", sk, "--", "tar", "-xz", "-f", "-", "-C", target, "--strip-components=3"}
	for _, name := range names {
		if provisioner.ValidateDomain(name) != nil {
			return errors.New("güvensiz alt alan yolu")
		}
		args = append(args, root+"/homedir/"+kind+"/"+name)
	}
	cmd := exec.Command("runuser", args...)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "Not found") {
			return nil
		}
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	return nil
}
