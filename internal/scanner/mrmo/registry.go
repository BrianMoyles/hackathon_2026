package mrmo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"compatibility-lab/internal/model"
)

const registryRelativePath = "internal/resourcetypes/registry.go"

func scanRegistry(repoPath string) ([]model.MRMOResource, error) {
	registryPath := filepath.Join(repoPath, registryRelativePath)
	entries, err := parseRegistryFile(registryPath)
	if err != nil {
		return nil, err
	}

	providerRoot := findProviderRepo(repoPath)
	resourceTypes, err := loadProviderResourceTypes(providerRoot, entries)
	if err != nil {
		return nil, err
	}

	resources := make([]model.MRMOResource, 0, len(entries))
	for _, entry := range entries {
		terraformType := entry.terraformTypeLiteral
		if terraformType == "" {
			resolved, ok := resourceTypes[entry.terraformTypeImport]
			if !ok || resolved == "" {
				return nil, fmt.Errorf(
					"unable to resolve Terraform type for %q (%s.ResourceType); set COMPATIBILITY_LAB_PROVIDER_REPO or place terraform-provider-genesyscloud next to the MRMO repo",
					entry.ref,
					entry.terraformTypeAlias,
				)
			}
			terraformType = resolved
		}

		resources = append(resources, model.MRMOResource{
			ResourceTypeRef:       entry.ref,
			TerraformType:         terraformType,
			Domain:                entry.domain,
			IntegrationTestStatus: "unknown",
		})
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].ResourceTypeRef < resources[j].ResourceTypeRef
	})
	return resources, nil
}

type registryEntry struct {
	ref                   string
	domain                string
	terraformTypeLiteral  string
	terraformTypeAlias    string
	terraformTypeImport   string
}

func parseRegistryFile(path string) ([]registryEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse MRMO registry: %w", err)
	}

	imports := map[string]string{}
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("parse MRMO registry import: %w", err)
		}
		alias := importAlias(imp, importPath)
		imports[alias] = importPath
	}

	var registryAssign *ast.CompositeLit
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if name.Name != "registry" || i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("MRMO registry is not a composite literal in %s", path)
				}
				registryAssign = lit
			}
		}
	}
	if registryAssign == nil {
		return nil, fmt.Errorf("MRMO registry map not found in %s", path)
	}

	entries := make([]registryEntry, 0, len(registryAssign.Elts))
	for _, elt := range registryAssign.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ref, err := stringLiteral(kv.Key)
		if err != nil {
			return nil, fmt.Errorf("MRMO registry key: %w", err)
		}
		infoLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("MRMO registry entry %q is not a composite literal", ref)
		}

		entry := registryEntry{ref: ref}
		for _, field := range infoLit.Elts {
			fieldKV, ok := field.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			fieldName, ok := fieldKV.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch fieldName.Name {
			case "Domain":
				domain, err := stringLiteral(fieldKV.Value)
				if err != nil {
					return nil, fmt.Errorf("MRMO registry domain for %q: %w", ref, err)
				}
				entry.domain = domain
			case "TerraformType":
				if lit, err := stringLiteral(fieldKV.Value); err == nil {
					entry.terraformTypeLiteral = lit
					continue
				}
				sel, ok := fieldKV.Value.(*ast.SelectorExpr)
				if !ok {
					return nil, fmt.Errorf("MRMO registry TerraformType for %q has unsupported form", ref)
				}
				alias, ok := sel.X.(*ast.Ident)
				if !ok || sel.Sel.Name != "ResourceType" {
					return nil, fmt.Errorf("MRMO registry TerraformType for %q is not alias.ResourceType", ref)
				}
				importPath, ok := imports[alias.Name]
				if !ok {
					return nil, fmt.Errorf("MRMO registry TerraformType for %q references unknown import alias %q", ref, alias.Name)
				}
				entry.terraformTypeAlias = alias.Name
				entry.terraformTypeImport = importPath
			case "Tier":
				// Tier values in registry.go are filled at runtime from
				// config/resource-hierarchy.yml (MRMO-3). Ignore literals here.
			}
		}
		if entry.terraformTypeLiteral == "" && entry.terraformTypeImport == "" {
			return nil, fmt.Errorf("MRMO registry entry %q is missing TerraformType", ref)
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no MRMO registry entries found in %s", path)
	}
	return entries, nil
}

func importAlias(imp *ast.ImportSpec, importPath string) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	base := filepath.Base(importPath)
	if idx := strings.Index(base, "@"); idx >= 0 {
		base = base[:idx]
	}
	return base
}

func stringLiteral(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("expected string literal")
	}
	return strconv.Unquote(lit.Value)
}
