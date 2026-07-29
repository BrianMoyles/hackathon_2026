package roundtrip

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareFilesFixture(t *testing.T) {
	source := fixturePath(t, "source.json")
	target := fixturePath(t, "target.json")

	report, err := CompareFiles(source, target, "genesyscloud_routing_queue")
	if err != nil {
		t.Fatalf("CompareFiles() error = %v", err)
	}

	if report.Summary.MissingInTarget != 1 {
		t.Errorf("MissingInTarget = %d, want 1", report.Summary.MissingInTarget)
	}
	if report.Summary.ExtraInTarget != 1 {
		t.Errorf("ExtraInTarget = %d, want 1", report.Summary.ExtraInTarget)
	}
	if report.Summary.AttributeChanges < 1 {
		t.Errorf("AttributeChanges = %d, want >= 1", report.Summary.AttributeChanges)
	}

	kinds := map[string]int{}
	for _, finding := range report.Findings {
		kinds[string(finding.Kind)+":"+finding.BlockLabel+":"+finding.Attribute]++
	}
	if kinds["missing_in_target:sales:"] != 1 {
		t.Errorf("expected sales missing_in_target, got %#v", kinds)
	}
	if kinds["extra_in_target:billing:"] != 1 {
		t.Errorf("expected billing extra_in_target, got %#v", kinds)
	}
	if kinds["attribute_change:support:acw_wrapup_prompt"] != 1 {
		t.Errorf("expected support acw_wrapup_prompt change, got %#v", kinds)
	}
}

func TestNormalizeStripsVolatileIDs(t *testing.T) {
	doc := ExportDocument{
		Resource: map[string]map[string]map[string]any{
			"genesyscloud_routing_queue": {
				"support": {
					"name":     "Support",
					"id":       "volatile",
					"self_uri": "/api/v2/x",
				},
			},
		},
	}
	normalized := Normalize(doc)
	attrs := normalized.Resource["genesyscloud_routing_queue"]["support"]
	if _, ok := attrs["id"]; ok {
		t.Error("id should be stripped")
	}
	if _, ok := attrs["self_uri"]; ok {
		t.Error("self_uri should be stripped")
	}
	if attrs["name"] != "Support" {
		t.Errorf("name = %v, want Support", attrs["name"])
	}
}

func TestCompareIdenticalAfterNormalization(t *testing.T) {
	source := ExportDocument{
		Resource: map[string]map[string]map[string]any{
			"genesyscloud_routing_queue": {
				"support": {
					"name": "Support",
					"id":   "source-id",
				},
			},
		},
	}
	target := ExportDocument{
		Resource: map[string]map[string]map[string]any{
			"genesyscloud_routing_queue": {
				"support": {
					"name": "Support",
					"id":   "target-id",
				},
			},
		},
	}
	report := Compare(source, target, "")
	if report.Summary.Total != 0 {
		t.Fatalf("expected no drift after normalization, got %#v", report.Findings)
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", "roundtrip", name)
}
