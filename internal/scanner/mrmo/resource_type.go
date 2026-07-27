package mrmo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const providerModulePrefix = "github.com/mypurecloud/terraform-provider-genesyscloud/"

func findProviderRepo(mrmoRepo string) string {
	candidates := []string{
		os.Getenv("COMPATIBILITY_LAB_PROVIDER_REPO"),
		filepath.Join(filepath.Dir(mrmoRepo), "terraform-provider-genesyscloud"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func loadProviderResourceTypes(providerRoot string, entries []registryEntry) (map[string]string, error) {
	needed := map[string]struct{}{}
	for _, entry := range entries {
		if entry.terraformTypeLiteral != "" {
			continue
		}
		if entry.terraformTypeImport == "" {
			continue
		}
		needed[entry.terraformTypeImport] = struct{}{}
	}
	if len(needed) == 0 {
		return map[string]string{}, nil
	}
	if providerRoot == "" {
		return nil, fmt.Errorf("provider repo required to resolve ResourceType constants")
	}

	resolved := make(map[string]string, len(needed))
	for importPath := range needed {
		pkgDir, err := providerPackageDir(providerRoot, importPath)
		if err != nil {
			return nil, err
		}
		resourceType, err := readResourceTypeConstant(pkgDir)
		if err != nil {
			return nil, fmt.Errorf("resolve ResourceType in %s: %w", pkgDir, err)
		}
		resolved[importPath] = resourceType
	}
	return resolved, nil
}

func providerPackageDir(providerRoot, importPath string) (string, error) {
	if !strings.HasPrefix(importPath, providerModulePrefix) {
		return "", fmt.Errorf("unexpected provider import path %q", importPath)
	}
	rel := strings.TrimPrefix(importPath, providerModulePrefix)
	pkgDir := filepath.Join(providerRoot, filepath.FromSlash(rel))
	info, err := os.Stat(pkgDir)
	if err != nil {
		return "", fmt.Errorf("provider package for %s: %w", importPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("provider package path is not a directory: %s", pkgDir)
	}
	return pkgDir, nil
}

func readResourceTypeConstant(pkgDir string) (string, error) {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return "", err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(pkgDir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return "", err
		}
		if value, ok := findResourceTypeInFile(file); ok {
			return value, nil
		}
	}
	return "", fmt.Errorf("ResourceType constant not found")
}

func findResourceTypeInFile(file *ast.File) (string, bool) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if name.Name != "ResourceType" || i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				return value, true
			}
		}
	}
	return "", false
}
