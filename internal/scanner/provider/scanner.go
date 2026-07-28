// Package provider scans a checkout of terraform-provider-genesyscloud and
// produces a manifest describing each Terraform resource type discovered
// through the provider's SetRegistrar / RegisterExporter conventions.
//
// The scanner is intentionally static and offline: it uses go/parser to walk
// the AST of every non-test .go file under `<repoPath>/genesyscloud/`. Two
// kinds of information are extracted:
//
//	CX-1 (registration status)
//	  Method calls of the form:
//	    regInstance.RegisterResource(<resourceType>, ...)
//	    regInstance.RegisterDataSource(<resourceType>, ...)
//	    regInstance.RegisterExporter(<resourceType>, <exporterFunc>())
//	  produce HasResource / HasDataSource / HasExporter booleans per type.
//
//	CX-2 (exporter metadata snapshot)
//	  For every RegisterExporter call, the scanner locates the exporter
//	  function in the same package and pulls the following fields off its
//	  returned `&<pkg>.ResourceExporter{...}` composite literal:
//	    - RefAttrs (attribute -> RefType, AltValues)
//	    - ExcludedAttributes
//	    - IsSingleton and ExportId
//	    - ThirdPartyRefAttrs
//	    - CustomFileWriter.SubDirectory -> CustomFileDirectory
//	    - CustomAttributeResolver presence -> HasCustomResolvers
//
//	CX-3 (encoded reference graph)
//	  EncodedRefAttrs is walked as a `map[*JsonEncodeRefAttr]*RefAttrSettings`
//	  and flattened into (ContainerAttribute, NestedAttribute, RefType) tuples.
//
//	CX-5 (file-output metadata)
//	  WritesFiles is set whenever the CustomFileWriter{} literal declares any
//	  field (either a writer func or a SubDirectory). This mirrors the
//	  runtime nil-check in tfexporter.customWriteAttributes.
//
//	CX-6 (block-hash hints)
//	  The GetResourcesFunc value is followed to its function body in the same
//	  package. If that body calls `util.QuickHashFields(...)` or assigns a
//	  `BlockHash:` key on a `ResourceMeta{...}` literal, BlockHashObserved is
//	  set to true. Otherwise it stays false so downstream tooling can flag
//	  the resource as "unknown" instead of hiding it.
//
// RefType selectors that reference other provider packages
// (e.g. `authDivision.ResourceType`) are resolved by looking up the referring
// file's imports and consulting that other package's collected string
// constants. Anything that cannot be resolved statically is left as the
// empty string / false so downstream tooling can decide how to handle
// unknowns.
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

// providerPathSegment is the sub-path that every provider package import
// contains. It lets the scanner turn `github.com/.../genesyscloud/foo` into
// `<repo>/genesyscloud/foo` without having to know the full module path.
const providerPathSegment = "/genesyscloud/"

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
// resources are registered as `Resource`, `DataSource`, and/or `Exporter`,
// alongside the exporter metadata pulled from each `ResourceExporter` literal.
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

	records := aggregateRegistrations(packages, genesyscloudRoot)
	resources := buildProviderResources(records)

	return model.ProviderManifest{
		RepoPath:  repoPath,
		Resources: resources,
	}, nil
}

// packageInfo captures every piece of AST-derived data we need from a single
// Go package under `genesyscloud/`.
type packageInfo struct {
	dir string

	// files keeps the parsed AST + per-file imports around so that we can
	// resolve identifiers (e.g. RefType selectors) after all packages have
	// been parsed.
	files []*fileInfo

	// stringConstants maps identifier name -> literal string value for any
	// const or var declaration in the package that assigns a plain string
	// literal. `const` and `var` are both accepted because a handful of
	// provider packages use `var ResourceType = "..."`.
	stringConstants map[string]string

	// calls records every RegisterResource/DataSource/Exporter site seen in
	// the package, along with which file owned it (needed later to resolve
	// cross-package identifiers against that file's imports).
	calls []registrationCall

	// funcs indexes non-method function declarations by name so we can find
	// the exporter function referenced from RegisterExporter(..., FooExporter()).
	funcs map[string]*funcRef
}

type fileInfo struct {
	path    string
	imports map[string]string // alias -> full import path
}

type funcRef struct {
	decl *ast.FuncDecl
	file *fileInfo
}

type registrationCall struct {
	kind         registrationKind
	arg          ast.Expr
	exporterFunc string // populated when kind == kindExporter and the arg is <ident>()
	file         *fileInfo
}

// registrationRecord is the resolved, per-Terraform-type accumulator that
// aggregateRegistrations produces. It carries just enough state to build a
// model.ProviderResource.
type registrationRecord struct {
	terraformType string
	hasResource   bool
	hasDataSource bool
	hasExporter   bool
	exporter      *exporterInfo
}

// exporterInfo mirrors the subset of resourceExporter.ResourceExporter fields
// that CX-2, CX-3, CX-5, and CX-6 promise to expose.
type exporterInfo struct {
	IsSingleton         bool
	ExportID            string
	RefAttrs            []model.RefAttr
	EncodedRefAttrs     []model.EncodedRefAttr
	ExcludedAttributes  []string
	ThirdPartyRefAttrs  []string
	CustomFileDirectory string
	WritesFiles         bool // CX-5: exporter has a populated CustomFileWriter{}
	HasCustomResolvers  bool
	BlockHashObserved   bool // CX-6: GetResourcesFunc computes a BlockHash
}

// ---------------------------------------------------------------------------
// Pass 1: parse every file, collect per-package facts
// ---------------------------------------------------------------------------

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
			return fmt.Errorf("parse %s: %w", path, err)
		}

		dir := filepath.Dir(path)
		pkg, ok := packages[dir]
		if !ok {
			pkg = &packageInfo{
				dir:             dir,
				stringConstants: map[string]string{},
				funcs:           map[string]*funcRef{},
			}
			packages[dir] = pkg
		}

		fi := &fileInfo{
			path:    path,
			imports: collectImports(file),
		}
		pkg.files = append(pkg.files, fi)

		collectStringConstants(file, pkg.stringConstants)
		pkg.calls = append(pkg.calls, collectRegistrationCalls(file, fi)...)
		collectFuncDecls(file, fi, pkg.funcs)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return packages, nil
}

// collectImports resolves both aliased and un-aliased import specs to a map
// from local alias -> full import path.
func collectImports(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		var alias string
		switch {
		case imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != ".":
			alias = imp.Name.Name
		default:
			// Fall back to the last segment of the path. This matches the
			// package's declared name for every provider package we care
			// about (they follow Go's usual dir-name-equals-package-name
			// convention).
			alias = filepath.Base(importPath)
		}
		imports[alias] = importPath
	}
	return imports
}

// collectStringConstants records every top-level const or var declaration
// whose value is a plain string literal. Computed values are ignored.
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
				if value, ok := stringLiteralValue(valueSpec.Values[i]); ok {
					out[name.Name] = value
				}
			}
		}
	}
}

// collectFuncDecls indexes every top-level (non-method) function declaration
// by name so the aggregation phase can locate exporter functions.
func collectFuncDecls(file *ast.File, fi *fileInfo, out map[string]*funcRef) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if funcDecl.Recv != nil {
			continue
		}
		out[funcDecl.Name.Name] = &funcRef{decl: funcDecl, file: fi}
	}
}

// collectRegistrationCalls walks the file looking for
// `<X>.RegisterResource(<arg>, ...)` / `RegisterDataSource` / `RegisterExporter`
// call sites. For RegisterExporter it also records the exporter function
// name so the aggregation phase can find its definition.
func collectRegistrationCalls(file *ast.File, fi *fileInfo) []registrationCall {
	var out []registrationCall

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

		record := registrationCall{
			kind: kind,
			arg:  call.Args[0],
			file: fi,
		}
		if kind == kindExporter && len(call.Args) >= 2 {
			if innerCall, ok := call.Args[1].(*ast.CallExpr); ok {
				if funcIdent, ok := innerCall.Fun.(*ast.Ident); ok {
					record.exporterFunc = funcIdent.Name
				}
			}
		}
		out = append(out, record)
		return true
	})
	return out
}

// ---------------------------------------------------------------------------
// Pass 2: fold per-package facts into per-Terraform-type records
// ---------------------------------------------------------------------------

func aggregateRegistrations(packages map[string]*packageInfo, genesyscloudRoot string) map[string]*registrationRecord {
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
				if info, ok := extractExporterInfo(call, pkg, packages, genesyscloudRoot); ok {
					record.exporter = info
				}
			}
		}
	}
	return records
}

// resolveResourceType turns the first argument of a registration call into a
// concrete Terraform type string.
//
// Supported forms:
//   - "genesyscloud_foo"     -> string literal
//   - ResourceType           -> package-local identifier
//   - somePkg.ResourceType   -> qualified identifier (skipped; not needed by
//     any provider Register* site we've observed)
func resolveResourceType(expr ast.Expr, constants map[string]string) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		return stringLiteralValue(node)
	case *ast.Ident:
		value, ok := constants[node.Name]
		return value, ok
	default:
		return "", false
	}
}

// ---------------------------------------------------------------------------
// Exporter composite-literal extraction (CX-2)
// ---------------------------------------------------------------------------

// extractExporterInfo finds the exporter function referenced from a
// RegisterExporter(...) call and walks its return statement to fill an
// exporterInfo. Best-effort: unknown fields stay zero-valued rather than
// causing the whole call to fail.
func extractExporterInfo(
	call registrationCall,
	pkg *packageInfo,
	packages map[string]*packageInfo,
	genesyscloudRoot string,
) (*exporterInfo, bool) {
	if call.exporterFunc == "" {
		return nil, false
	}
	funcRef, ok := pkg.funcs[call.exporterFunc]
	if !ok {
		return nil, false
	}
	if funcRef.decl.Body == nil {
		return nil, false
	}

	literal, ok := findResourceExporterLiteral(funcRef.decl.Body)
	if !ok {
		return nil, false
	}

	ctx := resolveContext{
		localConstants:   pkg.stringConstants,
		fileImports:      funcRef.file.imports,
		packagesByDir:    packages,
		genesyscloudRoot: genesyscloudRoot,
		currentPackage:   pkg,
	}
	return parseExporterCompositeLit(literal, ctx), true
}

// resolveContext is the bag of data needed by helpers that turn AST
// expressions into concrete strings. Kept as a single struct so the helpers
// stay readable without long argument lists.
type resolveContext struct {
	localConstants   map[string]string
	fileImports      map[string]string // alias -> full import path
	packagesByDir    map[string]*packageInfo
	genesyscloudRoot string
	// currentPackage is the package whose exporter composite literal we are
	// walking. CX-6 uses it to look up the GetResourcesFunc target in the
	// same package's `funcs` map.
	currentPackage *packageInfo
}

// findResourceExporterLiteral scans a function body for the exporter's
// ResourceExporter composite literal. Two return shapes are supported:
//
//	return &<X>.ResourceExporter{...}                              (direct)
//	<name> := &<X>.ResourceExporter{...}; ...; return <name>       (variable-first)
//
// The variable-first form covers architect_flow's dual-exporter pattern,
// where the returned exporter is assigned to a local before being handed
// back after side-effect calls (SetNewFlowResourceExporter, etc.).
func findResourceExporterLiteral(body *ast.BlockStmt) (*ast.CompositeLit, bool) {
	assignments := collectResourceExporterAssignments(body)

	var found *ast.CompositeLit
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		retStmt, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(retStmt.Results) != 1 {
			return true
		}
		expr := retStmt.Results[0]
		if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			expr = unary.X
		}
		if lit, ok := expr.(*ast.CompositeLit); ok && isResourceExporterType(lit.Type) {
			found = lit
			return false
		}
		if ident, ok := expr.(*ast.Ident); ok {
			if lit, ok := assignments[ident.Name]; ok {
				found = lit
				return false
			}
		}
		return true
	})
	return found, found != nil
}

// collectResourceExporterAssignments indexes every local variable in the
// function body that is assigned a `&<X>.ResourceExporter{...}` composite
// literal. Both `:=` (define) and `=` (assign) forms are captured so
// findResourceExporterLiteral can follow `return <ident>` back to the
// literal.
func collectResourceExporterAssignments(body *ast.BlockStmt) map[string]*ast.CompositeLit {
	assignments := map[string]*ast.CompositeLit{}
	ast.Inspect(body, func(n ast.Node) bool {
		assignStmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assignStmt.Lhs) != len(assignStmt.Rhs) {
			return true
		}
		for i, rhs := range assignStmt.Rhs {
			expr := rhs
			if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				expr = unary.X
			}
			lit, ok := expr.(*ast.CompositeLit)
			if !ok || !isResourceExporterType(lit.Type) {
				continue
			}
			lhsIdent, ok := assignStmt.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}
			assignments[lhsIdent.Name] = lit
		}
		return true
	})
	return assignments
}

// isResourceExporterType matches `X.ResourceExporter` type expressions. We
// only care about the Sel name because different provider packages import
// resource_exporter under different aliases (`resource_exporter`,
// `resourceExporter`, `re`, ...).
func isResourceExporterType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return selector.Sel.Name == "ResourceExporter"
}

func parseExporterCompositeLit(lit *ast.CompositeLit, ctx resolveContext) *exporterInfo {
	info := &exporterInfo{}
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch keyIdent.Name {
		case "IsSingleton":
			info.IsSingleton = resolveBool(keyValue.Value)
		case "ExportId":
			info.ExportID = resolveStringExpr(keyValue.Value, ctx)
		case "RefAttrs":
			info.RefAttrs = parseRefAttrsMap(keyValue.Value, ctx)
		case "EncodedRefAttrs":
			info.EncodedRefAttrs = parseEncodedRefAttrsMap(keyValue.Value, ctx)
		case "ExcludedAttributes":
			info.ExcludedAttributes = parseStringSlice(keyValue.Value, ctx)
		case "ThirdPartyRefAttrs":
			info.ThirdPartyRefAttrs = parseStringSlice(keyValue.Value, ctx)
		case "CustomFileWriter":
			// CX-5: capture both the SubDirectory string and a semantic
			// "this exporter writes files" flag. WritesFiles is true whenever
			// the CustomFileWriter literal declares any field (either the
			// writer func or a SubDirectory), which is the same signal that
			// tfexporter uses at runtime to decide whether to invoke the
			// writer func.
			info.CustomFileDirectory, info.WritesFiles = parseCustomFileWriter(keyValue.Value, ctx)
		case "CustomAttributeResolver":
			info.HasCustomResolvers = compositeLitHasEntries(keyValue.Value)
		case "GetResourcesFunc":
			// CX-6: extract the wrapped function name so we can walk its
			// body (in the second pass) for util.QuickHashFields() calls
			// and ResourceMeta{BlockHash: ...} literals.
			if funcName := extractGetResourcesFuncName(keyValue.Value); funcName != "" {
				info.BlockHashObserved = getResourcesFuncObservesBlockHash(funcName, ctx)
			}
		}
	}
	return info
}

// parseRefAttrsMap turns `map[string]*Y.RefAttrSettings{"attr": {RefType: ...}}`
// into a sorted []model.RefAttr. Attributes whose RefType cannot be resolved
// still appear, but with RefType == "" so callers can flag them.
func parseRefAttrsMap(expr ast.Expr, ctx resolveContext) []model.RefAttr {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	if len(lit.Elts) == 0 {
		return nil
	}

	attrs := make([]model.RefAttr, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		attrName, ok := stringLiteralValue(keyValue.Key)
		if !ok {
			continue
		}
		refType, altValues := parseRefAttrSettings(keyValue.Value, ctx)
		attrs = append(attrs, model.RefAttr{
			Attribute: attrName,
			RefType:   refType,
			AltValues: altValues,
		})
	}
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Attribute < attrs[j].Attribute
	})
	return attrs
}

// parseEncodedRefAttrsMap unwraps
//
//	map[*resourceExporter.JsonEncodeRefAttr]*resourceExporter.RefAttrSettings{
//	    {Attr: "config.properties", NestedAttr: "groups"}: {RefType: "genesyscloud_group"},
//	    ...
//	}
//
// into a sorted []model.EncodedRefAttr. Keys can be written either as
// `&Y.JsonEncodeRefAttr{...}` or just `{Attr: ..., NestedAttr: ...}` — Go
// auto-addresses the composite literal when the map's key type is a pointer.
// Both spellings are handled.
func parseEncodedRefAttrsMap(expr ast.Expr, ctx resolveContext) []model.EncodedRefAttr {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	if len(lit.Elts) == 0 {
		return nil
	}

	attrs := make([]model.EncodedRefAttr, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		container, nested, ok := parseJsonEncodeRefAttrKey(keyValue.Key)
		if !ok {
			continue
		}
		refType, altValues := parseRefAttrSettings(keyValue.Value, ctx)
		attrs = append(attrs, model.EncodedRefAttr{
			ContainerAttribute: container,
			NestedAttribute:    nested,
			RefType:            refType,
			AltValues:          altValues,
		})
	}
	sort.Slice(attrs, func(i, j int) bool {
		if attrs[i].ContainerAttribute != attrs[j].ContainerAttribute {
			return attrs[i].ContainerAttribute < attrs[j].ContainerAttribute
		}
		return attrs[i].NestedAttribute < attrs[j].NestedAttribute
	})
	return attrs
}

// parseJsonEncodeRefAttrKey pulls the Attr / NestedAttr string values from a
// JsonEncodeRefAttr composite literal.
func parseJsonEncodeRefAttrKey(expr ast.Expr) (container, nested string, ok bool) {
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
		expr = unary.X
	}
	lit, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return "", "", false
	}
	for _, elt := range lit.Elts {
		keyValue, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		keyIdent, isIdent := keyValue.Key.(*ast.Ident)
		if !isIdent {
			continue
		}
		value, isStr := stringLiteralValue(keyValue.Value)
		if !isStr {
			continue
		}
		switch keyIdent.Name {
		case "Attr":
			container = value
		case "NestedAttr":
			nested = value
		}
	}
	if container == "" && nested == "" {
		return "", "", false
	}
	return container, nested, true
}

// parseRefAttrSettings extracts RefType and AltValues from a `&Y.RefAttrSettings{...}`
// composite literal.
func parseRefAttrSettings(expr ast.Expr, ctx resolveContext) (string, []string) {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return "", nil
	}

	var (
		refType   string
		altValues []string
	)
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "RefType":
			refType = resolveStringExpr(keyValue.Value, ctx)
		case "AltValues":
			altValues = parseStringSlice(keyValue.Value, ctx)
		}
	}
	return refType, altValues
}

// parseStringSlice unwraps `[]string{"a", "b", CONST}` literals, resolving
// identifiers against local constants when possible.
func parseStringSlice(expr ast.Expr, ctx resolveContext) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	if len(lit.Elts) == 0 {
		return nil
	}
	out := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		if value := resolveStringExpr(elt, ctx); value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseCustomFileWriter digs the SubDirectory field out of a
// `CustomFileWriterSettings{...}` composite literal and also reports
// whether the literal declares any field at all. A non-empty literal is
// what tfexporter treats as "this exporter writes files" — that's the
// CX-5 signal we surface as WritesFiles on ProviderResource.
func parseCustomFileWriter(expr ast.Expr, ctx resolveContext) (subDirectory string, writesFiles bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	// If the literal has any populated field (SubDirectory or
	// RetrieveAndWriteFilesFunc), the exporter writes files. An empty
	// CustomFileWriter{} literal is treated as "no file writer configured",
	// which mirrors the runtime nil-check in tfexporter.
	if len(lit.Elts) > 0 {
		writesFiles = true
	}
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if keyIdent.Name == "SubDirectory" {
			subDirectory = resolveStringExpr(keyValue.Value, ctx)
		}
	}
	return subDirectory, writesFiles
}

// extractGetResourcesFuncName pulls the name of the function that supplies
// the exporter's GetResourcesFunc value. The provider consistently spells
// this one of two ways:
//
//	GetResourcesFunc: provider.GetAllWithPooledClient(getAllFoo),
//	GetResourcesFunc: getAllFoo,
//
// The second form is rare but real. Cross-package references
// (`otherpkg.GetAllFoo`) are skipped because CX-6 only walks bodies of
// funcs declared in the same package as the exporter.
func extractGetResourcesFuncName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.CallExpr:
		if len(node.Args) == 0 {
			return ""
		}
		if ident, ok := node.Args[0].(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return node.Name
	}
	return ""
}

// getResourcesFuncObservesBlockHash returns true if the named function in
// the current package computes a per-resource BlockHash via either
// `util.QuickHashFields(...)` or by directly assigning a `BlockHash:` key
// on a `ResourceMeta` composite literal. This is CX-6's "explicit hint"
// signal: when the function is present but no hash logic is seen, the
// caller reports BlockHashObserved=false so downstream tooling can flag
// the resource as "unknown" rather than assuming a stable hash exists.
func getResourcesFuncObservesBlockHash(funcName string, ctx resolveContext) bool {
	if ctx.currentPackage == nil {
		return false
	}
	target, ok := ctx.currentPackage.funcs[funcName]
	if !ok || target.decl.Body == nil {
		return false
	}
	observed := false
	ast.Inspect(target.decl.Body, func(n ast.Node) bool {
		if observed {
			return false
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			if callSelectorMatches(node.Fun, "QuickHashFields") ||
				callSelectorMatches(node.Fun, "QuickHashFieldsWithDefault") {
				observed = true
				return false
			}
		case *ast.CompositeLit:
			if isResourceMetaType(node.Type) && compositeLitHasKey(node, "BlockHash") {
				observed = true
				return false
			}
		}
		return true
	})
	return observed
}

// callSelectorMatches reports whether an expression is a selector call
// `<something>.<methodName>` (e.g. `util.QuickHashFields`). Bare-identifier
// calls to the same name are also matched so in-package aliasing is
// handled.
func callSelectorMatches(fun ast.Expr, methodName string) bool {
	switch node := fun.(type) {
	case *ast.SelectorExpr:
		return node.Sel != nil && node.Sel.Name == methodName
	case *ast.Ident:
		return node.Name == methodName
	}
	return false
}

// isResourceMetaType reports whether a composite literal's type expression
// resolves to `ResourceMeta` (bare or `<alias>.ResourceMeta`). The
// scanner does not attempt to verify the alias's target package — the
// method name is distinctive enough within the provider that false
// positives are effectively impossible.
func isResourceMetaType(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name == "ResourceMeta"
	case *ast.SelectorExpr:
		return node.Sel != nil && node.Sel.Name == "ResourceMeta"
	}
	return false
}

// compositeLitHasKey reports whether a struct-style composite literal
// declares a value for a given field name.
func compositeLitHasKey(lit *ast.CompositeLit, keyName string) bool {
	for _, elt := range lit.Elts {
		keyValue, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if keyIdent.Name == keyName {
			return true
		}
	}
	return false
}

// compositeLitHasEntries reports whether the value is a composite literal
// that actually contains at least one element. Used for HasCustomResolvers:
// declaring `CustomAttributeResolver: map[...]*...{}` with no entries is
// effectively "no custom resolvers" and shouldn't be counted.
func compositeLitHasEntries(expr ast.Expr) bool {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	return len(lit.Elts) > 0
}

// ---------------------------------------------------------------------------
// Value-resolution primitives shared by the composite-literal helpers
// ---------------------------------------------------------------------------

// resolveStringExpr converts an expression that is expected to evaluate to a
// string into the actual string, drawing on local constants and, when the
// expression is `alias.Ident`, the target package's constants.
func resolveStringExpr(expr ast.Expr, ctx resolveContext) string {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if value, ok := stringLiteralValue(node); ok {
			return value
		}
	case *ast.Ident:
		if value, ok := ctx.localConstants[node.Name]; ok {
			return value
		}
	case *ast.SelectorExpr:
		aliasIdent, ok := node.X.(*ast.Ident)
		if !ok {
			return ""
		}
		importPath, ok := ctx.fileImports[aliasIdent.Name]
		if !ok {
			return ""
		}
		targetDir := importPathToProviderDir(importPath, ctx.genesyscloudRoot)
		if targetDir == "" {
			return ""
		}
		targetPkg, ok := ctx.packagesByDir[targetDir]
		if !ok {
			return ""
		}
		if value, ok := targetPkg.stringConstants[node.Sel.Name]; ok {
			return value
		}
	}
	return ""
}

func resolveBool(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "true"
}

// importPathToProviderDir turns something like
// `github.com/mypurecloud/terraform-provider-genesyscloud/genesyscloud/auth_division`
// into `<genesyscloudRoot>/auth_division`. Returns "" for import paths that
// aren't rooted inside `.../genesyscloud/`.
func importPathToProviderDir(importPath, genesyscloudRoot string) string {
	idx := strings.LastIndex(importPath, providerPathSegment)
	if idx < 0 {
		return ""
	}
	relPath := importPath[idx+len(providerPathSegment):]
	if relPath == "" {
		return ""
	}
	return filepath.Join(genesyscloudRoot, filepath.FromSlash(relPath))
}

func stringLiteralValue(expr ast.Expr) (string, bool) {
	basic, ok := expr.(*ast.BasicLit)
	if !ok || basic.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(basic.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// ---------------------------------------------------------------------------
// Final assembly
// ---------------------------------------------------------------------------

func buildProviderResources(records map[string]*registrationRecord) []model.ProviderResource {
	resources := make([]model.ProviderResource, 0, len(records))
	for _, record := range records {
		resource := model.ProviderResource{
			TerraformType: record.terraformType,
			HasResource:   record.hasResource,
			HasDataSource: record.hasDataSource,
			HasExporter:   record.hasExporter,
		}
		if record.exporter != nil {
			resource.IsSingleton = record.exporter.IsSingleton
			resource.ExportID = record.exporter.ExportID
			resource.RefAttrs = record.exporter.RefAttrs
			resource.EncodedRefAttrs = record.exporter.EncodedRefAttrs
			resource.ExcludedAttributes = record.exporter.ExcludedAttributes
			resource.ThirdPartyRefAttrs = record.exporter.ThirdPartyRefAttrs
			resource.CustomFileDirectory = record.exporter.CustomFileDirectory
			resource.WritesFiles = record.exporter.WritesFiles
			resource.HasCustomResolvers = record.exporter.HasCustomResolvers
			resource.BlockHashObserved = record.exporter.BlockHashObserved
		}
		resources = append(resources, resource)
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
