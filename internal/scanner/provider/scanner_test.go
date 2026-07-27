package provider

import (
	"os"
	"path/filepath"
	"testing"

	"compatibility-lab/internal/model"
)

// TestScan_DetectsResourceDataSourceAndExporter builds a minimal Go source
// tree that mirrors the terraform-provider-genesyscloud layout (a
// `genesyscloud/<package>/*.go` structure) and asserts that the scanner:
//
//   - picks up string-literal Terraform types
//   - resolves package-local `ResourceType` identifiers to their string value
//   - correctly distinguishes hasResource, hasDataSource, and hasExporter
//   - deduplicates when multiple files in the same package register the same
//     Terraform type
func TestScan_DetectsResourceDataSourceAndExporter(t *testing.T) {
	repo := t.TempDir()
	writeFakeProvider(t, repo)

	manifest, err := Scan(repo)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if manifest.RepoPath != repo {
		t.Errorf("RepoPath = %q, want %q", manifest.RepoPath, repo)
	}

	byType := indexByType(manifest.Resources)

	expectRegistration(t, byType, "genesyscloud_fake_full", true, true, true)
	expectRegistration(t, byType, "genesyscloud_fake_exporter_only", false, false, true)
	expectRegistration(t, byType, "genesyscloud_fake_data_only", false, true, false)
}

// TestScan_MissingGenesyscloudDir keeps the error path honest so mis-pointed
// --provider-repo flags fail loudly instead of returning an empty manifest.
func TestScan_MissingGenesyscloudDir(t *testing.T) {
	repo := t.TempDir()
	if _, err := Scan(repo); err == nil {
		t.Fatal("Scan should fail when the repo has no genesyscloud/ subdirectory")
	}
}

func writeFakeProvider(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		// Full trio: identifier-resolved Terraform type + all three kinds of
		// registration in the same SetRegistrar call.
		"genesyscloud/fake_full/schema.go": `package fake_full

import registrar "example.com/registrar"

const ResourceType = "genesyscloud_fake_full"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterDataSource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, nil)
}
`,
		// Exporter-only: exercises the case where the Terraform type is only
		// visible in a sibling file (mirrors data_source_genesyscloud_*.go
		// declaring the constant and the schema file consuming it).
		"genesyscloud/fake_exporter_only/constants.go": `package fake_exporter_only

const ResourceType = "genesyscloud_fake_exporter_only"
`,
		"genesyscloud/fake_exporter_only/schema.go": `package fake_exporter_only

import registrar "example.com/registrar"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterExporter(ResourceType, nil)
}
`,
		// Data-source-only via a plain string literal, plus a duplicate
		// registration in another file to make sure booleans stay latched.
		"genesyscloud/fake_data_only/init.go": `package fake_data_only

import registrar "example.com/registrar"

func SetRegistrar(l registrar.Registrar) {
	l.RegisterDataSource("genesyscloud_fake_data_only", nil)
}
`,
		"genesyscloud/fake_data_only/redundant.go": `package fake_data_only

import registrar "example.com/registrar"

func registerAgain(l registrar.Registrar) {
	l.RegisterDataSource("genesyscloud_fake_data_only", nil)
}
`,
		// Should be ignored: tests never contribute registrations to the
		// production manifest, so putting one here is a regression trap.
		"genesyscloud/fake_data_only/schema_test.go": `package fake_data_only

import registrar "example.com/registrar"

func testHelper(l registrar.Registrar) {
	l.RegisterResource("genesyscloud_should_be_ignored", nil)
}
`,
	}

	for relPath, contents := range files {
		fullPath := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}
}

func indexByType(resources []model.ProviderResource) map[string]model.ProviderResource {
	byType := make(map[string]model.ProviderResource, len(resources))
	for _, r := range resources {
		byType[r.TerraformType] = r
	}
	return byType
}

func expectRegistration(
	t *testing.T,
	byType map[string]model.ProviderResource,
	terraformType string,
	wantResource, wantDataSource, wantExporter bool,
) {
	t.Helper()
	resource, ok := byType[terraformType]
	if !ok {
		t.Errorf("terraform type %q missing from manifest", terraformType)
		return
	}
	if resource.HasResource != wantResource {
		t.Errorf("%s HasResource = %v, want %v", terraformType, resource.HasResource, wantResource)
	}
	if resource.HasDataSource != wantDataSource {
		t.Errorf("%s HasDataSource = %v, want %v", terraformType, resource.HasDataSource, wantDataSource)
	}
	if resource.HasExporter != wantExporter {
		t.Errorf("%s HasExporter = %v, want %v", terraformType, resource.HasExporter, wantExporter)
	}

	if _, unwanted := byType["genesyscloud_should_be_ignored"]; unwanted {
		t.Errorf("test file registration leaked into manifest")
	}
}
