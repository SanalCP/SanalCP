package transfers

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"

	"sanalcp/internal/adlar"
	"sanalcp/internal/archivex"
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
	if len(subs) > 0 && h.Subdomain == nil {
		return 0, 0, errors.New("alt alan sağlayıcısı hazır değil")
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

	for i := range subs {
		body := []byte(fmt.Sprintf(`{"alt_ad":%q,"php_surum":%q}`, subs[i].Label, subs[i].PHPVersion))
		req := domainRequest(r, http.MethodPost, "/subdomain", parentID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Subdomain.Olustur(rr, req)
		if rr.Code < 200 || rr.Code >= 300 {
			return i, 0, fmt.Errorf("%s: %s", subs[i].Domain, strings.TrimSpace(rr.Body.String()))
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
	if err := restoreNativeChildTrees(archivePath, inv.ArchiveRoot, sk, "subdomains", subNames(subs)); err != nil {
		return len(subs), len(addons), fmt.Errorf("alt alan dosyaları: %w", err)
	}
	if err := restoreNativeChildTrees(archivePath, inv.ArchiveRoot, sk, "domains", addonNames(addons)); err != nil {
		return len(subs), len(addons), fmt.Errorf("ek alan dosyaları: %w", err)
	}
	return len(subs), len(addons), nil
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

func subNames(in []nativeSubdomain) []string {
	out := make([]string, 0, len(in))
	for _, x := range in {
		out = append(out, x.Domain)
	}
	return out
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
