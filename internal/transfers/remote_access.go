package transfers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"sanalcp/internal/httpx"
)

var (
	transferSSHDir       = "/etc/sanalcp/transfer-ssh"
	transferIdentityPath = "/etc/sanalcp/transfer-ssh/id_ed25519"
	transferKnownHosts   = "/etc/sanalcp/transfer-ssh/known_hosts"
	transferKeyMu        sync.Mutex
)

type hostKeyReq struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Fingerprints []string `json:"fingerprints"`
}

type hostKeyInfo struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Fingerprints []string `json:"fingerprints"`
}

// RemoteAccessKey hedef panelin yalnız aktarımlarda kullanılan SSH anahtarını
// üretir. Özel anahtar API'den hiçbir zaman dönmez; kullanıcı kaynak sunucuya
// yalnız public key'i ekler.
func (h *Handlers) RemoteAccessKey(w http.ResponseWriter, r *http.Request) {
	transferKeyMu.Lock()
	defer transferKeyMu.Unlock()
	if err := os.MkdirAll(transferSSHDir, 0o700); err != nil {
		httpx.WriteError(w, 500, "aktarım SSH dizini oluşturulamadı")
		return
	}
	if _, err := os.Stat(transferIdentityPath); errors.Is(err, os.ErrNotExist) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "sanalcp-transfer", "-f", transferIdentityPath).CombinedOutput()
		if err != nil {
			httpx.WriteError(w, 500, "SSH anahtarı üretilemedi: "+kisalt(strings.TrimSpace(string(out)), 300))
			return
		}
	}
	_ = os.Chmod(transferIdentityPath, 0o600)
	pub, err := os.ReadFile(transferIdentityPath + ".pub")
	if err != nil {
		httpx.WriteError(w, 500, "SSH public key okunamadı")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"public_key": strings.TrimSpace(string(pub))})
}

// RemoteHostKeyScan host key'i henüz güvenilir saymadan parmak izlerini gösterir.
func (h *Handlers) RemoteHostKeyScan(w http.ResponseWriter, r *http.Request) {
	req, ok := hostKeyIstegi(w, r)
	if !ok {
		return
	}
	info, _, err := hostKeyTara(r.Context(), req.Host, req.Port)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, info)
}

// RemoteHostKeyTrust TOCTOU/DNS değişimini önlemek için onay anında yeniden
// tarar ve yalnız kullanıcının gördüğü parmak izleri birebir aynıysa kaydeder.
func (h *Handlers) RemoteHostKeyTrust(w http.ResponseWriter, r *http.Request) {
	req, ok := hostKeyIstegi(w, r)
	if !ok {
		return
	}
	info, lines, err := hostKeyTara(r.Context(), req.Host, req.Port)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	want := append([]string{}, req.Fingerprints...)
	sort.Strings(want)
	if len(want) == 0 || strings.Join(want, "\n") != strings.Join(info.Fingerprints, "\n") {
		httpx.WriteError(w, http.StatusConflict, "SSH host key değişti; parmak izini yeniden doğrulayın")
		return
	}
	transferKeyMu.Lock()
	defer transferKeyMu.Unlock()
	if err := os.MkdirAll(transferSSHDir, 0o700); err != nil {
		httpx.WriteError(w, 500, "known_hosts dizini oluşturulamadı")
		return
	}
	existing, _ := os.ReadFile(transferKnownHosts)
	hostField := req.Host
	if req.Port != 22 {
		hostField = "[" + req.Host + "]:" + strconv.Itoa(req.Port)
	}
	var out strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(existing))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != hostField {
			out.WriteString(line + "\n")
		}
	}
	for _, line := range lines {
		out.WriteString(line + "\n")
	}
	tmp, err := os.CreateTemp(transferSSHDir, "known-hosts-*")
	if err != nil {
		httpx.WriteError(w, 500, "known_hosts hazırlanamadı")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.WriteString(out.String())
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, transferKnownHosts)
	}
	if err != nil {
		httpx.WriteError(w, 500, "known_hosts kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func hostKeyIstegi(w http.ResponseWriter, r *http.Request) (hostKeyReq, bool) {
	var req hostKeyReq
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return req, false
	}
	req.Host = strings.TrimSpace(req.Host)
	if req.Port == 0 {
		req.Port = 22
	}
	if !uzakHostGecerli(req.Host) || req.Port < 1 || req.Port > 65535 {
		httpx.WriteError(w, 400, "geçersiz SSH hedefi")
		return req, false
	}
	return req, true
}

func hostKeyTara(parent context.Context, host string, port int) (hostKeyInfo, []string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh-keyscan", "-T", "5", "-p", strconv.Itoa(port), host).Output()
	if err != nil || len(out) == 0 {
		return hostKeyInfo{}, nil, fmt.Errorf("SSH host key alınamadı")
	}
	lines, fps, err := hostKeyCoz(out)
	if err != nil {
		return hostKeyInfo{}, nil, err
	}
	return hostKeyInfo{Host: host, Port: port, Fingerprints: fps}, lines, nil
}

func hostKeyCoz(raw []byte) ([]string, []string, error) {
	var lines, fps []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(fields[1:], " ")))
		if err != nil {
			continue
		}
		fp := ssh.FingerprintSHA256(key)
		if !seen[fp] {
			seen[fp] = true
			fps = append(fps, fp)
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, nil, errors.New("geçerli SSH host key bulunamadı")
	}
	sort.Strings(fps)
	return lines, fps, nil
}

func uzakSSHArgs(port int) []string {
	args := []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=yes", "-oConnectTimeout=10", "-oConnectionAttempts=1"}
	if _, err := os.Stat(transferIdentityPath); err == nil {
		args = append(args, "-i", transferIdentityPath, "-oIdentitiesOnly=yes")
	}
	if _, err := os.Stat(transferKnownHosts); err == nil {
		args = append(args, "-oUserKnownHostsFile="+filepath.Clean(transferKnownHosts))
	}
	return append(args, "-p", strconv.Itoa(port))
}
