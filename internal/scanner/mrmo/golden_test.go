package mrmo

import (
	"path/filepath"
	"runtime"
	"testing"

	"compatibility-lab/internal/testutil"
)

func TestMRMOManifestGolden(t *testing.T) {
	manifest, err := Scan(mrmoFixturePath(t))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	manifest.RepoPath = "FIXTURE"

	testutil.AssertJSONGolden(t, goldenPath(t, "mrmo-manifest.json"), manifest)
}

func mrmoFixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "fixtures", "mrmo")
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "goldens", name)
}
