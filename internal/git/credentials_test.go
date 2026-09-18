package git

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentialURLAndOutput(t *testing.T) {
	const token = "ghp_test_secret"
	for _, raw := range []string{"https://" + token + "@github.com/owner/repo.git", "https://user:" + token + "@github.com/owner/repo.git"} {
		if gecerliRepoURL(raw) {
			t.Fatal("credential URL accepted")
		}
		if cleanRepoURL(raw) != "https://github.com/owner/repo.git" {
			t.Fatal("URL not cleaned")
		}
		out := redactGitOutput(raw+" "+token+" "+base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token)), token)
		if strings.Contains(out, token) || strings.Contains(out, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))) {
			t.Fatal("output exposed credentials")
		}
	}
	for _, raw := range []string{"https://github.com/owner/repo.git", "git@github.com:owner/repo.git", "ssh://git@example.org/repo.git"} {
		if !gecerliRepoURL(raw) || cleanRepoURL(raw) != raw {
			t.Fatalf("clean URL rejected: %s", raw)
		}
	}
}

// A real Git clone/fetch against an isolated TLS smart-HTTP server verifies
// authentication without argv/config/reflog leakage. No GitHub network access.
func TestGitHTTPSRuntimeCredentials(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	baseEnv := append(gitEnvironment(root), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	run := func(env []string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, gitPath, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, redactGitOutput(string(out), "test-only-pat"))
		}
		return string(out)
	}
	bare, work := filepath.Join(root, "repo.git"), filepath.Join(root, "work")
	run(baseEnv, "init", "--bare", bare)
	run(baseEnv, "init", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run(baseEnv, "-C", work, "add", "README")
	run(baseEnv, "-C", work, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-m", "test")
	run(baseEnv, "-C", work, "push", bare, "main")
	backend := &cgi.Handler{Path: gitPath, Args: []string{"http-backend"}, Dir: root, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	var authenticated, unexpected, redirected atomic.Int32
	var redirect atomic.Bool
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1); w.WriteHeader(404) }))
	defer sink.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/repo.git/") {
			if r.Header.Get("Authorization") != "" {
				unexpected.Add(1)
			}
			w.WriteHeader(404)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "x-access-token" || pass != "test-only-pat" {
			w.WriteHeader(401)
			return
		}
		authenticated.Add(1)
		if redirect.Load() {
			http.Redirect(w, r, sink.URL+"/stolen", http.StatusFound)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	defer server.Close()
	cert := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	repoURL := server.URL + "/repo.git"
	env := append(append([]string{}, baseEnv...), "GIT_SSL_CAINFO="+cert)
	env = append(env, gitCredentialEnvironment(repoURL, "test-only-pat")...)
	dst := filepath.Join(root, "clone")
	run(env, "clone", "--branch", "main", "--", repoURL, dst)
	run(env, "-C", dst, "fetch", "--", repoURL, "main")
	run(baseEnv, "-C", dst, "reset", "--hard", "FETCH_HEAD")
	if authenticated.Load() < 2 {
		t.Fatal("clone/fetch not authenticated")
	}
	if got := strings.TrimSpace(run(baseEnv, "-C", dst, "remote", "get-url", "origin")); got != repoURL {
		t.Fatal("origin URL changed")
	}
	for _, rel := range []string{".git/config", ".git/FETCH_HEAD", ".git/logs/HEAD"} {
		b, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "test-only-pat") || strings.Contains(string(b), "Authorization") {
			t.Fatalf("secret persisted in %s", rel)
		}
	}
	cmd := exec.CommandContext(ctx, gitPath, "ls-remote", server.URL+"/other.git")
	cmd.Env = env
	_ = cmd.Run()
	if unexpected.Load() != 0 {
		t.Fatal("header leaked to other repository")
	}
	redirect.Store(true)
	cmd = exec.CommandContext(ctx, gitPath, "ls-remote", repoURL)
	cmd.Env = env
	if err := cmd.Run(); err == nil || redirected.Load() != 0 {
		t.Fatal("redirect was followed")
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "test-only-pat") {
		t.Fatal("token in argv")
	}
}
