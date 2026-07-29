package fake_dep

import registrar "example.com/registrar"

const ResourceType = "genesyscloud_fake_dep"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
}
