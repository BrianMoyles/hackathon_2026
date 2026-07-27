package model

type ProviderManifest struct {
	RepoPath  string             `json:"repoPath"`
	Resources []ProviderResource `json:"resources"`
}

type ProviderResource struct {
	TerraformType       string    `json:"terraformType"`
	HasResource         bool      `json:"hasResource"`
	HasDataSource       bool      `json:"hasDataSource"`
	HasExporter         bool      `json:"hasExporter"`
	IsSingleton         bool      `json:"isSingleton"`
	ExportID            string    `json:"exportId,omitempty"`
	RefAttrs            []RefAttr `json:"refAttrs,omitempty"`
	ExcludedAttributes  []string  `json:"excludedAttributes,omitempty"`
	ThirdPartyRefAttrs  []string  `json:"thirdPartyRefAttrs,omitempty"`
	CustomFileDirectory string    `json:"customFileDirectory,omitempty"`
	HasCustomResolvers  bool      `json:"hasCustomResolvers"`
	BlockHashObserved   bool      `json:"blockHashObserved"`
}

type RefAttr struct {
	Attribute string   `json:"attribute"`
	RefType   string   `json:"refType"`
	AltValues []string `json:"altValues,omitempty"`
}

type MRMOManifest struct {
	RepoPath   string         `json:"repoPath"`
	Resources  []MRMOResource `json:"resources"`
	TopicCount int            `json:"topicCount"`
}

type MRMOResource struct {
	ResourceTypeRef        string       `json:"resourceTypeRef"`
	TerraformType          string       `json:"terraformType"`
	Domain                 string       `json:"domain,omitempty"`
	Tier                   int          `json:"tier"`
	Topics                 []TopicEntry `json:"topics,omitempty"`
	HandlerFiles           []string     `json:"handlerFiles,omitempty"`
	HandlerRegistered      bool         `json:"handlerRegistered"`
	ReconciliationEligible bool         `json:"reconciliationEligible"`
	IntegrationTestStatus  string       `json:"integrationTestStatus"`
}

type TopicEntry struct {
	Topic            string   `json:"topic"`
	Handler          string   `json:"handler"`
	AvroSchemaS3Path string   `json:"avroSchemaS3Path,omitempty"`
	SupportedTypes   []string `json:"supportedTypes,omitempty"`
}

type Issue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
}
