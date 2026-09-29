package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidReleaseRelIsDomainScoped(t *testing.T) {
	if !validReleaseRel(23, ".sanalcp/releases/23/release-222222222222-0123456789abcdef") {
		t.Fatal("valid release rejected")
	}
	for _, rel := range []string{
		".sanalcp/releases/24/release-222222222222-0123456789abcdef",
		".sanalcp/releases/23/../release-222222222222-0123456789abcdef",
		"public_html",
	} {
		if validReleaseRel(23, rel) {
			t.Errorf("unsafe release accepted: %s", rel)
		}
	}
}

func TestWorktreeRemovalCleansBuildArtifacts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(repo, "app.py"), []byte("print('ok')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "app.py")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	release := filepath.Join(filepath.Dir(repo), "release")
	run("worktree", "add", "--detach", release, "HEAD")
	if err := os.MkdirAll(filepath.Join(release, ".venv", "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, ".venv", "bin", "python"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}
	run("worktree", "remove", "--force", release)
	if _, err := os.Stat(release); !os.IsNotExist(err) {
		t.Fatalf("release remains after removal: %v", err)
	}
}
