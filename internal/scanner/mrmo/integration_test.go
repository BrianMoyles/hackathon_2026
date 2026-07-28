package mrmo

import (
	"path/filepath"
	"testing"

	"compatibility-lab/internal/model"
)

func TestScanIntegrationCoverageFixture(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	covered, err := scanIntegrationCoverage(repoPath)
	if err != nil {
		t.Fatalf("scanIntegrationCoverage() error = %v", err)
	}
	if covered == nil {
		t.Fatal("expected known coverage map")
	}
	if _, ok := covered["genesyscloud_routing_queue"]; !ok {
		t.Error("expected routing_queue coverage")
	}
	if _, ok := covered["genesyscloud_auth_division"]; !ok {
		t.Error("expected auth_division coverage")
	}
	if _, ok := covered["genesyscloud_flow"]; ok {
		t.Error("architect-flow/genesyscloud_flow should be uncovered in fixture")
	}
}

func TestApplyReconciliationEligibility(t *testing.T) {
	resources := []model.MRMOResource{
		{Topics: []model.TopicEntry{{Topic: "T"}}, Tier: 4},
		{Topics: []model.TopicEntry{{Topic: "T"}}, Tier: -1},
		{Topics: nil, Tier: 0},
	}
	applyReconciliationEligibility(resources)

	if !resources[0].ReconciliationEligible {
		t.Error("topic-wired + tiered resource should be eligible")
	}
	if resources[1].ReconciliationEligible {
		t.Error("missing hierarchy tier should not be eligible")
	}
	if resources[2].ReconciliationEligible {
		t.Error("unwired resource should not be eligible")
	}
}

func TestScanSetsCoverageAndEligibility(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	manifest, err := Scan(repoPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	byRef := map[string]model.MRMOResource{}
	for _, resource := range manifest.Resources {
		byRef[resource.ResourceTypeRef] = resource
	}

	queue := byRef["routing-queue"]
	if queue.IntegrationTestStatus != integrationCovered {
		t.Errorf("routing-queue coverage = %q, want covered", queue.IntegrationTestStatus)
	}
	if !queue.ReconciliationEligible {
		t.Error("routing-queue should be reconciliation eligible")
	}

	flow := byRef["architect-flow"]
	if flow.IntegrationTestStatus != integrationMissing {
		t.Errorf("architect-flow coverage = %q, want missing", flow.IntegrationTestStatus)
	}
	if !flow.ReconciliationEligible {
		t.Error("architect-flow should still be reconciliation eligible")
	}
}
