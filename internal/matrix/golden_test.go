package matrix

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"compatibility-lab/internal/scanner/mrmo"
	"compatibility-lab/internal/scanner/provider"
	"compatibility-lab/internal/testutil"
)

func TestCompatibilityReportGolden(t *testing.T) {
	providerManifest, err := provider.Scan(fixturePath(t, "provider"))
	if err != nil {
		t.Fatalf("provider.Scan() error = %v", err)
	}
	mrmoManifest, err := mrmo.Scan(fixturePath(t, "mrmo"))
	if err != nil {
		t.Fatalf("mrmo.Scan() error = %v", err)
	}

	providerManifest.RepoPath = "PROVIDER_FIXTURE"
	mrmoManifest.RepoPath = "MRMO_FIXTURE"

	got := Build(providerManifest, mrmoManifest)
	sort.Slice(got.Resources, func(i, j int) bool {
		return got.Resources[i].TerraformType < got.Resources[j].TerraformType
	})

	testutil.AssertJSONGolden(t, goldenPath(t, "compatibility-report.json"), got)
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", name)
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "goldens", name)
}
