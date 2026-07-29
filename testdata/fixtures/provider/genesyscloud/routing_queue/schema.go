package routing_queue

import (
	registrar "example.com/registrar"
	authDivision "example.com/genesyscloud/auth_division"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_routing_queue"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterDataSource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, RoutingQueueExporter())
}

func RoutingQueueExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		RefAttrs: map[string]*resourceExporter.RefAttrSettings{
			"division_id": {RefType: authDivision.ResourceType},
		},
	}
}
