package mrmo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"compatibility-lab/internal/model"
)

const handlersRelativePath = "internal/handlers"

// handlerFactories maps handler type name -> relative file path under the MRMO repo.
type handlerFactories map[string]string

func scanHandlerFactories(repoPath string) (handlerFactories, error) {
	handlersDir := filepath.Join(repoPath, handlersRelativePath)
	info, err := os.Stat(handlersDir)
	if err != nil {
		return nil, fmt.Errorf("MRMO handlers directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("MRMO handlers path is not a directory: %s", handlersDir)
	}

	factories := handlerFactories{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(handlersDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse handler file %s: %w", path, err)
		}

		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		for _, name := range findRegisteredHandlerNames(file) {
			factories[name] = rel
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return factories, nil
}

func findRegisteredHandlerNames(file *ast.File) []string {
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != "RegisterHandlerFactory" || len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil || name == "" {
			return true
		}
		names = append(names, name)
		return true
	})
	return names
}

func applyHandlerFactories(resources []model.MRMOResource, factories handlerFactories) {
	for i := range resources {
		resource := &resources[i]
		if len(resource.Topics) == 0 {
			resource.HandlerRegistered = false
			resource.HandlerFiles = nil
			continue
		}

		files := make([]string, 0, len(resource.Topics))
		seen := map[string]struct{}{}
		allRegistered := true
		for _, topic := range resource.Topics {
			file, ok := factories[topic.Handler]
			if !ok {
				allRegistered = false
				continue
			}
			if _, exists := seen[file]; exists {
				continue
			}
			seen[file] = struct{}{}
			files = append(files, file)
		}
		sort.Strings(files)
		resource.HandlerRegistered = allRegistered
		resource.HandlerFiles = files
	}
}
