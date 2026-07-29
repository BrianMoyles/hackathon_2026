package provider

import (
	"path/filepath"
	"runtime"
	"testing"

	"compatibility-lab/internal/testutil"
)

func TestProviderManifestGolden(t *testing.T) {
	manifest, err := Scan(providerFixturePath(t))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	manifest.RepoPath = "FIXTURE"

	testutil.AssertJSONGolden(t, goldenPath(t, "provider-manifest.json"), manifest)
}

func providerFixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "fixtures", "provider")
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "goldens", name)
}
