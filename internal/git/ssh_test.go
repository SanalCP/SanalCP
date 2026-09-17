package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestDeployKeyReuseAndPermissions(t *testing.T) {
	home := t.TempDir()
	pub, err := generateDeployKeyAt(home, os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, ".ssh", "sanalcp_deploy")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if pub != strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) {
		t.Fatal("key mismatch")
	}
	again, err := generateDeployKeyAt(home, os.Getuid(), os.Getgid())
	if err != nil || pub != again {
		t.Fatalf("existing key rotated: %v", err)
	}
	for name, mode := range map[string]os.FileMode{".": 0700, "sanalcp_deploy": 0600, "config": 0600, "sanalcp_deploy.pub": 0644} {
		st, err := os.Stat(filepath.Join(home, ".ssh", name))
		if err != nil || st.Mode().Perm() != mode {
			t.Fatalf("%s mode: %v %v", name, st, err)
		}
	}
}

func TestDeployKeySymlinksCannotEscape(t *testing.T) {
	for _, name := range []string{".ssh", "config", "sanalcp_deploy", "sanalcp_deploy.pub", "sanalcp_known_hosts"} {
		t.Run(name, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "secret")
			if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if name == ".ssh" {
				if err := os.Symlink(outside, filepath.Join(home, ".ssh")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(filepath.Join(home, ".ssh"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(home, ".ssh", name)); err != nil {
					t.Fatal(err)
				}
			}
			_, err := generateDeployKeyAt(home, os.Getuid(), os.Getgid())
			if (name == ".ssh" || name == "sanalcp_deploy") && err == nil {
				t.Fatal("unsafe key/directory accepted")
			}
			if name != ".ssh" && name != "sanalcp_deploy" && err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "unchanged" {
				t.Fatalf("outside modified: %v", err)
			}
			st, _ := os.Stat(target)
			if st.Mode().Perm() != 0600 {
				t.Fatal("outside permissions modified")
			}
		})
	}
}

func TestGitSSHOverridesLegacyInsecureConfig(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH unavailable")
	}
	home := t.TempDir()
	config := filepath.Join(home, "legacy-config")
	if err := os.WriteFile(config, []byte("Host *\n StrictHostKeyChecking no\n UserKnownHostsFile /dev/null\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var command string
	for _, e := range gitEnvironment(home) {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			command = strings.TrimPrefix(e, "GIT_SSH_COMMAND=")
		}
	}
	// -G only evaluates configuration; it never connects to a server.
	out, err := exec.Command("sh", "-c", command+" -F "+config+" -G github.com").Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "stricthostkeychecking true") || !strings.Contains(text, "batchmode yes") || strings.Contains(text, "userknownhostsfile /dev/null") {
		t.Fatalf("unsafe effective SSH config: %s", text)
	}
	if !strings.Contains(text, "sanalcp_known_hosts") {
		t.Fatal("pinned GitHub host key missing")
	}
}
