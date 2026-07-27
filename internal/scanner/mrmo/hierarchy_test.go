package mrmo

import (
	"path/filepath"
	"testing"

	"compatibility-lab/internal/model"
)

func TestParseHierarchyTiersFixture(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	tiers, err := parseHierarchyTiers(repoPath)
	if err != nil {
		t.Fatalf("parseHierarchyTiers() error = %v", err)
	}

	want := map[string]int{
		"genesyscloud_auth_division": 0,
		"genesyscloud_flow":          4,
		"genesyscloud_routing_queue": 4,
	}
	for resourceType, tier := range want {
		if got := tiers[resourceType]; got != tier {
			t.Errorf("tier[%q] = %d, want %d", resourceType, got, tier)
		}
	}
}

func TestApplyHierarchyTiersMissingIsNegativeOne(t *testing.T) {
	resources := []model.MRMOResource{
		{TerraformType: "genesyscloud_routing_queue"},
		{TerraformType: "genesyscloud_not_in_hierarchy"},
	}
	applyHierarchyTiers(resources, map[string]int{
		"genesyscloud_routing_queue": 4,
	})

	if resources[0].Tier != 4 {
		t.Errorf("known resource tier = %d, want 4", resources[0].Tier)
	}
	if resources[1].Tier != missingHierarchyTier {
		t.Errorf("missing resource tier = %d, want %d", resources[1].Tier, missingHierarchyTier)
	}
}

func TestScanAppliesHierarchyTiers(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	manifest, err := Scan(repoPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	want := map[string]int{
		"architect-flow": 4,
		"auth-division":  0,
		"routing-queue":  4,
	}
	for _, resource := range manifest.Resources {
		tier, ok := want[resource.ResourceTypeRef]
		if !ok {
			t.Errorf("unexpected resource %q", resource.ResourceTypeRef)
			continue
		}
		if resource.Tier != tier {
			t.Errorf("%s tier = %d, want %d", resource.ResourceTypeRef, resource.Tier, tier)
		}
	}
}
