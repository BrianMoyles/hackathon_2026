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
		providerResource := providerResource
		var mrmoResource *model.MRMOResource
		if resource, ok := mrmoByType[terraformType]; ok {
			resourceCopy := resource
			mrmoResource = &resourceCopy
		}
		resource := buildResourceReadiness(terraformType, &providerResource, mrmoResource, providerByType, mrmoByType)
		resources = append(resources, resource)
	}
	for terraformType, mrmoResource := range mrmoByType {
		mrmoResource := mrmoResource
		if _, ok := providerByType[terraformType]; ok {
			continue
		}
		resource := buildResourceReadiness(terraformType, nil, &mrmoResource, providerByType, mrmoByType)
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

func buildResourceReadiness(
	terraformType string,
	providerResource *model.ProviderResource,
	mrmoResource *model.MRMOResource,
	providerByType map[string]model.ProviderResource,
	mrmoByType map[string]model.MRMOResource,
) ResourceReadiness {
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
	if mrmoResource != nil && mrmoResource.Tier < 0 {
		readiness.addBlocker("MRMO_HIERARCHY_TIER_MISSING", "resource has no reconciliation hierarchy tier")
	}

	// CX-3: always emit dependency edges, even when the resource itself is
	// blocked. Operators debugging a red resource still want to see which
	// downstream types would need to be resolved before this one can move.
	readiness.Dependencies = buildDependencies(providerResource, providerByType, mrmoByType)

	return readiness
}

// buildDependencies flattens the provider resource's RefAttrs and
// EncodedRefAttrs into a single DependencyReadiness list, tagging each edge
// with whether the target is exportable in the provider and supported by
// MRMO. The Source field carries the origin of the edge (attribute path) so
// downstream reports can point back at the exact source of a broken link.
func buildDependencies(
	providerResource *model.ProviderResource,
	providerByType map[string]model.ProviderResource,
	mrmoByType map[string]model.MRMOResource,
) []DependencyReadiness {
	if providerResource == nil {
		return nil
	}
	deps := make([]DependencyReadiness, 0, len(providerResource.RefAttrs)+len(providerResource.EncodedRefAttrs))
	for _, ref := range providerResource.RefAttrs {
		deps = append(deps, buildDependencyEdge(
			ref.RefType,
			"RefAttrs."+ref.Attribute,
			providerByType,
			mrmoByType,
		))
	}
	for _, ref := range providerResource.EncodedRefAttrs {
		source := "EncodedRefAttrs." + ref.ContainerAttribute
		if ref.NestedAttribute != "" {
			source += "." + ref.NestedAttribute
		}
		deps = append(deps, buildDependencyEdge(
			ref.RefType,
			source,
			providerByType,
			mrmoByType,
		))
	}
	if len(deps) == 0 {
		return nil
	}
	return deps
}

// buildDependencyEdge computes the readiness of a single dependency edge:
//
//	ready    = target is provider-exportable AND MRMO-supported
//	warning  = target is provider-exportable but not MRMO-supported
//	blocked  = target is not even provider-exportable
//	unknown  = we could not statically resolve the target's Terraform type
func buildDependencyEdge(
	refType string,
	source string,
	providerByType map[string]model.ProviderResource,
	mrmoByType map[string]model.MRMOResource,
) DependencyReadiness {
	edge := DependencyReadiness{
		TerraformType: refType,
		Source:        source,
	}
	if refType == "" {
		edge.Status = "unknown"
		return edge
	}
	if target, ok := providerByType[refType]; ok && target.HasExporter {
		edge.ProviderExportable = true
	}
	if _, ok := mrmoByType[refType]; ok {
		edge.MRMOSupported = true
	}
	switch {
	case !edge.ProviderExportable:
		edge.Status = "blocked"
	case !edge.MRMOSupported:
		edge.Status = "warning"
	default:
		edge.Status = "ready"
	}
	return edge
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
