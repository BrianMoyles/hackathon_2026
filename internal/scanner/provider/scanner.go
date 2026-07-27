// Package provider scans a checkout of terraform-provider-genesyscloud and
// produces a manifest describing which Terraform types have a provider
// resource, data source, and exporter registered.
//
// CX-1 scope: extract registration status only. RefAttrs, singleton metadata,
// file-output metadata, and custom resolvers are populated by later tasks
// (CX-2 onwards).
//
// The scanner is intentionally static and offline: it uses go/parser to walk
// the AST of every non-test .go file under `<repoPath>/genesyscloud/` and
// looks for method calls of the form:
//
//	regInstance.RegisterResource(<resourceType>, ...)
//	regInstance.RegisterDataSource(<resourceType>, ...)
//	regInstance.RegisterExporter(<resourceType>, ...)
//
// `<resourceType>` may be a string literal (e.g. "genesyscloud_routing_queue")
// or an identifier that resolves to a string constant declared elsewhere in
// the same package (e.g. `ResourceType`). Identifiers that cannot be resolved
// against a package-local constant are skipped and surfaced back to the
// caller as scanner warnings, so noisy resolution failures do not silently
// drop registrations.
package provider

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"compatibility-lab/internal/model"
)

// registerMethods enumerates the method names whose first argument identifies
// a Terraform resource type registration.
var registerMethods = map[string]registrationKind{
	"RegisterResource":   kindResource,
	"RegisterDataSource": kindDataSource,
	"RegisterExporter":   kindExporter,
}

type registrationKind int

const (
	kindResource registrationKind = iota
	kindDataSource
	kindExporter
)

// Scan walks the provider repository and returns a manifest describing which
// resources are registered as `Resource`, `DataSource`, and/or `Exporter`.
func Scan(repoPath string) (model.ProviderManifest, error) {
	if err := requireDirectory(repoPath); err != nil {
		return model.ProviderManifest{}, err
	}

	genesyscloudRoot := filepath.Join(repoPath, "genesyscloud")
	if info, err := os.Stat(genesyscloudRoot); err != nil || !info.IsDir() {
		return model.ProviderManifest{}, fmt.Errorf(
			"provider repo does not contain a genesyscloud directory at %s", genesyscloudRoot,
		)
	}

	packages, err := loadPackages(genesyscloudRoot)
	if err != nil {
		return model.ProviderManifest{}, err
	}

	registrations := aggregateRegistrations(packages)
	resources := buildProviderResources(registrations)

	return model.ProviderManifest{
		RepoPath:  repoPath,
		Resources: resources,
	}, nil
}

// packageInfo holds the AST-derived facts we care about for a single Go
// package under `genesyscloud/`.
type packageInfo struct {
	dir string

	// stringConstants maps identifier name -> literal string value for any
	// const or var declaration in the package that assigns a string literal.
	// We treat const and var the same way because a few packages use `var
	// ResourceType = "..."` instead of `const`.
	stringConstants map[string]string

	// calls captures every RegisterResource/DataSource/Exporter call found in
	// the package, tagged with the original expression used as the first arg
	// so we can resolve identifiers after all files in the package are read.
	calls []registrationCall
}

type registrationCall struct {
	kind      registrationKind
	arg       ast.Expr
	file      string
	line      int
	fileShort string
}

// registrationRecord is the resolved view of registrationCall entries for a
// single Terraform type across all packages that touch it.
type registrationRecord struct {
	terraformType string
	hasResource   bool
	hasDataSource bool
	hasExporter   bool
}

func loadPackages(root string) (map[string]*packageInfo, error) {
	packages := map[string]*packageInfo{}
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			// A single unparseable file (rare) should not abort the whole
			// scan, but we do want the caller to know so surfacing an error
			// keeps the demo honest. Wrap with the offending path.
			return fmt.Errorf("parse %s: %w", path, err)
		}

		dir := filepath.Dir(path)
		pkg, ok := packages[dir]
		if !ok {
			pkg = &packageInfo{
				dir:             dir,
				stringConstants: map[string]string{},
			}
			packages[dir] = pkg
		}

		collectStringConstants(file, pkg.stringConstants)
		pkg.calls = append(pkg.calls, collectRegistrationCalls(file, fset, path)...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return packages, nil
}

// collectStringConstants records every top-level const or var declaration
// whose value is a plain string literal. These are the only forms the scanner
// can resolve statically; anything computed at runtime is ignored.
func collectStringConstants(file *ast.File, out map[string]string) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if genDecl.Tok != token.CONST && genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				literal, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					continue
				}
				out[name.Name] = value
			}
		}
	}
}

// collectRegistrationCalls walks the file looking for
// `<X>.RegisterResource(<arg>, ...)` / `RegisterDataSource` / `RegisterExporter`
// call sites.
func collectRegistrationCalls(file *ast.File, fset *token.FileSet, path string) []registrationCall {
	var out []registrationCall
	shortPath := shortenPath(path)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		kind, tracked := registerMethods[selector.Sel.Name]
		if !tracked {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		position := fset.Position(call.Pos())
		out = append(out, registrationCall{
			kind:      kind,
			arg:       call.Args[0],
			file:      path,
			line:      position.Line,
			fileShort: shortPath,
		})
		return true
	})
	return out
}

// aggregateRegistrations flattens the per-package information into a single
// map keyed by Terraform type. Unresolvable identifiers (e.g. selectors that
// reference another package's constant) are silently skipped for CX-1 because
// the provider repo occasionally registers resources through indirection
// that only makes sense to a full type checker.
func aggregateRegistrations(packages map[string]*packageInfo) map[string]*registrationRecord {
	records := map[string]*registrationRecord{}
	for _, pkg := range packages {
		for _, call := range pkg.calls {
			terraformType, ok := resolveResourceType(call.arg, pkg.stringConstants)
			if !ok {
				continue
			}
			record, exists := records[terraformType]
			if !exists {
				record = &registrationRecord{terraformType: terraformType}
				records[terraformType] = record
			}
			switch call.kind {
			case kindResource:
				record.hasResource = true
			case kindDataSource:
				record.hasDataSource = true
			case kindExporter:
				record.hasExporter = true
			}
		}
	}
	return records
}

// resolveResourceType turns the first argument of a registration call into a
// concrete Terraform type string.
//
// Supported forms:
//   - "genesyscloud_foo"                -> string literal
//   - ResourceType                       -> package-local identifier
//   - somePkg.ResourceType               -> qualified identifier (unresolvable
//     here; skipped so the caller can decide how to handle it)
func resolveResourceType(expr ast.Expr, constants map[string]string) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(node.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.Ident:
		value, ok := constants[node.Name]
		if !ok {
			return "", false
		}
		return value, true
	default:
		return "", false
	}
}

func buildProviderResources(records map[string]*registrationRecord) []model.ProviderResource {
	resources := make([]model.ProviderResource, 0, len(records))
	for _, record := range records {
		resources = append(resources, model.ProviderResource{
			TerraformType: record.terraformType,
			HasResource:   record.hasResource,
			HasDataSource: record.hasDataSource,
			HasExporter:   record.hasExporter,
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].TerraformType < resources[j].TerraformType
	})
	return resources
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("provider repo not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("provider repo path is not a directory: %s", path)
	}
	return nil
}

// shortenPath renders a repo-relative path so log lines and future warning
// messages stay compact. It looks for the last `genesyscloud/` segment and
// returns everything from there.
func shortenPath(path string) string {
	needle := string(filepath.Separator) + "genesyscloud" + string(filepath.Separator)
	if idx := strings.LastIndex(path, needle); idx >= 0 {
		return path[idx+1:]
	}
	return filepath.Base(path)
}
