package resourcetypes

// Trimmed fixture mirroring MRMO internal/resourcetypes/registry.go.
// TerraformType uses string literals so golden tests stay offline.
var registry = map[string]Info{
	"architect-flow": {
		TerraformType: "genesyscloud_flow",
		Domain:        "architect",
	},
	"routing-queue": {
		TerraformType: "genesyscloud_routing_queue",
		Domain:        "routing",
	},
	"auth-division": {
		TerraformType: "genesyscloud_auth_division",
		Domain:        "authorization",
	},
}

type Info struct {
	TerraformType string
	Domain        string
	Tier          int
}
