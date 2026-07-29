package architect_flow

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_flow"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FlowExporter())
}

func FlowExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		ThirdPartyRefAttrs: []string{"filepath"},
		CustomFileWriter: resourceExporter.CustomFileWriterSettings{
			SubDirectory: "flows",
		},
	}
}
