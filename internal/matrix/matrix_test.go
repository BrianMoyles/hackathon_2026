package matrix

import (
	"testing"

	"compatibility-lab/internal/model"
)

// TestBuild_DependencyReadinessClassification is the CX-3 anchor test: given
// a synthetic provider manifest with RefAttrs and EncodedRefAttrs pointing at
// a mix of exportable/non-exportable and MRMO-supported/unsupported targets,
// it asserts that each dependency edge is classified into the right
// (ProviderExportable, MRMOSupported, Status) triple.
func TestBuild_DependencyReadinessClassification(t *testing.T) {
	providerManifest := model.ProviderManifest{
		Resources: []model.ProviderResource{
			{
				TerraformType: "genesyscloud_source",
				HasResource:   true,
				HasExporter:   true,
				RefAttrs: []model.RefAttr{
					{Attribute: "queue_id", RefType: "genesyscloud_ready_dep"},
					{Attribute: "flow_id", RefType: "genesyscloud_warning_dep"},
					{Attribute: "orphan_id", RefType: "genesyscloud_unknown_dep"},
					{Attribute: "no_type", RefType: ""},
				},
				EncodedRefAttrs: []model.EncodedRefAttr{
					{
						ContainerAttribute: "config.properties",
						NestedAttribute:    "userIds",
						RefType:            "genesyscloud_warning_dep",
					},
				},
			},
			// A provider-only dep that is exportable and MRMO-supported.
			{
				TerraformType: "genesyscloud_ready_dep",
				HasResource:   true,
				HasExporter:   true,
			},
			// A provider-only dep that is exportable but NOT MRMO-supported.
			{
				TerraformType: "genesyscloud_warning_dep",
				HasResource:   true,
				HasExporter:   true,
			},
			// A dep registered in the provider but WITHOUT an exporter — this
			// is what "not exportable" looks like in practice (the provider
			// has a resource block, but MRMO can't reach its state).
			{
				TerraformType: "genesyscloud_unknown_dep",
				HasResource:   true,
				HasExporter:   false,
			},
		},
	}
	mrmoManifest := model.MRMOManifest{
		Resources: []model.MRMOResource{
			{
				TerraformType:     "genesyscloud_source",
				HandlerRegistered: true,
				Topics:            []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				TerraformType:     "genesyscloud_ready_dep",
				HandlerRegistered: true,
				Topics:            []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
		},
	}

	report := Build(providerManifest, mrmoManifest)

	source := findResource(t, report, "genesyscloud_source")
	deps := indexDeps(source.Dependencies)

	expectDep(t, deps, "RefAttrs.queue_id", depExpectation{
		TerraformType:      "genesyscloud_ready_dep",
		ProviderExportable: true,
		MRMOSupported:      true,
		Status:             "ready",
	})
	expectDep(t, deps, "RefAttrs.flow_id", depExpectation{
		TerraformType:      "genesyscloud_warning_dep",
		ProviderExportable: true,
		MRMOSupported:      false,
		Status:             "warning",
	})
	expectDep(t, deps, "RefAttrs.orphan_id", depExpectation{
		TerraformType:      "genesyscloud_unknown_dep",
		ProviderExportable: false,
		MRMOSupported:      false,
		Status:             "blocked",
	})
	expectDep(t, deps, "RefAttrs.no_type", depExpectation{
		TerraformType:      "",
		ProviderExportable: false,
		MRMOSupported:      false,
		Status:             "unknown",
	})
	expectDep(t, deps, "EncodedRefAttrs.config.properties.userIds", depExpectation{
		TerraformType:      "genesyscloud_warning_dep",
		ProviderExportable: true,
		MRMOSupported:      false,
		Status:             "warning",
	})
}

// TestBuild_DependenciesEmittedForBlockedResources locks in the CX-3 behavior
// fix: even when the source resource is blocked (e.g. MRMO doesn't yet
// support it), its dependency graph is still visible so operators can debug
// why it is stuck.
func TestBuild_DependenciesEmittedForBlockedResources(t *testing.T) {
	providerManifest := model.ProviderManifest{
		Resources: []model.ProviderResource{
			{
				TerraformType: "genesyscloud_blocked_source",
				HasResource:   true,
				HasExporter:   true,
				RefAttrs: []model.RefAttr{
					{Attribute: "queue_id", RefType: "genesyscloud_target"},
				},
			},
			{
				TerraformType: "genesyscloud_target",
				HasResource:   true,
				HasExporter:   true,
			},
		},
	}
	// MRMO manifest is intentionally empty so the source resource is blocked
	// by MRMO_REGISTRY_MISSING.
	report := Build(providerManifest, model.MRMOManifest{})

	source := findResource(t, report, "genesyscloud_blocked_source")
	if source.Status != "blocked" {
		t.Fatalf("expected source to be blocked, got status = %q", source.Status)
	}
	if len(source.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency to still be emitted, got %d", len(source.Dependencies))
	}
	if source.Dependencies[0].TerraformType != "genesyscloud_target" {
		t.Errorf("dep TerraformType = %q, want genesyscloud_target", source.Dependencies[0].TerraformType)
	}
}

// ---- helpers ----

type depExpectation struct {
	TerraformType      string
	ProviderExportable bool
	MRMOSupported      bool
	Status             string
}

func findResource(t *testing.T, report CompatibilityReport, terraformType string) ResourceReadiness {
	t.Helper()
	for _, r := range report.Resources {
		if r.TerraformType == terraformType {
			return r
		}
	}
	t.Fatalf("resource %q missing from report", terraformType)
	return ResourceReadiness{}
}

func indexDeps(deps []DependencyReadiness) map[string]DependencyReadiness {
	byKey := make(map[string]DependencyReadiness, len(deps))
	for _, d := range deps {
		byKey[d.Source] = d
	}
	return byKey
}

func expectDep(t *testing.T, deps map[string]DependencyReadiness, source string, want depExpectation) {
	t.Helper()
	got, ok := deps[source]
	if !ok {
		t.Errorf("dependency %q missing", source)
		return
	}
	if got.TerraformType != want.TerraformType {
		t.Errorf("%s TerraformType = %q, want %q", source, got.TerraformType, want.TerraformType)
	}
	if got.ProviderExportable != want.ProviderExportable {
		t.Errorf("%s ProviderExportable = %v, want %v", source, got.ProviderExportable, want.ProviderExportable)
	}
	if got.MRMOSupported != want.MRMOSupported {
		t.Errorf("%s MRMOSupported = %v, want %v", source, got.MRMOSupported, want.MRMOSupported)
	}
	if got.Status != want.Status {
		t.Errorf("%s Status = %q, want %q", source, got.Status, want.Status)
	}
}
