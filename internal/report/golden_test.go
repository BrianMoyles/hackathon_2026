package report

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"compatibility-lab/internal/matrix"
	"compatibility-lab/internal/scanner/mrmo"
	"compatibility-lab/internal/scanner/provider"
	"compatibility-lab/internal/testutil"
)

func TestExplainReportGolden(t *testing.T) {
	providerManifest, err := provider.Scan(fixturePath(t, "provider"))
	if err != nil {
		t.Fatalf("provider.Scan() error = %v", err)
	}
	mrmoManifest, err := mrmo.Scan(fixturePath(t, "mrmo"))
	if err != nil {
		t.Fatalf("mrmo.Scan() error = %v", err)
	}

	var buf bytes.Buffer
	for _, query := range []string{"routing-queue", "architect-flow", "genesyscloud_blocked_only"} {
		resource := matrix.Explain(providerManifest, mrmoManifest, query)
		if err := WriteResource(&buf, resource); err != nil {
			t.Fatalf("WriteResource(%q): %v", query, err)
		}
		buf.WriteString("---\n")
	}

	testutil.AssertTextGolden(t, goldenPath(t, "explain-demo.txt"), buf.String())
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
