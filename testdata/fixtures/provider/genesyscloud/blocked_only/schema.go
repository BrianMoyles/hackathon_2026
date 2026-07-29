package blocked_only

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_blocked_only"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, BlockedOnlyExporter())
}

func BlockedOnlyExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{}
}
