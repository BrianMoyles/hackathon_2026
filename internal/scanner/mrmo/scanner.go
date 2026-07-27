package mrmo

import (
	"fmt"
	"os"

	"compatibility-lab/internal/model"
)

func Scan(repoPath string) (model.MRMOManifest, error) {
	if err := requireDirectory(repoPath); err != nil {
		return model.MRMOManifest{}, err
	}

	resources, err := scanRegistry(repoPath)
	if err != nil {
		return model.MRMOManifest{}, err
	}

	bindings, topicCount, err := parseTopicBindings(repoPath)
	if err != nil {
		return model.MRMOManifest{}, err
	}
	applyTopicBindings(resources, bindings)

	tiers, err := parseHierarchyTiers(repoPath)
	if err != nil {
		return model.MRMOManifest{}, err
	}
	applyHierarchyTiers(resources, tiers)

	// TODO(MRMO-4+): populate handler factories, integration tests,
	// and reconciliation eligibility.
	return model.MRMOManifest{
		RepoPath:   repoPath,
		Resources:  resources,
		TopicCount: topicCount,
	}, nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("MRMO repo not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("MRMO repo path is not a directory: %s", path)
	}
	return nil
}
