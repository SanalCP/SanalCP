package backups

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEntegrasyonVerificationRejectsClientShell(t *testing.T) {
	if os.Getenv("SANALCP_SQLIMPORT_IT") != "1" || os.Geteuid() != 0 {
		t.Skip("SANALCP_SQLIMPORT_IT=1 and root required")
	}
	dir := t.TempDir()
	marker, dump := filepath.Join(dir, "executed"), filepath.Join(dir, "dump.sql")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := os.WriteFile(dump, []byte("CREATE TABLE safe_table(id INT);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreDrillSQL(ctx, dump); err != nil {
		t.Fatal("normal verification failed:", err)
	}
	if err := os.WriteFile(dump, []byte("CREATE TABLE safe_table(id INT);\n\\! touch "+marker+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreDrillSQL(ctx, dump); err == nil || !strings.Contains(err.Error(), "SQL") {
		t.Fatalf("unsafe dump not rejected: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("verification executed local shell: %v", err)
	}
}
