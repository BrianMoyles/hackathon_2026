package auth_division

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_auth_division"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, AuthDivisionExporter())
}

func AuthDivisionExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{}
}
