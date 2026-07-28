package mrmo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"

	"compatibility-lab/internal/model"
)

const integrationHandlersTestRelativePath = "internal/integration/tests/handlers_test.go"

const (
	integrationCovered = "covered"
	integrationMissing = "missing"
	integrationUnknown = "unknown"
)

func scanIntegrationCoverage(repoPath string) (map[string]struct{}, error) {
	path := filepath.Join(repoPath, integrationHandlersTestRelativePath)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat MRMO integration handlers test: %w", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse MRMO integration handlers test: %w", err)
	}

	imports := map[string]string{}
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("parse integration test import: %w", err)
		}
		imports[importAlias(imp, importPath)] = importPath
	}

	covered := map[string]struct{}{}
	neededImports := map[string]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != "assertArchetypeFields" || len(call.Args) < 3 {
			return true
		}
		switch arg := call.Args[2].(type) {
		case *ast.BasicLit:
			if arg.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(arg.Value)
			if err != nil || value == "" {
				return true
			}
			covered[value] = struct{}{}
		case *ast.SelectorExpr:
			alias, ok := arg.X.(*ast.Ident)
			if !ok || arg.Sel.Name != "ResourceType" {
				return true
			}
			importPath, ok := imports[alias.Name]
			if !ok {
				return true
			}
			neededImports[importPath] = struct{}{}
		}
		return true
	})

	if len(neededImports) == 0 {
		return covered, nil
	}

	providerRoot := findProviderRepo(repoPath)
	if providerRoot == "" {
		// Selectors could not be resolved offline.
		if len(covered) == 0 {
			return nil, nil
		}
		return covered, nil
	}

	for importPath := range neededImports {
		pkgDir, err := providerPackageDir(providerRoot, importPath)
		if err != nil {
			continue
		}
		resourceType, err := readResourceTypeConstant(pkgDir)
		if err != nil {
			continue
		}
		covered[resourceType] = struct{}{}
	}
	return covered, nil
}

func applyIntegrationCoverage(resources []model.MRMOResource, covered map[string]struct{}, coverageKnown bool) {
	for i := range resources {
		if !coverageKnown {
			resources[i].IntegrationTestStatus = integrationUnknown
			continue
		}
		if _, ok := covered[resources[i].TerraformType]; ok {
			resources[i].IntegrationTestStatus = integrationCovered
			continue
		}
		resources[i].IntegrationTestStatus = integrationMissing
	}
}

func applyReconciliationEligibility(resources []model.MRMOResource) {
	for i := range resources {
		resources[i].ReconciliationEligible = len(resources[i].Topics) > 0 && resources[i].Tier >= 0
	}
}
