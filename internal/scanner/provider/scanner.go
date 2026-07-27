package provider

import (
	"fmt"
	"os"

	"compatibility-lab/internal/model"
)

func Scan(repoPath string) (model.ProviderManifest, error) {
	if err := requireDirectory(repoPath); err != nil {
		return model.ProviderManifest{}, err
	}

	// TODO(CX): populate from provider_registrar and ResourceExporter metadata.
	return model.ProviderManifest{
		RepoPath: repoPath,
		Resources: []model.ProviderResource{
			{
				TerraformType: "genesyscloud_routing_queue",
				HasResource:   true,
				HasDataSource: true,
				HasExporter:   true,
				RefAttrs: []model.RefAttr{
					{Attribute: "division_id", RefType: "genesyscloud_auth_division"},
				},
			},
		},
	}, nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("provider repo not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("provider repo path is not a directory: %s", path)
	}
	return nil
}
