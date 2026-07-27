package matrix

import "compatibility-lab/internal/model"

type CompatibilityReport struct {
	SchemaVersion string              `json:"schemaVersion"`
	Summary       Summary             `json:"summary"`
	Resources     []ResourceReadiness `json:"resources"`
	Issues        []model.Issue       `json:"issues,omitempty"`
	Inputs        CompatibilityInputs `json:"inputs"`
}

type CompatibilityInputs struct {
	ProviderRepo string `json:"providerRepo"`
	MRMORepo     string `json:"mrmoRepo"`
}

type Summary struct {
	ProviderResourceCount int `json:"providerResourceCount"`
	MRMOResourceCount     int `json:"mrmoResourceCount"`
	ReadyCount            int `json:"readyCount"`
	WarningCount          int `json:"warningCount"`
	BlockedCount          int `json:"blockedCount"`
}

type ResourceReadiness struct {
	TerraformType   string                  `json:"terraformType"`
	ResourceTypeRef string                  `json:"resourceTypeRef,omitempty"`
	Status          string                  `json:"status"`
	Score           int                     `json:"score"`
	Provider        *model.ProviderResource `json:"provider,omitempty"`
	MRMO            *model.MRMOResource     `json:"mrmo,omitempty"`
	Dependencies    []DependencyReadiness   `json:"dependencies,omitempty"`
	Issues          []model.Issue           `json:"issues,omitempty"`
}

type DependencyReadiness struct {
	TerraformType      string `json:"terraformType"`
	Source             string `json:"source"`
	ProviderExportable bool   `json:"providerExportable"`
	MRMOSupported      bool   `json:"mrmoSupported"`
	Status             string `json:"status"`
}

func Build(providerManifest model.ProviderManifest, mrmoManifest model.MRMOManifest) CompatibilityReport {
	providerByType := mapProviderResources(providerManifest.Resources)
	mrmoByType := mapMRMOResources(mrmoManifest.Resources)

	var resources []ResourceReadiness
	for terraformType, providerResource := range providerByType {
		var mrmoResource *model.MRMOResource
		if resource, ok := mrmoByType[terraformType]; ok {
			resourceCopy := resource
			mrmoResource = &resourceCopy
		}
		resource := buildResourceReadiness(terraformType, &providerResource, mrmoResource)
		resources = append(resources, resource)
	}
	for terraformType, mrmoResource := range mrmoByType {
		if _, ok := providerByType[terraformType]; ok {
			continue
		}
		resource := buildResourceReadiness(terraformType, nil, &mrmoResource)
		resources = append(resources, resource)
	}

	summary := Summary{
		ProviderResourceCount: len(providerManifest.Resources),
		MRMOResourceCount:     len(mrmoManifest.Resources),
	}
	for _, resource := range resources {
		switch resource.Status {
		case "ready":
			summary.ReadyCount++
		case "warning":
			summary.WarningCount++
		case "blocked":
			summary.BlockedCount++
		}
	}

	return CompatibilityReport{
		SchemaVersion: "compatibility-lab/v1",
		Summary:       summary,
		Resources:     resources,
		Inputs: CompatibilityInputs{
			ProviderRepo: providerManifest.RepoPath,
			MRMORepo:     mrmoManifest.RepoPath,
		},
	}
}

func Explain(providerManifest model.ProviderManifest, mrmoManifest model.MRMOManifest, query string) ResourceReadiness {
	report := Build(providerManifest, mrmoManifest)
	for _, resource := range report.Resources {
		if resource.TerraformType == query || resource.ResourceTypeRef == query {
			return resource
		}
	}
	return ResourceReadiness{
		TerraformType: query,
		Status:        "unknown",
		Score:         0,
		Issues: []model.Issue{
			{Severity: "warning", Code: "RESOURCE_NOT_FOUND", Message: "resource was not found in provider or MRMO manifests"},
		},
	}
}

func DependencyClosure(providerManifest model.ProviderManifest, mrmoManifest model.MRMOManifest, query string) []DependencyReadiness {
	resource := Explain(providerManifest, mrmoManifest, query)
	return resource.Dependencies
}

func buildResourceReadiness(terraformType string, providerResource *model.ProviderResource, mrmoResource *model.MRMOResource) ResourceReadiness {
	readiness := ResourceReadiness{
		TerraformType: terraformType,
		Provider:      providerResource,
		MRMO:          mrmoResource,
		Status:        "ready",
		Score:         100,
	}
	if mrmoResource != nil {
		readiness.ResourceTypeRef = mrmoResource.ResourceTypeRef
	}

	if providerResource == nil || !providerResource.HasResource {
		readiness.addBlocker("PROVIDER_RESOURCE_MISSING", "resource is not registered in the provider")
	}
	if providerResource == nil || !providerResource.HasExporter {
		readiness.addBlocker("PROVIDER_EXPORTER_MISSING", "resource does not have a provider exporter")
	}
	if mrmoResource == nil {
		readiness.addBlocker("MRMO_REGISTRY_MISSING", "resource is not registered as MRMO-supported")
	}
	if mrmoResource != nil && len(mrmoResource.Topics) == 0 {
		readiness.addBlocker("MRMO_TOPIC_MISSING", "resource has no MRMO topic wiring")
	}
	if mrmoResource != nil && !mrmoResource.HandlerRegistered {
		readiness.addBlocker("MRMO_HANDLER_FACTORY_MISSING", "resource handler is not registered")
	}
	if len(readiness.Issues) == 0 && providerResource != nil {
		for _, ref := range providerResource.RefAttrs {
			readiness.Dependencies = append(readiness.Dependencies, DependencyReadiness{
				TerraformType:      ref.RefType,
				Source:             "RefAttrs." + ref.Attribute,
				ProviderExportable: true,
				MRMOSupported:      false,
				Status:             "warning",
			})
		}
	}

	return readiness
}

func (r *ResourceReadiness) addBlocker(code, message string) {
	r.Status = "blocked"
	r.Score = 0
	r.Issues = append(r.Issues, model.Issue{
		Severity: "blocker",
		Code:     code,
		Message:  message,
	})
}

func mapProviderResources(resources []model.ProviderResource) map[string]model.ProviderResource {
	result := make(map[string]model.ProviderResource, len(resources))
	for _, resource := range resources {
		result[resource.TerraformType] = resource
	}
	return result
}

func mapMRMOResources(resources []model.MRMOResource) map[string]model.MRMOResource {
	result := make(map[string]model.MRMOResource, len(resources))
	for _, resource := range resources {
		result[resource.TerraformType] = resource
	}
	return result
}
