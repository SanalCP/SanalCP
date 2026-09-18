package github

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"sanalcp/internal/secretcrypt"
)

var repoName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

// CloneURL builds a credential-free URL; user input cannot change the host,
// inject query parameters or escape the owner/repository path.
func CloneURL(repo string) (string, error) {
	parts := strings.Split(repo, "/")
	if !repoName.MatchString(repo) || len(parts) != 2 || parts[1] == "." || parts[1] == ".." || len(repo) > 200 {
		return "", errors.New("geçersiz GitHub deposu (owner/name)")
	}
	return "https://github.com/" + repo + ".git", nil
}

// DeployToken returns credentials only for the selected repository of this
// domain. An arbitrary HTTPS remote must never receive the GitHub PAT.
func DeployToken(ctx context.Context, db *sql.DB, domainID int64, repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", nil
	}
	var encrypted, selected string
	err = db.QueryRowContext(ctx, `SELECT pat, secili_repo FROM github_connections WHERE domain_id=?`, domainID).Scan(&encrypted, &selected)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("GitHub bağlantısı okunamadı")
	}
	expected, err := CloneURL(selected)
	if err != nil || repoURL != expected {
		return "", nil
	}
	if box == nil || !secretcrypt.IsEncrypted(encrypted) {
		return "", errors.New("GitHub token'ı güvenli biçimde okunamadı; bağlantıyı yenileyin")
	}
	token, err := box.Decrypt(encrypted)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", errors.New("GitHub token'ı çözülemedi; bağlantıyı yenileyin")
	}
	return token, nil
}
