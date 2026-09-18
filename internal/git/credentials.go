package git

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var credentialURL = regexp.MustCompile(`(?i)(https?://)[^\s/"'<>]+@`)

func redactGitOutput(out, token string) string {
	out = credentialURL.ReplaceAllString(out, "${1}")
	if token != "" {
		for _, secret := range []string{base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token)), base64.StdEncoding.EncodeToString([]byte(token + ":")), url.QueryEscape(token), url.PathEscape(token), token} {
			out = strings.ReplaceAll(out, secret, "[REDACTED]")
		}
	}
	return out
}

func cleanRepoURL(raw string) string {
	if strings.HasPrefix(raw, "git@") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" || u.Scheme == "http" {
		u.User = nil
		// Repo URLs have no credential-bearing query/fragment in panel flows.
		u.RawQuery, u.Fragment = "", ""
		return u.String()
	}
	return raw // SSH usernames (git@host) are not PATs.
}

// Runtime configuration stays in the child environment, never argv or a Git
// config file. The header is scoped to the exact selected HTTPS repository.
// https://git-scm.com/docs/git-config#Documentation/git-config.txt-GITCONFIGCOUNT
func gitCredentialEnvironment(repoURL, token string) []string {
	if token == "" {
		return nil
	}
	settings := [][2]string{
		{"credential.helper", ""},
		{"http.extraHeader", ""},
		{"http." + repoURL + ".extraHeader", ""},
		{"http." + repoURL + ".extraHeader", "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))},
		{"http.followRedirects", "false"},
		{"http.sslVerify", "true"},
		{"protocol.allow", "never"},
		{"protocol.https.allow", "always"},
	}
	env := []string{fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(settings))}
	for i, setting := range settings {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, setting[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, setting[1]))
	}
	return env
}
