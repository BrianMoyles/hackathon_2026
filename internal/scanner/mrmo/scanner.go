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

	// TODO(MRMO): populate from registry.go, topics.yaml, resource-hierarchy.yml,
	// handler factory registrations, and integration tests.
	return model.MRMOManifest{
		RepoPath:   repoPath,
		TopicCount: 1,
		Resources: []model.MRMOResource{
			{
				ResourceTypeRef:        "routing-queue",
				TerraformType:          "genesyscloud_routing_queue",
				Domain:                 "routing",
				Tier:                   4,
				HandlerRegistered:      true,
				ReconciliationEligible: true,
				IntegrationTestStatus:  "covered",
				Topics: []model.TopicEntry{
					{
						Topic:            "AssignmentQueueConfigurationChange",
						Handler:          "AssignmentQueueConfigurationHandler",
						AvroSchemaS3Path: "repository",
					},
				},
			},
		},
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
