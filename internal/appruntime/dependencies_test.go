package appruntime

import (
	"strings"
	"testing"
)

func TestSelectedBinaryRejectsUnlistedPath(t *testing.T) {
	if got := selectedBinary("node", "/tmp/node"); got != "" {
		t.Fatalf("unlisted interpreter accepted: %s", got)
	}
	if got := selectedBinary("unknown", ""); got != "" {
		t.Fatalf("unknown runtime accepted: %s", got)
	}
}

func TestPythonUnitUsesTenantVenv(t *testing.T) {
	s := site{id: 23, domain: "example.com", sk: "c_example_com"}
	c := Config{Runtime: "python", Interpreter: "/usr/bin/python3", Port: 3000}
	body := unit(s, c, c.Interpreter, "/home/c_example_com/public_html/app.py")
	if !strings.Contains(body, "ExecStart="+venvPython(s, c.Interpreter)+" /home/c_example_com/public_html/app.py") {
		t.Fatalf("unit does not use tenant venv: %s", body)
	}
	if venvPython(s, "/usr/local/bin/python3") == venvPython(s, c.Interpreter) {
		t.Fatal("different interpreters share a virtual environment")
	}
}

func TestDependencyOutputIsBounded(t *testing.T) {
	var output cappedOutput
	input := strings.Repeat("x", 32<<10)
	if n, err := output.Write([]byte(input)); err != nil || n != len(input) {
		t.Fatalf("write failed: %d, %v", n, err)
	}
	if len(output.data) != 16<<10 {
		t.Fatalf("output length = %d", len(output.data))
	}
}

func TestReleaseUnitKeepsCodeAndPythonEnvironmentInRelease(t *testing.T) {
	s := site{id: 23, domain: "example.com", sk: "c_example_com"}
	rel := ".sanalcp/releases/23/release-222222222222-0123456789abcdef"
	c := Config{Runtime: "python", Interpreter: "/usr/bin/python3", Entrypoint: "public_html/app.py", ReleaseDir: rel, Port: 3000}
	body := unit(s, c, c.Interpreter, "")
	for _, want := range []string{
		"WorkingDirectory=/home/c_example_com/" + rel,
		"ExecStart=/home/c_example_com/" + rel + "/.venv/bin/python /home/c_example_com/" + rel + "/app.py",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("release unit missing %q", want)
		}
	}
	if _, err := releasePath(s, ".sanalcp/releases/24/release-222222222222-0123456789abcdef"); err == nil {
		t.Fatal("another domain release accepted")
	}
	if _, err := releasePath(s, ".sanalcp/releases/23/../release-222222222222-0123456789abcdef"); err == nil {
		t.Fatal("traversal release accepted")
	}
}

func TestLegacyPythonUnitWithoutPinnedInterpreterKeepsSystemBinary(t *testing.T) {
	s := site{id: 23, domain: "example.com", sk: "c_example_com"}
	c := Config{Runtime: "python", Entrypoint: "public_html/app.py", Port: 3000}
	body := unit(s, c, "/usr/bin/python3", "/home/c_example_com/public_html/app.py")
	if !strings.Contains(body, "ExecStart=/usr/bin/python3 /home/c_example_com/public_html/app.py") {
		t.Fatalf("legacy Python unit changed unexpectedly: %s", body)
	}
}
