package mrmo

import (
	"fmt"
	"os"
	"path/filepath"

	"compatibility-lab/internal/model"

	"gopkg.in/yaml.v3"
)

const hierarchyRelativePath = "config/resource-hierarchy.yml"

// missingHierarchyTier matches MRMO hierarchy.GetResourceTier when a type is absent.
const missingHierarchyTier = -1

type hierarchyFile struct {
	Tiers []hierarchyTier `yaml:"tiers"`
}

type hierarchyTier struct {
	Tier      int      `yaml:"tier"`
	Resources []string `yaml:"resources"`
}

func parseHierarchyTiers(repoPath string) (map[string]int, error) {
	path := filepath.Join(repoPath, hierarchyRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read MRMO resource hierarchy: %w", err)
	}

	var file hierarchyFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse MRMO resource hierarchy: %w", err)
	}
	if len(file.Tiers) == 0 {
		return nil, fmt.Errorf("no tiers configured in %s", path)
	}

	tiers := make(map[string]int)
	for _, tierData := range file.Tiers {
		for _, resourceType := range tierData.Resources {
			tiers[resourceType] = tierData.Tier
		}
	}
	if len(tiers) == 0 {
		return nil, fmt.Errorf("no resources found in %s", path)
	}
	return tiers, nil
}

func applyHierarchyTiers(resources []model.MRMOResource, tiers map[string]int) {
	for i := range resources {
		tier, ok := tiers[resources[i].TerraformType]
		if !ok {
			resources[i].Tier = missingHierarchyTier
			continue
		}
		resources[i].Tier = tier
	}
}
