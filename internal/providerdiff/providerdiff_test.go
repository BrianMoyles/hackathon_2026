package providerdiff

import (
	"testing"

	"compatibility-lab/internal/model"
)

// findingKey collapses a Finding down to the tuple the tests actually
// care about (TerraformType, Kind, Attribute, Risk). Two findings with
// the same key are considered equivalent; the exact wording of Message
// is not asserted so we can tweak copy without breaking tests.
type findingKey struct {
	terraformType string
	kind          Kind
	attribute     string
	risk          Risk
}

func keyOf(f Finding) findingKey {
	return findingKey{f.TerraformType, f.Kind, f.Attribute, f.Risk}
}

func TestDiff_EmptyReturnsEmptyFindingsNotNil(t *testing.T) {
	report := Diff(model.ProviderManifest{}, model.ProviderManifest{}, model.MRMOManifest{})

	if report.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", report.SchemaVersion, SchemaVersion)
	}
	if report.Findings == nil {
		t.Fatal("Findings should be a non-nil empty slice, got nil (JSON contract)")
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected zero findings, got %d", len(report.Findings))
	}
	if report.Summary.TotalFindings != 0 {
		t.Errorf("TotalFindings = %d, want 0", report.Summary.TotalFindings)
	}
}

// TestDiff_ExporterRemovedOnMRMOSupportedIsHighRisk is the direct LAB-7
// acceptance test: a resource that MRMO cares about loses its exporter
// between base and head, and must land as high risk.
func TestDiff_ExporterRemovedOnMRMOSupportedIsHighRisk(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_routing_queue", HasResource: true, HasExporter: true},
	}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_routing_queue", HasResource: true, HasExporter: false},
	}}
	mrmo := model.MRMOManifest{Resources: []model.MRMOResource{
		{TerraformType: "genesyscloud_routing_queue", Tier: 4, ReconciliationEligible: true},
	}}

	report := Diff(base, head, mrmo)

	found := findByKind(report.Findings, KindExporterRemoved)
	if found == nil {
		t.Fatal("expected EXPORTER_REMOVED finding, got none")
	}
	if found.Risk != RiskHigh {
		t.Errorf("Risk = %q, want %q (MRMO-supported exporter removal)", found.Risk, RiskHigh)
	}
	if !found.MRMOSupported {
		t.Error("MRMOSupported = false, want true")
	}
	if report.Summary.HighRiskCount != 1 {
		t.Errorf("HighRiskCount = %d, want 1", report.Summary.HighRiskCount)
	}
}

// The same removal on a resource MRMO does not track should stay low
// risk — otherwise every dead-code cleanup in the provider would fail
// the check.
func TestDiff_ExporterRemovedOnUnmanagedIsLowRisk(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_experimental", HasResource: true, HasExporter: true},
	}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_experimental", HasResource: true, HasExporter: false},
	}}

	report := Diff(base, head, model.MRMOManifest{})

	found := findByKind(report.Findings, KindExporterRemoved)
	if found == nil {
		t.Fatal("expected EXPORTER_REMOVED finding, got none")
	}
	if found.Risk != RiskLow {
		t.Errorf("Risk = %q, want %q (not MRMO-supported)", found.Risk, RiskLow)
	}
	if found.MRMOSupported {
		t.Error("MRMOSupported = true, want false")
	}
}

// TestDiff_RefAttrAddedAndChanged covers the change + add axis of
// LAB-7's RefAttr acceptance criterion on an MRMO-supported resource.
// The isolated removal path lives in TestDiff_RefAttrRemovedRiskGrading
// so the two failure modes surface with different test names.
func TestDiff_RefAttrAddedAndChanged(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_routing_queue",
		HasResource:   true,
		HasExporter:   true,
		RefAttrs: []model.RefAttr{
			{Attribute: "division_id", RefType: "genesyscloud_auth_division"},
			{Attribute: "queue_flow_id", RefType: "genesyscloud_flow"},
		},
	}}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_routing_queue",
		HasResource:   true,
		HasExporter:   true,
		RefAttrs: []model.RefAttr{
			{Attribute: "division_id", RefType: "genesyscloud_auth_division"},
			{Attribute: "queue_flow_id", RefType: "genesyscloud_new_flow_type"},
			{Attribute: "callback_flow_id", RefType: "genesyscloud_flow"},
		},
	}}}
	mrmo := model.MRMOManifest{Resources: []model.MRMOResource{
		{TerraformType: "genesyscloud_routing_queue"},
	}}

	report := Diff(base, head, mrmo)
	got := make(map[findingKey]Finding, len(report.Findings))
	for _, f := range report.Findings {
		got[keyOf(f)] = f
	}

	// Added: low risk regardless of MRMO status (new capability, not a
	// breaking change).
	if _, ok := got[findingKey{"genesyscloud_routing_queue", KindRefAttrAdded, "callback_flow_id", RiskLow}]; !ok {
		t.Errorf("missing REFATTR_ADDED / callback_flow_id / low, got %+v", findingsKeys(report.Findings))
	}
	// Changed: high risk because the resource is MRMO-supported.
	change, ok := got[findingKey{"genesyscloud_routing_queue", KindRefAttrChanged, "queue_flow_id", RiskHigh}]
	if !ok {
		t.Fatalf("missing REFATTR_CHANGED / queue_flow_id / high, got %+v", findingsKeys(report.Findings))
	}
	if change.BeforeValue != "genesyscloud_flow" || change.AfterValue != "genesyscloud_new_flow_type" {
		t.Errorf("RefAttrChanged before/after = %q/%q, want genesyscloud_flow / genesyscloud_new_flow_type",
			change.BeforeValue, change.AfterValue)
	}
}

// TestDiff_RefAttrRemovedRiskGrading isolates the removal case with a
// clean setup (the previous test intentionally does NOT remove any
// RefAttr) and checks both MRMO-supported and unsupported paths.
func TestDiff_RefAttrRemovedRiskGrading(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_flow",
		HasResource:   true,
		HasExporter:   true,
		RefAttrs: []model.RefAttr{
			{Attribute: "division_id", RefType: "genesyscloud_auth_division"},
		},
	}}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_flow",
		HasResource:   true,
		HasExporter:   true,
	}}}

	// Supported case → high.
	reportSupported := Diff(base, head, model.MRMOManifest{Resources: []model.MRMOResource{
		{TerraformType: "genesyscloud_flow"},
	}})
	removed := findByKind(reportSupported.Findings, KindRefAttrRemoved)
	if removed == nil {
		t.Fatal("expected REFATTR_REMOVED finding")
	}
	if removed.Risk != RiskHigh {
		t.Errorf("Risk = %q, want %q (MRMO-supported)", removed.Risk, RiskHigh)
	}
	if removed.Attribute != "division_id" || removed.BeforeValue != "genesyscloud_auth_division" {
		t.Errorf("REFATTR_REMOVED attribute/before = %q/%q, want division_id / genesyscloud_auth_division",
			removed.Attribute, removed.BeforeValue)
	}

	// Unsupported case → low.
	reportUnsupported := Diff(base, head, model.MRMOManifest{})
	removedLow := findByKind(reportUnsupported.Findings, KindRefAttrRemoved)
	if removedLow == nil {
		t.Fatal("expected REFATTR_REMOVED finding on unmanaged resource")
	}
	if removedLow.Risk != RiskLow {
		t.Errorf("Risk = %q, want %q (not MRMO-supported)", removedLow.Risk, RiskLow)
	}
}

func TestDiff_EncodedRefAttrsAndSingletonAndExportID(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_config",
		HasResource:   true,
		HasExporter:   true,
		IsSingleton:   true,
		ExportID:      "singleton",
		EncodedRefAttrs: []model.EncodedRefAttr{
			{ContainerAttribute: "config.properties", NestedAttribute: "userId", RefType: "genesyscloud_user"},
		},
	}}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{{
		TerraformType: "genesyscloud_config",
		HasResource:   true,
		HasExporter:   true,
		IsSingleton:   false,
		ExportID:      "renamed",
		EncodedRefAttrs: []model.EncodedRefAttr{
			{ContainerAttribute: "config.properties", NestedAttribute: "userId", RefType: "genesyscloud_person"},
		},
	}}}
	mrmo := model.MRMOManifest{Resources: []model.MRMOResource{{TerraformType: "genesyscloud_config"}}}

	report := Diff(base, head, mrmo)
	keys := make(map[Kind]Finding, len(report.Findings))
	for _, f := range report.Findings {
		keys[f.Kind] = f
	}

	if _, ok := keys[KindSingletonFlipped]; !ok {
		t.Error("expected SINGLETON_FLIPPED finding")
	} else if keys[KindSingletonFlipped].Risk != RiskMedium {
		t.Errorf("SingletonFlipped risk = %q, want %q", keys[KindSingletonFlipped].Risk, RiskMedium)
	}
	if _, ok := keys[KindExportIDChanged]; !ok {
		t.Error("expected EXPORT_ID_CHANGED finding")
	} else if keys[KindExportIDChanged].Risk != RiskMedium {
		t.Errorf("ExportIDChanged risk = %q, want %q", keys[KindExportIDChanged].Risk, RiskMedium)
	}
	changed, ok := keys[KindEncodedRefAttrChanged]
	if !ok {
		t.Fatal("expected ENCODED_REFATTR_CHANGED finding")
	}
	if changed.Risk != RiskHigh {
		t.Errorf("EncodedRefAttrChanged risk = %q, want %q (MRMO-supported)", changed.Risk, RiskHigh)
	}
	if changed.Attribute != "config.properties.userId" {
		t.Errorf("EncodedRefAttrChanged attribute = %q, want %q", changed.Attribute, "config.properties.userId")
	}
	if changed.BeforeValue != "genesyscloud_user" || changed.AfterValue != "genesyscloud_person" {
		t.Errorf("EncodedRefAttrChanged before/after = %q/%q, want genesyscloud_user / genesyscloud_person",
			changed.BeforeValue, changed.AfterValue)
	}
}

func TestDiff_ResourceAddedAndRemoved(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_deprecated", HasResource: true, HasExporter: true},
	}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_new_resource", HasResource: true, HasExporter: true},
	}}
	mrmo := model.MRMOManifest{Resources: []model.MRMOResource{
		{TerraformType: "genesyscloud_deprecated"},
	}}

	report := Diff(base, head, mrmo)

	added := findByKind(report.Findings, KindResourceAdded)
	if added == nil {
		t.Fatal("expected RESOURCE_ADDED finding")
	}
	if added.Risk != RiskLow {
		t.Errorf("RESOURCE_ADDED risk = %q, want %q (additions are always low)", added.Risk, RiskLow)
	}
	if added.TerraformType != "genesyscloud_new_resource" {
		t.Errorf("RESOURCE_ADDED terraformType = %q, want %q", added.TerraformType, "genesyscloud_new_resource")
	}

	removed := findByKind(report.Findings, KindResourceRemoved)
	if removed == nil {
		t.Fatal("expected RESOURCE_REMOVED finding")
	}
	if removed.Risk != RiskHigh {
		t.Errorf("RESOURCE_REMOVED risk = %q, want %q (MRMO-supported)", removed.Risk, RiskHigh)
	}
	if !removed.MRMOSupported {
		t.Error("RESOURCE_REMOVED MRMOSupported = false, want true")
	}

	if report.Summary.TotalFindings != 2 {
		t.Errorf("TotalFindings = %d, want 2", report.Summary.TotalFindings)
	}
	if report.Summary.HighRiskCount != 1 || report.Summary.LowRiskCount != 1 {
		t.Errorf("summary tally wrong: %+v", report.Summary)
	}
}

// TestDiff_IsDeterministic re-runs Diff several times on the same input
// and asserts the findings slice comes back in identical order every
// time. Without the sort in Diff, map iteration would cause flapping.
func TestDiff_IsDeterministic(t *testing.T) {
	base := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_z", HasResource: true, HasExporter: true},
		{TerraformType: "genesyscloud_a", HasResource: true, HasExporter: true, RefAttrs: []model.RefAttr{
			{Attribute: "x", RefType: "T1"},
			{Attribute: "y", RefType: "T2"},
		}},
		{TerraformType: "genesyscloud_m", HasResource: true, HasExporter: true},
	}}
	head := model.ProviderManifest{Resources: []model.ProviderResource{
		{TerraformType: "genesyscloud_z", HasResource: true, HasExporter: false},
		{TerraformType: "genesyscloud_a", HasResource: true, HasExporter: true, RefAttrs: []model.RefAttr{
			{Attribute: "x", RefType: "T1"},
			{Attribute: "y", RefType: "T3"},
		}},
		{TerraformType: "genesyscloud_m", HasResource: true, HasExporter: true},
		{TerraformType: "genesyscloud_new", HasResource: true, HasExporter: true},
	}}
	mrmo := model.MRMOManifest{}

	first := Diff(base, head, mrmo).Findings
	for i := 0; i < 5; i++ {
		next := Diff(base, head, mrmo).Findings
		if len(next) != len(first) {
			t.Fatalf("run %d: len=%d, want %d", i, len(next), len(first))
		}
		for j := range next {
			if next[j] != first[j] {
				t.Fatalf("run %d: findings[%d] differs\nfirst=%+v\n next=%+v", i, j, first[j], next[j])
			}
		}
	}
}

func findByKind(findings []Finding, kind Kind) *Finding {
	for i, f := range findings {
		if f.Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func findingsKeys(findings []Finding) []findingKey {
	out := make([]findingKey, len(findings))
	for i, f := range findings {
		out[i] = keyOf(f)
	}
	return out
}
