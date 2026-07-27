package mrmo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"compatibility-lab/internal/model"

	"gopkg.in/yaml.v3"
)

const topicsRelativePath = "config/topics.yaml"

type topicsFile struct {
	Topics map[string]topicConfig `yaml:"topics"`
}

type topicConfig struct {
	Handler          string            `yaml:"handler"`
	HandlerMap       []handlerMapEntry `yaml:"handlerMap"`
	Validation       *validationConfig `yaml:"validation"`
	SupportedTypes   []string          `yaml:"supportedTypes"`
	AvroSchemaS3Path string            `yaml:"avroSchemaS3Path"`
	ResourceTypeRef  string            `yaml:"resourceTypeRef"`
}

type handlerMapEntry struct {
	AvroSchema      string            `yaml:"avroSchema"`
	Handler         string            `yaml:"handler"`
	Validation      *validationConfig `yaml:"validation"`
	ResourceTypeRef string            `yaml:"resourceTypeRef"`
}

type validationConfig struct {
	Type string `yaml:"type"`
}

// topicBinding pairs a TopicEntry with the resourceTypeRef it wires.
type topicBinding struct {
	ref   string
	entry model.TopicEntry
}

func parseTopicBindings(repoPath string) ([]topicBinding, int, error) {
	path := filepath.Join(repoPath, topicsRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read MRMO topics config: %w", err)
	}

	var file topicsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, 0, fmt.Errorf("parse MRMO topics config: %w", err)
	}
	if len(file.Topics) == 0 {
		return nil, 0, fmt.Errorf("no topics configured in %s", path)
	}

	bindings := make([]topicBinding, 0)
	for topicName, cfg := range file.Topics {
		bindings = append(bindings, expandTopicBindings(topicName, cfg)...)
	}

	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ref != bindings[j].ref {
			return bindings[i].ref < bindings[j].ref
		}
		if bindings[i].entry.Topic != bindings[j].entry.Topic {
			return bindings[i].entry.Topic < bindings[j].entry.Topic
		}
		if bindings[i].entry.Handler != bindings[j].entry.Handler {
			return bindings[i].entry.Handler < bindings[j].entry.Handler
		}
		return bindings[i].entry.AvroSchema < bindings[j].entry.AvroSchema
	})

	return bindings, len(file.Topics), nil
}

func expandTopicBindings(topicName string, cfg topicConfig) []topicBinding {
	supported := append([]string(nil), cfg.SupportedTypes...)

	if len(cfg.HandlerMap) > 0 {
		bindings := make([]topicBinding, 0, len(cfg.HandlerMap))
		for _, mapped := range cfg.HandlerMap {
			validationType := ""
			if mapped.Validation != nil {
				validationType = mapped.Validation.Type
			}
			bindings = append(bindings, topicBinding{
				ref: mapped.ResourceTypeRef,
				entry: model.TopicEntry{
					Topic:            topicName,
					Handler:          mapped.Handler,
					AvroSchemaS3Path: cfg.AvroSchemaS3Path,
					AvroSchema:       mapped.AvroSchema,
					SupportedTypes:   append([]string(nil), supported...),
					ValidationType:   validationType,
				},
			})
		}
		return bindings
	}

	validationType := ""
	if cfg.Validation != nil {
		validationType = cfg.Validation.Type
	}
	return []topicBinding{{
		ref: cfg.ResourceTypeRef,
		entry: model.TopicEntry{
			Topic:            topicName,
			Handler:          cfg.Handler,
			AvroSchemaS3Path: cfg.AvroSchemaS3Path,
			SupportedTypes:   supported,
			ValidationType:   validationType,
		},
	}}
}

func applyTopicBindings(resources []model.MRMOResource, bindings []topicBinding) {
	byRef := make(map[string][]model.TopicEntry)
	for _, binding := range bindings {
		if binding.ref == "" {
			continue
		}
		byRef[binding.ref] = append(byRef[binding.ref], binding.entry)
	}
	for i := range resources {
		resources[i].Topics = byRef[resources[i].ResourceTypeRef]
	}
}
