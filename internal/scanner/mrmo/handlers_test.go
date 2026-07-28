package mrmo

import (
	"path/filepath"
	"testing"

	"compatibility-lab/internal/model"
)

func TestScanHandlerFactoriesFixture(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	factories, err := scanHandlerFactories(repoPath)
	if err != nil {
		t.Fatalf("scanHandlerFactories() error = %v", err)
	}

	want := map[string]string{
		"AssignmentQueueConfigurationHandler": "internal/handlers/assignment_queue_configuration_handler.go",
		"AuthorizationDivisionChangeHandler":  "internal/handlers/authorization_division_change_handler.go",
		"ArchitectFlowHandler":                "internal/handlers/architect_flow_handler.go",
	}
	for name, file := range want {
		if got := factories[name]; got != file {
			t.Errorf("factories[%q] = %q, want %q", name, got, file)
		}
	}
}

func TestApplyHandlerFactories(t *testing.T) {
	resources := []model.MRMOResource{
		{
			ResourceTypeRef: "routing-queue",
			Topics: []model.TopicEntry{
				{Handler: "AssignmentQueueConfigurationHandler"},
			},
		},
		{
			ResourceTypeRef: "missing-handler",
			Topics: []model.TopicEntry{
				{Handler: "DoesNotExistHandler"},
			},
		},
		{
			ResourceTypeRef: "no-topics",
		},
	}

	applyHandlerFactories(resources, handlerFactories{
		"AssignmentQueueConfigurationHandler": "internal/handlers/assignment_queue_configuration_handler.go",
	})

	if !resources[0].HandlerRegistered {
		t.Error("routing-queue should be handler-registered")
	}
	if len(resources[0].HandlerFiles) != 1 {
		t.Errorf("routing-queue HandlerFiles = %#v", resources[0].HandlerFiles)
	}
	if resources[1].HandlerRegistered {
		t.Error("missing-handler should not be handler-registered")
	}
	if resources[2].HandlerRegistered {
		t.Error("no-topics resource should not be handler-registered")
	}
}

func TestScanMarksHandlersRegistered(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	manifest, err := Scan(repoPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	for _, resource := range manifest.Resources {
		if !resource.HandlerRegistered {
			t.Errorf("%s HandlerRegistered = false, want true", resource.ResourceTypeRef)
		}
		if len(resource.HandlerFiles) == 0 {
			t.Errorf("%s HandlerFiles empty", resource.ResourceTypeRef)
		}
	}
}
