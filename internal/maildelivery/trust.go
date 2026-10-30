package maildelivery

import (
	"os/user"
	"path"
	"strings"
	"sync"
)

// SYSLOG_IDENTIFIER is an ordinary journal field that any local process can
// write. Only journald-attached _UID/_EXE prove which program produced an entry,
// so records are authorized by both before they can attribute mail to a tenant.
var postfixDaemonDirs = []string{"/usr/lib/postfix/sbin", "/usr/libexec/postfix", "/usr/lib/postfix"}

var (
	postfixUIDOnce sync.Once
	postfixUID     string
)

func trustedPostfixUID() string {
	postfixUIDOnce.Do(func() {
		if u, err := user.Lookup("postfix"); err == nil {
			postfixUID = u.Uid
		}
	})
	return postfixUID
}

func expectedExecutable(identifier string) (name string, admin bool) {
	switch identifier {
	case "postfix/smtpd", "postfix/submission/smtpd", "postfix/submissions/smtpd":
		return "smtpd", false
	case "postfix/postsuper":
		return "postsuper", true
	}
	if rest, ok := strings.CutPrefix(identifier, "postfix/"); ok && !strings.Contains(rest, "/") {
		return rest, false
	}
	return "", false
}

// trustedRecord accepts only root, or the dedicated postfix user, running the
// exact Postfix executable that owns the log tag.
func trustedRecord(r Record, postfixUID string) bool {
	name, admin := expectedExecutable(r.Identifier)
	if name == "" || r.UID == "" || r.Exe == "" || path.Clean(r.Exe) != r.Exe {
		return false
	}
	if r.UID != "0" && (admin || postfixUID == "" || r.UID != postfixUID) {
		return false
	}
	if admin {
		return r.Exe == "/usr/sbin/postsuper" || r.Exe == "/usr/bin/postsuper"
	}
	for _, dir := range postfixDaemonDirs {
		if r.Exe == dir+"/"+name {
			return true
		}
	}
	return false
}

func (c *Collector) trusted(r Record) bool { return trustedRecord(r, trustedPostfixUID()) }
