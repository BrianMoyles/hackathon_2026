package matrix

import (
	"encoding/json"
	"testing"

	"compatibility-lab/internal/model"
)

// TestBuild_JSONIsDeterministic re-marshals the same manifests several
// times and asserts the bytes are byte-identical. Its sibling
// TestCompatibilityReportGolden proves the JSON *shape* is correct; this
// test proves the JSON is *stable* across runs even when Go map iteration
// order changes underneath us. Keeping the two failure modes separate
// means "schema drift" and "reintroduced map-order dependence" surface
// with very different error messages.
func TestBuild_JSONIsDeterministic(t *testing.T) {
	first, err := json.Marshal(Build(determinismProviderManifest(), determinismMRMOManifest()))
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	// Go's map iteration is only randomised sometimes, so run a handful
	// of iterations to give ordering bugs a chance to surface.
	for i := 0; i < 5; i++ {
		next, err := json.Marshal(Build(determinismProviderManifest(), determinismMRMOManifest()))
		if err != nil {
			t.Fatalf("marshal %d: %v", i, err)
		}
		if string(next) != string(first) {
			t.Fatalf("CompatibilityReport JSON is not deterministic on run %d", i)
		}
	}
}

// determinismProviderManifest deliberately declares resources in a
// non-alphabetical order so that any test asserting sorted output
// actually exercises the sort in Build rather than the input order.
func determinismProviderManifest() model.ProviderManifest {
	return model.ProviderManifest{
		Resources: []model.ProviderResource{
			{
				TerraformType:     "genesyscloud_z_blocked_singleton",
				HasResource:       true,
				HasExporter:       true,
				IsSingleton:       true,
				BlockHashObserved: true,
			},
			{
				TerraformType:     "genesyscloud_a_unknown_ref",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				RefAttrs: []model.RefAttr{
					{Attribute: "orphan_id", RefType: ""},
				},
			},
			{
				TerraformType:     "genesyscloud_m_ready",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				RefAttrs: []model.RefAttr{
					{Attribute: "target_id", RefType: "genesyscloud_target"},
				},
			},
			{
				TerraformType:     "genesyscloud_k_warning",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
				EncodedRefAttrs: []model.EncodedRefAttr{
					{
						ContainerAttribute: "config.properties",
						NestedAttribute:    "userId",
						RefType:            "genesyscloud_target",
					},
				},
			},
			{
				TerraformType:     "genesyscloud_target",
				HasResource:       true,
				HasExporter:       true,
				BlockHashObserved: true,
			},
		},
	}
}

func determinismMRMOManifest() model.MRMOManifest {
	return model.MRMOManifest{
		Resources: []model.MRMOResource{
			{
				ResourceTypeRef:        "z-blocked-singleton",
				TerraformType:          "genesyscloud_z_blocked_singleton",
				Tier:                   3,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				ResourceTypeRef:        "a-unknown-ref",
				TerraformType:          "genesyscloud_a_unknown_ref",
				Tier:                   2,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				ResourceTypeRef:        "m-ready",
				TerraformType:          "genesyscloud_m_ready",
				Tier:                   4,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				ResourceTypeRef:        "k-warning",
				TerraformType:          "genesyscloud_k_warning",
				Tier:                   4,
				HandlerRegistered:      true,
				ReconciliationEligible: false,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				ResourceTypeRef:        "target",
				TerraformType:          "genesyscloud_target",
				Tier:                   1,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
			{
				ResourceTypeRef:        "orphan",
				TerraformType:          "genesyscloud_orphan",
				Tier:                   0,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics:                 []model.TopicEntry{{Topic: "T", Handler: "H"}},
			},
		},
	}
}
