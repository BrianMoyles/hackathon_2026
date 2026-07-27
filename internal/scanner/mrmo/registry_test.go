package mrmo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRegistryFixture(t *testing.T) {
	repoPath := filepath.Join("..", "..", "..", "testdata", "fixtures", "mrmo")

	manifest, err := Scan(repoPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if got := len(manifest.Resources); got != 3 {
		t.Fatalf("resource count = %d, want 3", got)
	}

	want := []struct {
		ref string
		tf  string
		dom string
	}{
		{"architect-flow", "genesyscloud_flow", "architect"},
		{"auth-division", "genesyscloud_auth_division", "authorization"},
		{"routing-queue", "genesyscloud_routing_queue", "routing"},
	}

	for i, entry := range want {
		got := manifest.Resources[i]
		if got.ResourceTypeRef != entry.ref {
			t.Errorf("resources[%d].ResourceTypeRef = %q, want %q", i, got.ResourceTypeRef, entry.ref)
		}
		if got.TerraformType != entry.tf {
			t.Errorf("resources[%d].TerraformType = %q, want %q", i, got.TerraformType, entry.tf)
		}
		if got.Domain != entry.dom {
			t.Errorf("resources[%d].Domain = %q, want %q", i, got.Domain, entry.dom)
		}
	}
}

func TestParseRegistryFileRejectsMissingMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.go")
	content := "package resourcetypes\n\nvar other = map[string]int{}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := parseRegistryFile(path); err == nil {
		t.Fatal("expected error for missing registry map")
	}
}
