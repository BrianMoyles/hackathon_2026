package report

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"compatibility-lab/internal/matrix"
	"compatibility-lab/internal/model"
	"compatibility-lab/internal/providerdiff"
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

// TestMarkdownReportGolden is the LAB-4 anchor test. It scans the shared
// fixture provider + MRMO trees, feeds the resulting CompatibilityReport
// through WriteMarkdown, and compares the output byte-for-byte to
// testdata/goldens/compatibility-report.md. Repo paths are scrubbed to
// static labels so the golden stays portable across machines. Regenerate
// with `go test ./internal/report/... -update` after intentional wording
// or layout changes and diff the golden file in the same PR.
func TestMarkdownReportGolden(t *testing.T) {
	providerManifest, err := provider.Scan(fixturePath(t, "provider"))
	if err != nil {
		t.Fatalf("provider.Scan() error = %v", err)
	}
	mrmoManifest, err := mrmo.Scan(fixturePath(t, "mrmo"))
	if err != nil {
		t.Fatalf("mrmo.Scan() error = %v", err)
	}
	// Freeze the header line — otherwise it leaks the developer's
	// absolute path into the golden.
	providerManifest.RepoPath = "PROVIDER_FIXTURE"
	mrmoManifest.RepoPath = "MRMO_FIXTURE"

	report := matrix.Build(providerManifest, mrmoManifest)

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, report); err != nil {
		t.Fatalf("WriteMarkdown() error = %v", err)
	}

	testutil.AssertTextGolden(t, goldenPath(t, "compatibility-report.md"), buf.String())
}

// TestDiffReportMarkdownGolden is the LAB-7 anchor test. It scans the
// fixture provider to get a realistic base manifest, hand-mutates a
// copy to construct a head manifest that exercises every Kind
// (RESOURCE_ADDED/REMOVED, EXPORTER_REMOVED, REFATTR_*, ENCODED_REFATTR_*,
// SINGLETON_FLIPPED, EXPORT_ID_CHANGED), and asserts the markdown output
// is byte-identical to testdata/goldens/provider-diff-report.md.
//
// Using the fixture provider (as opposed to synthetic in-file manifests)
// keeps the golden legible: reviewers can cross-reference terraform
// types against the fixtures shipped for LAB-5.
func TestDiffReportMarkdownGolden(t *testing.T) {
	baseManifest, err := provider.Scan(fixturePath(t, "provider"))
	if err != nil {
		t.Fatalf("provider.Scan(base) error = %v", err)
	}
	mrmoManifest, err := mrmo.Scan(fixturePath(t, "mrmo"))
	if err != nil {
		t.Fatalf("mrmo.Scan() error = %v", err)
	}

	headManifest := mutateForDiffGolden(baseManifest)

	diffReport := providerdiff.Diff(baseManifest, headManifest, mrmoManifest)
	// Freeze inputs so the golden is portable across machines and CI.
	diffReport.Inputs = providerdiff.DiffInputs{
		ProviderRepo: "PROVIDER_FIXTURE",
		MRMORepo:     "MRMO_FIXTURE",
		BaseRef:      "main",
		HeadRef:      "pr/example",
	}

	var buf bytes.Buffer
	if err := WriteDiffMarkdown(&buf, diffReport); err != nil {
		t.Fatalf("WriteDiffMarkdown() error = %v", err)
	}

	testutil.AssertTextGolden(t, goldenPath(t, "provider-diff-report.md"), buf.String())
}

// mutateForDiffGolden derives a "head" manifest from a base by making
// six deliberate changes, one per Kind we want to see in the golden:
//
//   - EXPORTER_REMOVED (high, MRMO-supported)  on routing_queue
//   - REFATTR_CHANGED  (high, MRMO-supported)  on routing_queue.division_id
//   - REFATTR_ADDED    (low)                   on routing_queue.new_field_id
//   - EXPORT_ID_CHANGED (medium)               on architect_flow
//   - ENCODED_REFATTR_CHANGED (high on MRMO)   on architect_flow (if present)
//   - RESOURCE_ADDED   (low)                   genesyscloud_new_thing
//   - RESOURCE_REMOVED (high, MRMO-supported)  removes auth_division
//
// The mutations are intentionally hand-picked to hit every risk tier and
// every Kind so the golden document doubles as a visual regression test
// for the WriteDiffMarkdown layout.
func mutateForDiffGolden(base model.ProviderManifest) model.ProviderManifest {
	head := model.ProviderManifest{
		RepoPath:  base.RepoPath,
		Resources: make([]model.ProviderResource, 0, len(base.Resources)+1),
	}
	for _, resource := range base.Resources {
		mutated := resource
		switch resource.TerraformType {
		case "genesyscloud_auth_division":
			// Skip this one entirely to produce a RESOURCE_REMOVED
			// finding. MRMO knows about auth-division so this should
			// classify as high risk.
			continue
		case "genesyscloud_routing_queue":
			mutated.HasExporter = false
			if len(mutated.RefAttrs) > 0 {
				mutated.RefAttrs = append([]model.RefAttr(nil), mutated.RefAttrs...)
				mutated.RefAttrs[0] = model.RefAttr{
					Attribute: mutated.RefAttrs[0].Attribute,
					RefType:   "genesyscloud_renamed_target",
				}
				mutated.RefAttrs = append(mutated.RefAttrs, model.RefAttr{
					Attribute: "new_field_id",
					RefType:   "genesyscloud_new_ref",
				})
			}
		case "genesyscloud_flow":
			mutated.ExportID = "renamed_export_id"
			if len(mutated.EncodedRefAttrs) > 0 {
				mutated.EncodedRefAttrs = append([]model.EncodedRefAttr(nil), mutated.EncodedRefAttrs...)
				mutated.EncodedRefAttrs[0].RefType = "genesyscloud_renamed_target"
			}
		}
		head.Resources = append(head.Resources, mutated)
	}
	head.Resources = append(head.Resources, model.ProviderResource{
		TerraformType: "genesyscloud_new_thing",
		HasResource:   true,
		HasExporter:   true,
	})
	return head
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
