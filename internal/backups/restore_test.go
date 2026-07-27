package backups

import "testing"

func TestSafeRestoreRelativePath(t *testing.T) {
	valid := map[string]string{
		"public_html/index.php":  "public_html/index.php",
		"/mail/user/cur/message": "mail/user/cur/message",
	}
	for input, want := range valid {
		got, err := safeRestoreRelativePath(input)
		if err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "/", ".", "..", "../etc/passwd", "public_html/../../etc/passwd"} {
		if _, err := safeRestoreRelativePath(input); err == nil {
			t.Errorf("güvensiz yol kabul edildi: %q", input)
		}
	}
}
