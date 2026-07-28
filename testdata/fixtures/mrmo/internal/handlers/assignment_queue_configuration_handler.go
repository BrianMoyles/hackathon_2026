package handlers

func init() {
	RegisterHandlerFactory("AssignmentQueueConfigurationHandler", nil)
}

func RegisterHandlerFactory(handlerType string, factory any) {}
