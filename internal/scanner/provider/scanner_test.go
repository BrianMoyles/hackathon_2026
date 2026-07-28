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

	if _, unwanted := byType["genesyscloud_should_be_ignored"]; unwanted {
		t.Errorf("test file registration leaked into manifest")
	}
}

// TestScan_ExtractsExporterMetadata is the CX-2 companion: for every fake
// package it asserts that the fields lifted from the ResourceExporter
// composite literal — RefAttrs, ExcludedAttributes, singleton flags, ExportId,
// ThirdPartyRefAttrs, CustomFileDirectory, HasCustomResolvers — match what
// the source declares. Cross-package `alias.ResourceType` references are
// exercised too so we catch regressions in import-based resolution.
func TestScan_ExtractsExporterMetadata(t *testing.T) {
	repo := t.TempDir()
	writeFakeProvider(t, repo)

	manifest, err := Scan(repo)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	byType := indexByType(manifest.Resources)

	// fake_full is our RefAttrs-heavy resource: it uses both a package-local
	// identifier (ResourceType, self-reference) and a cross-package
	// selector (deps.ResourceType) as RefType values, plus AltValues.
	full, ok := byType["genesyscloud_fake_full"]
	if !ok {
		t.Fatal("genesyscloud_fake_full missing from manifest")
	}
	if full.IsSingleton {
		t.Errorf("fake_full.IsSingleton = true, want false")
	}
	if full.ExportID != "" {
		t.Errorf("fake_full.ExportID = %q, want empty", full.ExportID)
	}
	if !full.HasCustomResolvers {
		t.Errorf("fake_full.HasCustomResolvers = false, want true")
	}
	if len(full.RefAttrs) != 3 {
		t.Fatalf("fake_full.RefAttrs len = %d, want 3", len(full.RefAttrs))
	}
	expectRefAttr(t, full.RefAttrs, "queue_id", "genesyscloud_fake_full", nil)
	expectRefAttr(t, full.RefAttrs, "queue_ids_with_wildcard", "genesyscloud_fake_dep", []string{"*"})
	expectRefAttr(t, full.RefAttrs, "team_id", "genesyscloud_fake_dep", nil)
	expectExcluded(t, full.ExcludedAttributes, []string{"deprecated_field", "legacy_field"})

	// fake_exporter_only is the singleton test: identifier ExportId + no
	// RefAttrs. Also confirms ExportId resolves the same-package ResourceType.
	singleton, ok := byType["genesyscloud_fake_exporter_only"]
	if !ok {
		t.Fatal("genesyscloud_fake_exporter_only missing from manifest")
	}
	if !singleton.IsSingleton {
		t.Errorf("fake_exporter_only.IsSingleton = false, want true")
	}
	if singleton.ExportID != "genesyscloud_fake_exporter_only" {
		t.Errorf("fake_exporter_only.ExportID = %q, want %q",
			singleton.ExportID, "genesyscloud_fake_exporter_only")
	}
	if len(singleton.RefAttrs) != 0 {
		t.Errorf("fake_exporter_only.RefAttrs = %v, want empty", singleton.RefAttrs)
	}

	// fake_file_writer proves ThirdPartyRefAttrs + CustomFileWriter.SubDirectory
	// extraction and confirms the scanner ignores a CustomAttributeResolver
	// map that is present but empty (HasCustomResolvers stays false).
	fileWriter, ok := byType["genesyscloud_fake_file_writer"]
	if !ok {
		t.Fatal("genesyscloud_fake_file_writer missing from manifest")
	}
	if fileWriter.CustomFileDirectory != "audio_prompts" {
		t.Errorf("fake_file_writer.CustomFileDirectory = %q, want %q",
			fileWriter.CustomFileDirectory, "audio_prompts")
	}
	if !fileWriter.WritesFiles {
		t.Errorf("fake_file_writer.WritesFiles = false, want true (CX-5: populated CustomFileWriter{})")
	}
	wantThirdParty := []string{"resources.filename", "resources.file_content_hash"}
	if !stringSlicesEqual(fileWriter.ThirdPartyRefAttrs, wantThirdParty) {
		t.Errorf("fake_file_writer.ThirdPartyRefAttrs = %v, want %v",
			fileWriter.ThirdPartyRefAttrs, wantThirdParty)
	}
	if fileWriter.HasCustomResolvers {
		t.Errorf("fake_file_writer.HasCustomResolvers = true; empty resolver map should not count")
	}
}

// TestScan_FileOutputMetadata is the CX-5 anchor test. It confirms that
// WritesFiles is set whenever the CustomFileWriter literal declares any
// field — not just SubDirectory. This matches the runtime semantics where
// tfexporter checks for a non-nil RetrieveAndWriteFilesFunc and does not
// care whether SubDirectory is populated.
func TestScan_FileOutputMetadata(t *testing.T) {
	repo := t.TempDir()
	writeFakeProvider(t, repo)

	manifest, err := Scan(repo)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	byType := indexByType(manifest.Resources)

	// Writer-func only: no SubDirectory, but WritesFiles is still true
	// because the CustomFileWriter{} literal declares a field.
	funcOnly, ok := byType["genesyscloud_fake_writer_func_only"]
	if !ok {
		t.Fatal("genesyscloud_fake_writer_func_only missing from manifest")
	}
	if !funcOnly.WritesFiles {
		t.Errorf("fake_writer_func_only.WritesFiles = false, want true")
	}
	if funcOnly.CustomFileDirectory != "" {
		t.Errorf("fake_writer_func_only.CustomFileDirectory = %q, want empty",
			funcOnly.CustomFileDirectory)
	}

	// Resources without a CustomFileWriter literal do not write files.
	full, ok := byType["genesyscloud_fake_full"]
	if !ok {
		t.Fatal("genesyscloud_fake_full missing from manifest")
	}
	if full.WritesFiles {
		t.Errorf("fake_full.WritesFiles = true, want false (no CustomFileWriter declared)")
	}
}

// TestScan_BlockHashObserved is the CX-6 anchor test. It confirms three
// things:
//
//   - a GetResourcesFunc that calls util.QuickHashFields(...) is detected;
//   - a GetResourcesFunc that assigns BlockHash on a ResourceMeta literal
//     is also detected;
//   - a GetResourcesFunc whose body does neither leaves BlockHashObserved
//     false so the "unknown" state is loud instead of silent.
func TestScan_BlockHashObserved(t *testing.T) {
	repo := t.TempDir()
	writeFakeProvider(t, repo)

	manifest, err := Scan(repo)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	byType := indexByType(manifest.Resources)

	viaUtil, ok := byType["genesyscloud_fake_hash_via_util"]
	if !ok {
		t.Fatal("genesyscloud_fake_hash_via_util missing from manifest")
	}
	if !viaUtil.BlockHashObserved {
		t.Errorf("fake_hash_via_util.BlockHashObserved = false, want true (util.QuickHashFields present)")
	}

	viaMeta, ok := byType["genesyscloud_fake_hash_via_meta"]
	if !ok {
		t.Fatal("genesyscloud_fake_hash_via_meta missing from manifest")
	}
	if !viaMeta.BlockHashObserved {
		t.Errorf("fake_hash_via_meta.BlockHashObserved = false, want true (ResourceMeta.BlockHash assigned)")
	}

	none, ok := byType["genesyscloud_fake_hash_none"]
	if !ok {
		t.Fatal("genesyscloud_fake_hash_none missing from manifest")
	}
	if none.BlockHashObserved {
		t.Errorf("fake_hash_none.BlockHashObserved = true, want false (unknown must stay explicit)")
	}
}

// TestScan_ExtractsEncodedRefAttrs is the CX-3 companion: EncodedRefAttrs use
// a struct-key map with `{Attr: ..., NestedAttr: ...}` composite literals as
// keys and *RefAttrSettings as values. The scanner has to unwrap both halves
// and preserve the container/nested attribute names so matrix.go can build a
// dependency edge that points at the exact location of the encoded reference.
func TestScan_ExtractsEncodedRefAttrs(t *testing.T) {
	repo := t.TempDir()
	writeFakeProvider(t, repo)

	manifest, err := Scan(repo)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	byType := indexByType(manifest.Resources)

	encoded, ok := byType["genesyscloud_fake_encoded"]
	if !ok {
		t.Fatal("genesyscloud_fake_encoded missing from manifest")
	}
	if len(encoded.EncodedRefAttrs) != 2 {
		t.Fatalf("fake_encoded.EncodedRefAttrs len = %d, want 2", len(encoded.EncodedRefAttrs))
	}
	// Entries are sorted by container then nested attribute, so we know the
	// exact order and can assert each field directly.
	first := encoded.EncodedRefAttrs[0]
	if first.ContainerAttribute != "config.properties" ||
		first.NestedAttribute != "flowId" ||
		first.RefType != "genesyscloud_fake_dep" {
		t.Errorf("EncodedRefAttrs[0] = %+v, want config.properties/flowId -> genesyscloud_fake_dep", first)
	}
	second := encoded.EncodedRefAttrs[1]
	if second.ContainerAttribute != "config.properties" ||
		second.NestedAttribute != "groupId" ||
		second.RefType != "genesyscloud_fake_full" {
		t.Errorf("EncodedRefAttrs[1] = %+v, want config.properties/groupId -> genesyscloud_fake_full", second)
	}
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
		// A dependency package other fakes reference via `deps.ResourceType`.
		// This lets us test cross-package identifier resolution, which is
		// how the real provider expresses most RefTypes.
		"genesyscloud/fake_dep/schema.go": `package fake_dep

import registrar "example.com/registrar"

const ResourceType = "genesyscloud_fake_dep"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
}
`,
		// fake_full covers the RefAttrs + AltValues + self-reference +
		// CustomAttributeResolver code paths in one shot.
		"genesyscloud/fake_full/schema.go": `package fake_full

import (
	registrar "example.com/registrar"
	deps "example.com/genesyscloud/fake_dep"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_fake_full"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterDataSource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeFullExporter())
}

func FakeFullExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		RefAttrs: map[string]*resourceExporter.RefAttrSettings{
			"team_id":                 {RefType: deps.ResourceType},
			"queue_id":                {RefType: ResourceType},
			"queue_ids_with_wildcard": {RefType: deps.ResourceType, AltValues: []string{"*"}},
		},
		ExcludedAttributes: []string{
			"deprecated_field",
			"legacy_field",
		},
		CustomAttributeResolver: map[string]*resourceExporter.RefAttrCustomResolver{
			"team_id": nil,
		},
	}
}
`,
		// Exporter-only + singleton case. Also splits the ResourceType
		// constant into a sibling file to keep the CX-1 cross-file
		// resolution path exercised.
		"genesyscloud/fake_exporter_only/constants.go": `package fake_exporter_only

const ResourceType = "genesyscloud_fake_exporter_only"
`,
		"genesyscloud/fake_exporter_only/schema.go": `package fake_exporter_only

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterExporter(ResourceType, FakeExporter())
}

func FakeExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		IsSingleton: true,
		ExportId:    ResourceType,
	}
}
`,
		// Encoded-ref resource that mirrors genesyscloud_integration: exercises
		// the `map[*JsonEncodeRefAttr]*RefAttrSettings{...}` shape where the
		// key is a struct composite literal that Go auto-addresses.
		"genesyscloud/fake_encoded/schema.go": `package fake_encoded

import (
	registrar "example.com/registrar"
	full "example.com/genesyscloud/fake_full"
	deps "example.com/genesyscloud/fake_dep"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_fake_encoded"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeEncodedExporter())
}

func FakeEncodedExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		EncodedRefAttrs: map[*resourceExporter.JsonEncodeRefAttr]*resourceExporter.RefAttrSettings{
			{Attr: "config.properties", NestedAttr: "groupId"}: {RefType: full.ResourceType},
			{Attr: "config.properties", NestedAttr: "flowId"}:  {RefType: deps.ResourceType},
		},
	}
}
`,
		// File-writer resource that mirrors architect_user_prompt: has
		// ThirdPartyRefAttrs + CustomFileWriter{SubDirectory: "..."} and an
		// EMPTY CustomAttributeResolver map (HasCustomResolvers must stay
		// false — declaring the field but leaving it empty is not the same
		// as actually having a resolver).
		"genesyscloud/fake_file_writer/schema.go": `package fake_file_writer

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_fake_file_writer"

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeFileWriterExporter())
}

func FakeFileWriterExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		CustomFileWriter: resourceExporter.CustomFileWriterSettings{
			SubDirectory: "audio_prompts",
		},
		ThirdPartyRefAttrs: []string{
			"resources.filename",
			"resources.file_content_hash",
		},
		CustomAttributeResolver: map[string]*resourceExporter.RefAttrCustomResolver{},
	}
}
`,
		// File-writer resource that only sets the writer func (no
		// SubDirectory). CX-5 must still report WritesFiles=true because
		// the CustomFileWriter{} literal declares a field. Runtime
		// tfexporter uses the func-nil check, not SubDirectory.
		"genesyscloud/fake_writer_func_only/schema.go": `package fake_writer_func_only

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_fake_writer_func_only"

func writerFunc(a, b, c string, d map[string]interface{}, e interface{}, f resourceExporter.ResourceInfo) error {
	return nil
}

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeWriterFuncOnlyExporter())
}

func FakeWriterFuncOnlyExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		CustomFileWriter: resourceExporter.CustomFileWriterSettings{
			RetrieveAndWriteFilesFunc: writerFunc,
		},
	}
}
`,
		// CX-6: GetResourcesFunc wraps a package-local func that calls
		// util.QuickHashFields, matching the pattern in genesyscloud_user,
		// genesyscloud_integration, etc. Verifies the CX-6 walk follows
		// provider.GetAllWithPooledClient(...) and pattern-matches on the
		// util call's method name.
		"genesyscloud/fake_hash_via_util/schema.go": `package fake_hash_via_util

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
	provider "example.com/provider"
	util "example.com/util"
)

const ResourceType = "genesyscloud_fake_hash_via_util"

func getAllHashed(ctx interface{}) (resourceExporter.ResourceIDMetaMap, error) {
	hash, _ := util.QuickHashFields("a", "b")
	_ = hash
	return nil, nil
}

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeHashViaUtilExporter())
}

func FakeHashViaUtilExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		GetResourcesFunc: provider.GetAllWithPooledClient(getAllHashed),
	}
}
`,
		// CX-6: GetResourcesFunc is a bare identifier and the body
		// assigns BlockHash on a ResourceMeta composite literal. This
		// exercises both the bare-ident branch of extractGetResourcesFuncName
		// and the ResourceMeta compositeLit branch of the walker.
		"genesyscloud/fake_hash_via_meta/schema.go": `package fake_hash_via_meta

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
)

const ResourceType = "genesyscloud_fake_hash_via_meta"

func getAllViaMeta(ctx interface{}) (resourceExporter.ResourceIDMetaMap, error) {
	resources := map[string]*resourceExporter.ResourceMeta{}
	resources["x"] = &resourceExporter.ResourceMeta{
		BlockLabel: "some-label",
		BlockHash:  "abc123",
	}
	return resources, nil
}

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeHashViaMetaExporter())
}

func FakeHashViaMetaExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		GetResourcesFunc: getAllViaMeta,
	}
}
`,
		// CX-6: GetResourcesFunc is present but the body does not call
		// QuickHashFields and does not populate BlockHash. The scanner
		// MUST leave BlockHashObserved false so downstream tooling can
		// flag the resource as "unknown" instead of hiding it.
		"genesyscloud/fake_hash_none/schema.go": `package fake_hash_none

import (
	registrar "example.com/registrar"
	resourceExporter "example.com/resource_exporter"
	provider "example.com/provider"
)

const ResourceType = "genesyscloud_fake_hash_none"

func getAllPlain(ctx interface{}) (resourceExporter.ResourceIDMetaMap, error) {
	resources := map[string]*resourceExporter.ResourceMeta{}
	resources["x"] = &resourceExporter.ResourceMeta{BlockLabel: "just-a-label"}
	return resources, nil
}

func SetRegistrar(regInstance registrar.Registrar) {
	regInstance.RegisterResource(ResourceType, nil)
	regInstance.RegisterExporter(ResourceType, FakeHashNoneExporter())
}

func FakeHashNoneExporter() *resourceExporter.ResourceExporter {
	return &resourceExporter.ResourceExporter{
		GetResourcesFunc: provider.GetAllWithPooledClient(getAllPlain),
	}
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
}

func expectRefAttr(t *testing.T, attrs []model.RefAttr, name, wantRefType string, wantAltValues []string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Attribute != name {
			continue
		}
		if attr.RefType != wantRefType {
			t.Errorf("RefAttr %q RefType = %q, want %q", name, attr.RefType, wantRefType)
		}
		if !stringSlicesEqual(attr.AltValues, wantAltValues) {
			t.Errorf("RefAttr %q AltValues = %v, want %v", name, attr.AltValues, wantAltValues)
		}
		return
	}
	t.Errorf("RefAttr %q missing", name)
}

func expectExcluded(t *testing.T, got, want []string) {
	t.Helper()
	if !stringSlicesEqual(got, want) {
		t.Errorf("ExcludedAttributes = %v, want %v", got, want)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
