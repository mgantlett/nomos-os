package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"gopkg.in/yaml.v3"
)

// MethodContract holds the expected signature of an interface method.
type MethodContract struct {
	Name    string   `yaml:"name" validate:"required"`
	Params  []string `yaml:"params"`
	Returns []string `yaml:"returns"`
}

// SymbolContract specifies the contract constraints for a Go or Polyglot symbol.
type SymbolContract struct {
	Name     string           `yaml:"name" validate:"required"`
	Kind     string           `yaml:"kind" validate:"omitempty,oneof=interface struct function"` // interface, struct, function
	Methods  []MethodContract `yaml:"methods" validate:"dive"`
	Params   []string         `yaml:"params"`
	Returns  []string         `yaml:"returns"`
	Patterns []string         `yaml:"patterns"` // Regex patterns to verify polyglot code
}

// FileContract binds a source file to its expected symbol contracts.
type FileContract struct {
	File     string           `yaml:"file" validate:"required"`
	Language string           `yaml:"language" validate:"required"`
	Symbols  []SymbolContract `yaml:"symbols" validate:"required,dive"`
}

// ContractsSpec represents the root structure of the contracts.yaml configuration.
type ContractsSpec struct {
	Contracts []FileContract `yaml:"contracts" validate:"required,dive"`
}

// loadContractsSpec handles file reading, unmarshaling, and struct validation
// of the contracts.yaml specification file.
func loadContractsSpec(specPath string) (*ContractsSpec, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read contracts spec at %s: %w", specPath, err)
	}
	var spec ContractsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse contracts spec: %w", err)
	}
	validate := validator.New()
	validate.RegisterStructValidation(validateFileContract, FileContract{})
	if err := validate.Struct(&spec); err != nil {
		return nil, fmt.Errorf("contracts spec validation failed: %w", err)
	}
	return &spec, nil
}

// runContractFirstCheck loads contracts configuration and runs AST or polyglot validation.
// It verifies that code signatures matches contracts.yaml specification constraints.
func runContractFirstCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	res := StageResult{Name: "Contract-First Gate", Passed: true}
	specPath := config.ContractsPath(root)

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		res.Message = "Skipped: no global contracts.yaml spec found"
		return res, nil
	}

	spec, err := loadContractsSpec(specPath)
	if err != nil {
		res.Passed = false
		res.Error = err
		return res, nil
	}

	var failures []string
	for _, fc := range spec.Contracts {
		if errs := verifySingleContract(root, fc); len(errs) > 0 {
			failures = append(failures, errs...)
		}
	}

	if len(failures) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("contract-first signature drift detected:\n - %s", strings.Join(failures, "\n - "))
	} else {
		res.Message = fmt.Sprintf("Passed contract verification for %d source file(s)", len(spec.Contracts))
	}
	return res, nil
}

// verifySingleContract checks a single file contract and returns any drift findings.
// Supports both Go AST parsing and general polyglot regex pattern matching.
func verifySingleContract(root string, fc FileContract) []string {
	var failures []string
	fullPath := filepath.Join(root, fc.File)

	// Fail immediately if target contract file cannot be found.
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return []string{fmt.Sprintf("contract file %s does not exist", fc.File)}
	}

	// Route file checking based on language configuration.
	if strings.ToLower(fc.Language) == "go" {
		if errs := verifyGoContract(fullPath, fc.Symbols); len(errs) > 0 {
			failures = append(failures, errs...)
		}
	} else {
		if errs := verifyPolyglotContract(fullPath, fc.Symbols); len(errs) > 0 {
			failures = append(failures, errs...)
		}
	}
	return failures
}

// verifyGoContract parses a Go source file using Go AST and compares symbols to target contracts.
// It verifies public interface methods and package-level function signatures.
// verifyGoContract implements the AST-based inspection of a parsed Go file against a defined symbol contract.
// It searches the syntax tree for matching declarations (interfaces, structs, functions) and asserts their shape.
func verifyGoContract(filePath string, symbols []SymbolContract) []string {
	var errs []string
	fset := token.NewFileSet()
	// Parse the target Go source file into an AST structure.
	node, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return []string{fmt.Sprintf("failed to parse Go AST for %s: %v", filePath, err)}
	}

	// getTypeName extracts a simplified string representation of a type signature from its AST expression.
	// This allows verifying if a method's parameters and return types precisely match the contract definition.
	exprStr := func(expr ast.Expr) string {
		var buf bytes.Buffer
		fsetPrinter := token.NewFileSet()
		if err := printer.Fprint(&buf, fsetPrinter, expr); err != nil {
			return ""
		}
		return buf.String()
	}

	decls := collectDeclarations(node)

	// Iterate over each declared contract symbol.
	for _, sym := range symbols {
		if symErrs := verifySingleSymbol(sym, decls, exprStr); len(symErrs) > 0 {
			errs = append(errs, symErrs...)
		}
	}
	return errs
}

// verifySingleSymbol checks a single contract symbol against collected declarations.
func verifySingleSymbol(sym SymbolContract, decls fileDeclarations, exprStr func(ast.Expr) string) []string {
	if sym.Kind == "function" {
		fd, found := decls.Functions[sym.Name]
		if !found {
			return []string{fmt.Sprintf("expected symbol %s not found in Go AST", sym.Name)}
		}
		return matchGoFunction(fd, sym, exprStr)
	}
	if sym.Kind == "interface" {
		ts, found := decls.Interfaces[sym.Name]
		if !found {
			return []string{fmt.Sprintf("expected symbol %s not found in Go AST", sym.Name)}
		}
		return matchGoInterface(ts, sym, exprStr)
	}
	return nil
}

// fileDeclarations stores top-level function and interface declarations internally.
type fileDeclarations struct {
	Functions  map[string]*ast.FuncDecl
	Interfaces map[string]*ast.TypeSpec
}

// collectDeclarations aggregates top-level functions and interface types in a single pass.
func collectDeclarations(node *ast.File) fileDeclarations {
	decls := fileDeclarations{
		Functions:  make(map[string]*ast.FuncDecl),
		Interfaces: make(map[string]*ast.TypeSpec),
	}
	for _, d := range node.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			decls.Functions[fd.Name.Name] = fd
		} else if gd, ok := d.(*ast.GenDecl); ok {
			collectTypeSpec(gd, decls.Interfaces)
		}
	}
	return decls
}

// collectTypeSpec filters type declarations for interface definitions.
func collectTypeSpec(gd *ast.GenDecl, interfaces map[string]*ast.TypeSpec) {
	if gd.Tok != token.TYPE {
		return
	}
	for _, spec := range gd.Specs {
		if ts, ok := spec.(*ast.TypeSpec); ok {
			if _, isItf := ts.Type.(*ast.InterfaceType); isItf {
				interfaces[ts.Name.Name] = ts
			}
		}
	}
}

// matchGoFunction checks a top-level function declaration against a target symbol contract.
func matchGoFunction(fd *ast.FuncDecl, sym SymbolContract, exprStr func(ast.Expr) string) []string {
	var errs []string
	if len(sym.Params) > 0 {
		actual := getFuncParamTypes(fd.Type.Params, exprStr)
		if !sliceEquals(actual, sym.Params) {
			errs = append(errs, fmt.Sprintf("function %s params mismatch: expected %v, got %v", sym.Name, sym.Params, actual))
		}
	}
	if len(sym.Returns) > 0 {
		actual := getFuncParamTypes(fd.Type.Results, exprStr)
		if !sliceEquals(actual, sym.Returns) {
			errs = append(errs, fmt.Sprintf("function %s returns mismatch: expected %v, got %v", sym.Name, sym.Returns, actual))
		}
	}
	return errs
}

// getFuncParamTypes extracts parameter types from a function field list.
// It iterates over all fields and applies the provided expression stringifier
// to return a flattened slice of stringified types, allowing exact signature matching.
func getFuncParamTypes(fields *ast.FieldList, exprStr func(ast.Expr) string) []string {
	if fields == nil {
		return nil
	}
	var types []string
	for _, field := range fields.List {
		types = append(types, exprStr(field.Type))
	}
	return types
}

// Error messages related to signature and parameter validation.
const (
	errInterfaceMissing = "expected interface but missing"
	errMethodMissing    = "missing method %s in %s"
	errMethodParams     = "method %s params mismatch in %s"
	errMethodReturns    = "method %s returns mismatch in %s"
)

// InterfaceMethod stores interface method signatures.
type InterfaceMethod struct {
	Params  []string
	Returns []string
	HasSig  bool
}

// collectInterfaceMethods indexes all methods declared in an interface type.
// It maps the method names to their corresponding parameters and returns,
// skipping embedded interfaces or non-function fields for precise verification.
func collectInterfaceMethods(ityp *ast.InterfaceType, exprStr func(ast.Expr) string) map[string]InterfaceMethod {
	methods := make(map[string]InterfaceMethod)
	if ityp.Methods == nil {
		return methods
	}
	for _, field := range ityp.Methods.List {
		if len(field.Names) == 0 {
			continue
		}
		name := field.Names[0].Name
		ft, ok := field.Type.(*ast.FuncType)
		if !ok {
			methods[name] = InterfaceMethod{HasSig: false}
			continue
		}
		methods[name] = InterfaceMethod{
			Params:  getFuncParamTypes(ft.Params, exprStr),
			Returns: getFuncParamTypes(ft.Results, exprStr),
			HasSig:  true,
		}
	}
	return methods
}

// matchGoInterface checks an interface type declaration against target symbol methods.
func matchGoInterface(ts *ast.TypeSpec, sym SymbolContract, exprStr func(ast.Expr) string) []string {
	ityp, ok := ts.Type.(*ast.InterfaceType)
	if !ok {
		return []string{fmt.Sprintf("symbol %s is not an interface as expected", sym.Name)}
	}

	methods := collectInterfaceMethods(ityp, exprStr)
	var errs []string

	for _, mc := range sym.Methods {
		m, found := methods[mc.Name]
		if !found {
			errs = append(errs, fmt.Sprintf("interface %s is missing contract method %s", sym.Name, mc.Name))
			continue
		}
		if !m.HasSig {
			errs = append(errs, fmt.Sprintf("interface method %s.%s has no signature", sym.Name, mc.Name))
			continue
		}
		if !sliceEquals(m.Params, mc.Params) {
			errs = append(errs, fmt.Sprintf("interface method %s.%s params mismatch: expected %v, got %v", sym.Name, mc.Name, mc.Params, m.Params))
		}
		if !sliceEquals(m.Returns, mc.Returns) {
			errs = append(errs, fmt.Sprintf("interface method %s.%s returns mismatch: expected %v, got %v", sym.Name, mc.Name, mc.Returns, m.Returns))
		}
	}
	return errs
}

// verifyPolyglotContract validates non-Go source code using regex patterns.
// verifyPolyglotContract acts as a generic regex-based gate for non-Go languages.
// It iterates through the text payload and attempts to match all provided structural Regex patterns.
func verifyPolyglotContract(filePath string, symbols []SymbolContract) []string {
	var errs []string
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return []string{fmt.Sprintf("failed to read polyglot file %s: %v", filePath, err)}
	}
	content := string(contentBytes)

	for _, sym := range symbols {
		for _, pat := range sym.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				if !strings.Contains(content, pat) {
					errs = append(errs, fmt.Sprintf("polyglot pattern match failed for symbol %s: expected substring %q not found", sym.Name, pat))
				}
				continue
			}
			if !re.MatchString(content) {
				errs = append(errs, fmt.Sprintf("polyglot contract violation for symbol %s: pattern %q not found", sym.Name, pat))
			}
		}
	}
	return errs
}

// sliceEquals compares two slices of strings for exact sequence equality.
// It acts as a strict verification check to ensure parameter lists and
// return lists precisely match the desired contract specification.
func sliceEquals(a, b []string) bool {
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

// validateFileContract performs struct-level schema validation on FileContract entries.
// Enforces kind matching for Go symbols and pattern constraints for non-Go symbols.
// It dynamically dispatches to language-specific validators depending on the file language.
func validateFileContract(sl validator.StructLevel) {
	fc := sl.Current().Interface().(FileContract)
	if strings.ToLower(fc.Language) == "go" {
		validateGoSymbols(sl, fc.Symbols)
	} else {
		validatePolyglotSymbols(sl, fc.Symbols)
	}
}

// validateGoSymbols checks validation conditions for Go symbols.
func validateGoSymbols(sl validator.StructLevel, symbols []SymbolContract) {
	for i, sym := range symbols {
		if sym.Kind == "" {
			sl.ReportError(sym.Kind, fmt.Sprintf("Symbols[%d].Kind", i), "Kind", "required_for_go", "")
		} else if sym.Kind != "interface" && sym.Kind != "struct" && sym.Kind != "function" {
			sl.ReportError(sym.Kind, fmt.Sprintf("Symbols[%d].Kind", i), "Kind", "oneof=interface struct function", "")
		}
	}
}

// validatePolyglotSymbols checks validation conditions for polyglot symbols.
func validatePolyglotSymbols(sl validator.StructLevel, symbols []SymbolContract) {
	for i, sym := range symbols {
		if len(sym.Patterns) == 0 {
			sl.ReportError(sym.Patterns, fmt.Sprintf("Symbols[%d].Patterns", i), "Patterns", "required_for_polyglot", "")
		}
	}
}
