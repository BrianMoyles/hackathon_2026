package testutil

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGoldens rewrites golden files when tests are run with -update.
var updateGoldens = flag.Bool("update", false, "update golden files")

// AssertJSONGolden marshals got to indented JSON and compares it to the
// golden file at path. Pass -update to refresh the golden.
func AssertJSONGolden(t *testing.T, path string, got any) {
	t.Helper()

	actual, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden value: %v", err)
	}
	actual = append(actual, '\n')

	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (re-run with -update to create)", path, err)
	}
	if !bytes.Equal(want, actual) {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, actual)
	}
}

// AssertTextGolden compares got against a text golden file. Pass -update
// to refresh the golden.
func AssertTextGolden(t *testing.T, path, got string) {
	t.Helper()

	actual := []byte(got)
	if len(actual) == 0 || actual[len(actual)-1] != '\n' {
		actual = append(actual, '\n')
	}

	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (re-run with -update to create)", path, err)
	}
	if !bytes.Equal(want, actual) {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, actual)
	}
}
