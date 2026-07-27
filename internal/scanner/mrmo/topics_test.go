package mrmo

import (
	"path/filepath"
	"testing"
)

func TestParseTopicBindingsFixture(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	bindings, topicCount, err := parseTopicBindings(repoPath)
	if err != nil {
		t.Fatalf("parseTopicBindings() error = %v", err)
	}
	if topicCount != 3 {
		t.Fatalf("topicCount = %d, want 3", topicCount)
	}
	if len(bindings) != 3 {
		t.Fatalf("binding count = %d, want 3", len(bindings))
	}

	byRef := map[string]topicBinding{}
	for _, binding := range bindings {
		byRef[binding.ref] = binding
	}

	queue, ok := byRef["routing-queue"]
	if !ok {
		t.Fatal("missing routing-queue binding")
	}
	if queue.entry.Topic != "AssignmentQueueConfigurationChange" {
		t.Errorf("routing-queue topic = %q", queue.entry.Topic)
	}
	if queue.entry.Handler != "AssignmentQueueConfigurationHandler" {
		t.Errorf("routing-queue handler = %q", queue.entry.Handler)
	}
	if queue.entry.AvroSchemaS3Path != "repository" {
		t.Errorf("routing-queue avroSchemaS3Path = %q", queue.entry.AvroSchemaS3Path)
	}
	if queue.entry.ValidationType != "accept_all" {
		t.Errorf("routing-queue validationType = %q", queue.entry.ValidationType)
	}
	if len(queue.entry.SupportedTypes) != 1 {
		t.Errorf("routing-queue supportedTypes = %#v", queue.entry.SupportedTypes)
	}

	flow, ok := byRef["architect-flow"]
	if !ok {
		t.Fatal("missing architect-flow binding")
	}
	if flow.entry.Topic != "ArchitectObjectNotification" {
		t.Errorf("architect-flow topic = %q", flow.entry.Topic)
	}
	if flow.entry.Handler != "ArchitectFlowHandler" {
		t.Errorf("architect-flow handler = %q", flow.entry.Handler)
	}
	if flow.entry.AvroSchema != "com.inin.avro.notifications.architect.ArchitectFlowNotification" {
		t.Errorf("architect-flow avroSchema = %q", flow.entry.AvroSchema)
	}
	if flow.entry.ValidationType != "field_match" {
		t.Errorf("architect-flow validationType = %q", flow.entry.ValidationType)
	}
}

func TestScanJoinsTopicsOntoRegistryResources(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	manifest, err := Scan(repoPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if manifest.TopicCount != 3 {
		t.Fatalf("TopicCount = %d, want 3", manifest.TopicCount)
	}

	for _, resource := range manifest.Resources {
		if len(resource.Topics) != 1 {
			t.Errorf("%s topic wiring count = %d, want 1", resource.ResourceTypeRef, len(resource.Topics))
		}
	}
}
