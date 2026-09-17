package git

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"sanalcp/internal/jailpath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

// GitHub'ın HTTPS üzerinden yayımladığı Ed25519 host key:
// https://api.github.com/meta (2026-09-17). Ağdan TOFU/keyscan yapılmaz.
const githubKnownHost = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"

var deployKeyLocks [64]sync.Mutex

func generateDeployKey(sk string) (string, error) {
	home, err := jailpath.TenantHome(sk)
	if err != nil {
		return "", err
	}
	uid, gid, ok := jailpath.TenantIDs(sk)
	if !ok || uid == 0 {
		return "", errors.New("tenant kullanıcısı bulunamadı")
	}
	return generateDeployKeyAt(home, uid, gid)
}

// Anahtar bellekte üretilir; tenant'ın kontrol ettiği yollarda root olarak
// ssh-keygen/chown/restorecon çalıştırılmaz. Dizinin fd'si tüm işlemde pinlidir.
func generateDeployKeyAt(home string, uid, gid int) (string, error) {
	hash := sha256.Sum256([]byte(home))
	mu := &deployKeyLocks[int(hash[0])%len(deployKeyLocks)]
	mu.Lock()
	defer mu.Unlock()
	dir, err := openSSHDir(home, uid, gid)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	fd := int(dir.Fd())
	var signer ssh.Signer
	keyfd, err := unix.Openat(fd, "sanalcp_deploy", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == nil {
		f := os.NewFile(uintptr(keyfd), "deploy-key")
		defer f.Close()
		var st unix.Stat_t
		if err := unix.Fstat(keyfd, &st); err != nil {
			return "", err
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG || int(st.Uid) != uid || st.Nlink != 1 || st.Size > 16384 {
			return "", errors.New("güvenli olmayan deploy anahtarı")
		}
		b, err := io.ReadAll(io.LimitReader(f, 16385))
		if err != nil || len(b) > 16384 {
			return "", errors.New("deploy anahtarı okunamadı")
		}
		signer, err = ssh.ParsePrivateKey(b)
		if err != nil {
			return "", fmt.Errorf("deploy anahtarı geçersiz: %w", err)
		}
		if err := f.Chmod(0600); err != nil {
			return "", err
		}
	} else if errors.Is(err, unix.ENOENT) {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return "", err
		}
		block, err := ssh.MarshalPrivateKey(key, "deploy@sanalcp")
		if err != nil {
			return "", err
		}
		if err := writeSSHFile(fd, "sanalcp_deploy", pem.EncodeToMemory(block), 0600, uid, gid); err != nil {
			return "", err
		}
		signer, err = ssh.NewSignerFromKey(key)
		if err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	pub := ssh.MarshalAuthorizedKey(signer.PublicKey())
	if err := writeSSHFile(fd, "sanalcp_deploy.pub", pub, 0644, uid, gid); err != nil {
		return "", err
	}
	if err := writeSSHFile(fd, "sanalcp_known_hosts", []byte(githubKnownHost), 0644, uid, gid); err != nil {
		return "", err
	}
	config := "Host github.com\n    HostName github.com\n    User git\n    IdentityFile ~/.ssh/sanalcp_deploy\n    StrictHostKeyChecking yes\n    UserKnownHostsFile ~/.ssh/known_hosts ~/.ssh/sanalcp_known_hosts\n"
	if err := writeSSHFile(fd, "config", []byte(config), 0600, uid, gid); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(pub)), nil
}

func openSSHDir(home string, uid, gid int) (*os.File, error) {
	h, err := jailpath.AcDizin(home, ".")
	if err != nil {
		return nil, err
	}
	defer h.Close()
	if err := unix.Mkdirat(int(h.Fd()), ".ssh", 0700); err != nil && err != unix.EEXIST {
		return nil, err
	}
	fd, err := unix.Openat(int(h.Fd()), ".ssh", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), ".ssh")
	if err := f.Chown(uid, gid); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Chmod(0700); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Yeni inode + rename: mevcut hedef symlink/hardlink olsa bile hedefi yazmaz.
func writeSSHFile(dirfd int, name string, data []byte, mode uint32, uid, gid int) error {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	tmp := ".sanalcp-" + hex.EncodeToString(b)
	fd, err := unix.Openat(dirfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(dirfd, tmp, 0)
	f := os.NewFile(uintptr(fd), tmp)
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chown(uid, gid); err != nil {
		return err
	}
	if err := f.Chmod(os.FileMode(mode)); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return unix.Renameat(dirfd, tmp, dirfd, name)
}

func prepareKnownHosts(home string, uid, gid int) error {
	f, err := openSSHDir(home, uid, gid)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeSSHFile(int(f.Fd()), "sanalcp_known_hosts", []byte(githubKnownHost), 0644, uid, gid)
}

func gitEnvironment(home string) []string {
	// home yalnız TenantHome'un doğruladığı c_<slug> değerinden üretilir.
	return []string{
		"HOME=" + home,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -i " + home + "/.ssh/sanalcp_deploy -o 'UserKnownHostsFile=" + home + "/.ssh/known_hosts " + home + "/.ssh/sanalcp_known_hosts'",
	}
}
