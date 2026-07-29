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

// TestBuildFlagsMissingHierarchyTier is the MRMO-3 anchor test brought in
// from `mo1`. It confirms that a resource whose tier is the "missing"
// sentinel (Tier < 0) picks up the MRMO_HIERARCHY_TIER_MISSING blocker so
// the matrix reflects hierarchy-file gaps.
func TestBuildFlagsMissingHierarchyTier(t *testing.T) {
	report := Build(
		model.ProviderManifest{
			Resources: []model.ProviderResource{{
				TerraformType: "genesyscloud_routing_queue",
				HasResource:   true,
				HasExporter:   true,
			}},
		},
		model.MRMOManifest{
			Resources: []model.MRMOResource{{
				ResourceTypeRef:       "routing-queue",
				TerraformType:         "genesyscloud_routing_queue",
				Tier:                  -1,
				HandlerRegistered:     true,
				IntegrationTestStatus: "covered",
				Topics: []model.TopicEntry{{
					Topic:   "AssignmentQueueConfigurationChange",
					Handler: "AssignmentQueueConfigurationHandler",
				}},
			}},
		},
	)

	if len(report.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(report.Resources))
	}
	found := false
	for _, issue := range report.Resources[0].Issues {
		if issue.Code == "MRMO_HIERARCHY_TIER_MISSING" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MRMO_HIERARCHY_TIER_MISSING, got %#v", report.Resources[0].Issues)
	}
}

func TestBuildWarnsOnMissingIntegrationTests(t *testing.T) {
	report := Build(
		model.ProviderManifest{
			Resources: []model.ProviderResource{{
				TerraformType: "genesyscloud_routing_queue",
				HasResource:   true,
				HasExporter:   true,
			}},
		},
		model.MRMOManifest{
			Resources: []model.MRMOResource{{
				ResourceTypeRef:       "routing-queue",
				TerraformType:         "genesyscloud_routing_queue",
				Tier:                  4,
				HandlerRegistered:     true,
				IntegrationTestStatus: "missing",
				Topics: []model.TopicEntry{{
					Topic:   "AssignmentQueueConfigurationChange",
					Handler: "AssignmentQueueConfigurationHandler",
				}},
			}},
		},
	)

	resource := report.Resources[0]
	if resource.Status != "warning" {
		t.Fatalf("status = %q, want warning", resource.Status)
	}
	if !containsCode(issueCodes(resource), "MRMO_INTEGRATION_TEST_MISSING") {
		t.Fatalf("expected MRMO_INTEGRATION_TEST_MISSING, got %#v", resource.Issues)
	}
}

func issueCodes(resource ResourceReadiness) []string {
	codes := make([]string, 0, len(resource.Issues))
	for _, issue := range resource.Issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

// TestBuild_SingletonExportIDMissing is the CX-4 anchor test. It walks a
// small mixed manifest through Build and asserts that
// PROVIDER_SINGLETON_EXPORT_ID_MISSING fires exactly for singleton
// resources that lack an ExportID, and stays quiet for the well-formed
// singleton and for non-singleton resources.
func TestBuild_SingletonExportIDMissing(t *testing.T) {
	report := Build(
		model.ProviderManifest{
			Resources: []model.ProviderResource{
				// Broken: singleton with no ExportID -> blocker fires.
				{
					TerraformType: "genesyscloud_broken_singleton",
					HasResource:   true,
					HasExporter:   true,
					IsSingleton:   true,
				},
				// Healthy: singleton with an ExportID -> blocker does NOT fire.
				{
					TerraformType: "genesyscloud_routing_utilization",
					HasResource:   true,
					HasExporter:   true,
					IsSingleton:   true,
					ExportID:      "genesyscloud_routing_utilization",
				},
				// Non-singleton with no ExportID -> irrelevant, blocker does NOT fire.
				{
					TerraformType: "genesyscloud_routing_queue",
					HasResource:   true,
					HasExporter:   true,
				},
			},
		},
		model.MRMOManifest{},
	)

	byType := make(map[string][]string, len(report.Resources))
	for _, res := range report.Resources {
		for _, issue := range res.Issues {
			byType[res.TerraformType] = append(byType[res.TerraformType], issue.Code)
		}
	}

	if !containsCode(byType["genesyscloud_broken_singleton"], "PROVIDER_SINGLETON_EXPORT_ID_MISSING") {
		t.Errorf("broken singleton missing expected blocker: got %#v", byType["genesyscloud_broken_singleton"])
	}
	if containsCode(byType["genesyscloud_routing_utilization"], "PROVIDER_SINGLETON_EXPORT_ID_MISSING") {
		t.Errorf("well-formed singleton fired blocker: got %#v", byType["genesyscloud_routing_utilization"])
	}
	if containsCode(byType["genesyscloud_routing_queue"], "PROVIDER_SINGLETON_EXPORT_ID_MISSING") {
		t.Errorf("non-singleton fired blocker: got %#v", byType["genesyscloud_routing_queue"])
	}
}

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}

// TestBuild_StatusPrecedence is the LAB-2 anchor test for status
// classification. It builds four synthetic resources that each hit
// exactly one severity tier and asserts the resulting Status +
// UnknownCount tally are what LAB-2 promises:
//
//	blocked > warning > unknown > ready
//
// A fifth "stacked" resource picks up an unknown AND a blocker in the
// same run to verify the precedence never demotes: the final Status
// stays blocked even though addUnknown fired first.
func TestBuild_StatusPrecedence(t *testing.T) {
	providerManifest := model.ProviderManifest{
		Resources: []model.ProviderResource{
			// ready: exportable, hash observed, no unresolved refs.
			{
				TerraformType:     "genesyscloud_ready",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
			},
			// warning: exportable + hash observed, but MRMO says the
			// resource is not reconciliation-eligible.
			{
				TerraformType:     "genesyscloud_warning",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
			},
			// unknown: exportable but the scanner could not resolve
			// one of its RefAttrs statically.
			{
				TerraformType:     "genesyscloud_unknown",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				RefAttrs: []model.RefAttr{
					{Attribute: "orphan_id", RefType: ""},
				},
			},
			// blocked: singleton without an ExportID (CX-4 blocker).
			{
				TerraformType:     "genesyscloud_blocked",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				IsSingleton:       true,
			},
			// stacked: hits an unknown signal AND a blocker in the
			// same Build. Final Status must stay blocked.
			{
				TerraformType: "genesyscloud_stacked",
				HasResource:   true,
				HasExporter:   true,
				IsSingleton:   true,
				RefAttrs: []model.RefAttr{
					{Attribute: "orphan_id", RefType: ""},
				},
			},
		},
	}
	readyMRMO := model.MRMOResource{
		TerraformType:          "genesyscloud_ready",
		HandlerRegistered:      true,
		ReconciliationEligible: true,
		IntegrationTestStatus:  "covered",
		Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
	}
	warnMRMO := readyMRMO
	warnMRMO.TerraformType = "genesyscloud_warning"
	warnMRMO.ReconciliationEligible = false
	unknownMRMO := readyMRMO
	unknownMRMO.TerraformType = "genesyscloud_unknown"
	blockedMRMO := readyMRMO
	blockedMRMO.TerraformType = "genesyscloud_blocked"
	stackedMRMO := readyMRMO
	stackedMRMO.TerraformType = "genesyscloud_stacked"

	report := Build(providerManifest, model.MRMOManifest{
		Resources: []model.MRMOResource{readyMRMO, warnMRMO, unknownMRMO, blockedMRMO, stackedMRMO},
	})
	byStatus := map[string]string{}
	for _, r := range report.Resources {
		byStatus[r.TerraformType] = r.Status
	}

	cases := map[string]string{
		"genesyscloud_ready":   "ready",
		"genesyscloud_warning": "warning",
		"genesyscloud_unknown": "unknown",
		"genesyscloud_blocked": "blocked",
		"genesyscloud_stacked": "blocked",
	}
	for terraformType, wantStatus := range cases {
		if got := byStatus[terraformType]; got != wantStatus {
			t.Errorf("%s Status = %q, want %q", terraformType, got, wantStatus)
		}
	}

	// Summary counts must match — the stacked resource is blocked, not
	// unknown, so only one resource should land in the unknown bucket.
	if report.Summary.ReadyCount != 1 {
		t.Errorf("Summary.ReadyCount = %d, want 1", report.Summary.ReadyCount)
	}
	if report.Summary.WarningCount != 1 {
		t.Errorf("Summary.WarningCount = %d, want 1", report.Summary.WarningCount)
	}
	if report.Summary.UnknownCount != 1 {
		t.Errorf("Summary.UnknownCount = %d, want 1", report.Summary.UnknownCount)
	}
	if report.Summary.BlockedCount != 2 {
		t.Errorf("Summary.BlockedCount = %d, want 2", report.Summary.BlockedCount)
	}

	// --strict must trip on any blocked resource.
	if !report.HasStrictFailures() {
		t.Errorf("HasStrictFailures() = false, want true (report has %d blocked)", report.Summary.BlockedCount)
	}
}

// TestBuild_UnknownSignals exercises each CX-side unknown code
// individually so a regression that mis-classifies one of them shows
// up as a specific failure rather than a status-count drift.
func TestBuild_UnknownSignals(t *testing.T) {
	// A clean MRMO record so we can isolate CX-side signals without
	// picking up MRMO warnings or blockers.
	cleanMRMO := func(terraformType string) model.MRMOResource {
		return model.MRMOResource{
			TerraformType:          terraformType,
			HandlerRegistered:      true,
			ReconciliationEligible: true,
			IntegrationTestStatus:  "covered",
			Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
		}
	}

	tests := []struct {
		name         string
		resource     model.ProviderResource
		wantCode     string
		wantStatus   string
		wantWarnCode string // optional, empty means no warning expected
	}{
		{
			name: "unresolved RefAttr fires PROVIDER_REFATTR_UNRESOLVED",
			resource: model.ProviderResource{
				TerraformType:     "genesyscloud_bad_ref",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				RefAttrs: []model.RefAttr{
					{Attribute: "queue_id", RefType: ""},
				},
			},
			wantCode:   "PROVIDER_REFATTR_UNRESOLVED",
			wantStatus: "unknown",
		},
		{
			name: "unresolved EncodedRefAttr fires PROVIDER_ENCODED_REFATTR_UNRESOLVED",
			resource: model.ProviderResource{
				TerraformType:     "genesyscloud_bad_encoded",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				EncodedRefAttrs: []model.EncodedRefAttr{
					{ContainerAttribute: "config.properties", NestedAttribute: "userId", RefType: ""},
				},
			},
			wantCode:   "PROVIDER_ENCODED_REFATTR_UNRESOLVED",
			wantStatus: "unknown",
		},
		{
			name: "no BlockHash on non-singleton fires PROVIDER_BLOCK_HASH_UNKNOWN",
			resource: model.ProviderResource{
				TerraformType: "genesyscloud_no_hash",
				HasResource:   true,
				HasExporter:   true,
			},
			wantCode:   "PROVIDER_BLOCK_HASH_UNKNOWN",
			wantStatus: "unknown",
		},
		{
			name: "no BlockHash on singleton is EXPECTED and does not fire",
			resource: model.ProviderResource{
				TerraformType: "genesyscloud_ok_singleton",
				HasResource:   true,
				HasExporter:   true,
				IsSingleton:   true,
				ExportID:      "genesyscloud_ok_singleton",
			},
			wantCode:   "",
			wantStatus: "ready",
		},
		{
			name: "MRMO reconciliation-ineligible fires MRMO_RECONCILIATION_NOT_ELIGIBLE",
			resource: model.ProviderResource{
				TerraformType:     "genesyscloud_not_reconciled",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
			},
			wantWarnCode: "MRMO_RECONCILIATION_NOT_ELIGIBLE",
			wantStatus:   "warning",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mrmo := cleanMRMO(tc.resource.TerraformType)
			if tc.wantWarnCode == "MRMO_RECONCILIATION_NOT_ELIGIBLE" {
				mrmo.ReconciliationEligible = false
			}
			report := Build(
				model.ProviderManifest{Resources: []model.ProviderResource{tc.resource}},
				model.MRMOManifest{Resources: []model.MRMOResource{mrmo}},
			)
			if len(report.Resources) != 1 {
				t.Fatalf("resource count = %d, want 1", len(report.Resources))
			}
			got := report.Resources[0]
			if got.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q; issues = %#v", got.Status, tc.wantStatus, got.Issues)
			}
			codes := issueCodes(got)
			if tc.wantCode != "" && !containsCode(codes, tc.wantCode) {
				t.Errorf("issue codes = %v, want to contain %q", codes, tc.wantCode)
			}
			if tc.wantWarnCode != "" && !containsCode(codes, tc.wantWarnCode) {
				t.Errorf("issue codes = %v, want to contain %q", codes, tc.wantWarnCode)
			}
		})
	}
}

// TestHasStrictFailures locks in the --strict contract: blockers fail
// strict, warnings and unknowns are report-only.
func TestHasStrictFailures(t *testing.T) {
	tests := []struct {
		name string
		summary Summary
		want bool
	}{
		{"empty report is not a failure", Summary{}, false},
		{"warnings only stay report-only", Summary{WarningCount: 5}, false},
		{"unknowns only stay report-only", Summary{UnknownCount: 5}, false},
		{"a single blocker fails strict", Summary{BlockedCount: 1}, true},
		{"blockers alongside anything else still fail", Summary{ReadyCount: 3, WarningCount: 2, UnknownCount: 1, BlockedCount: 1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := CompatibilityReport{Summary: tc.summary}
			if got := report.HasStrictFailures(); got != tc.want {
				t.Errorf("HasStrictFailures() = %v, want %v", got, tc.want)
			}
		})
	}
}

