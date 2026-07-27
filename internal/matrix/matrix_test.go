package matrix

import (
	"testing"

	"compatibility-lab/internal/model"
)

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
				ResourceTypeRef:   "routing-queue",
				TerraformType:     "genesyscloud_routing_queue",
				Tier:              -1,
				HandlerRegistered: true,
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
