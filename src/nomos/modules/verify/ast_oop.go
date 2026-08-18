package verify

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// OOPFinding represents a violation of the Anti-OOP "No Single-Implementation Interface" rule.
type OOPFinding struct {
	File    string
	Line    int
	Message string
}

// analyzeASTInterfaces parses all .go files in the project and returns findings
// for interfaces that lack multiple concrete implementations (excluding mocks).
func analyzeASTInterfaces(root string, stagedOnly bool) ([]OOPFinding, error) {
	var files []string
	var err error

	if stagedOnly {
		files, err = getStagedFiles(root)
	} else {
		files, err = getProjectFiles(root)
	}
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	// methodSets maps a struct's name to its list of method signatures.
	structMethodSets := make(map[string]map[string]bool)
	// interfaceSets maps an interface's name (and its file/line location) to its method signatures.
	type ifaceLocation struct {
		file string
		line int
	}
	interfaceSets := make(map[ifaceLocation]map[string]bool)

	// mockTypes tracks the names of structs defined in _test.go files.
	mockTypes := make(map[string]bool)

	for _, f := range files {
		fSlash := filepath.ToSlash(f)
		if !strings.HasSuffix(fSlash, ".go") || isIgnoredPath(fSlash) || workspace.IsInternalSystemDir(fSlash) {
			continue
		}

		absPath := filepath.Join(root, f)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		node, err := parser.ParseFile(fset, absPath, content, 0)
		if err != nil {
			continue
		}

		isTest := strings.HasSuffix(fSlash, "_test.go")

		ast.Inspect(node, func(n ast.Node) bool {
			// Extract Interface definitions
			if typeSpec, ok := n.(*ast.TypeSpec); ok {
				if ifaceType, ok := typeSpec.Type.(*ast.InterfaceType); ok {
					pos := fset.Position(typeSpec.Pos())
					loc := ifaceLocation{file: f, line: pos.Line}
					methods := extractInterfaceMethods(fset, ifaceType)
					interfaceSets[loc] = methods
				}
				// Track mock structs
				if isTest {
					if _, ok := typeSpec.Type.(*ast.StructType); ok {
						mockTypes[typeSpec.Name.Name] = true
					}
				}
			}

			// Extract Struct Method definitions
			if funcDecl, ok := n.(*ast.FuncDecl); ok && funcDecl.Recv != nil {
				recvType := funcDecl.Recv.List[0].Type
				structName := getStructName(recvType)
				if structName != "" {
					if structMethodSets[structName] == nil {
						structMethodSets[structName] = make(map[string]bool)
					}
					sig := funcDecl.Name.Name + stringifyFuncType(fset, funcDecl.Type)
					structMethodSets[structName][sig] = true
				}
			}

			return true
		})
	}

	var findings []OOPFinding

	// Compare interfaces against structs
	for loc, ifaceMethods := range interfaceSets {
		if len(ifaceMethods) == 0 {
			continue // Ignore empty interfaces (e.g. interface{})
		}

		implementationCount := 0
		var implNames []string

		for structName, structMethods := range structMethodSets {
			implements := true
			for methodSig := range ifaceMethods {
				if !structMethods[methodSig] {
					implements = false
					break
				}
			}
			if implements {
				implementationCount++
				implNames = append(implNames, structName)
			}
		}

		if implementationCount < 2 {
			// If it's 1 implementation and it's a mock, we allow it if there's NO primary implementation.
			// Actually, wait. If there's 1 implementation and it's a mock, it's weird.
			// If there's exactly 1 implementation, and it's a mock, it fails because there's no real implementation.
			// If there's exactly 1 implementation, and it's NOT a mock, it's a single-implementation interface.
			// If there are 2 implementations (1 real, 1 mock), it passes!
			
			// So if implementationCount < 2, it is always a violation, because even if the 1 is a mock, 
			// it means there's no actual implementation!
			
			// Wait, what if the interface is exported for OTHERS to implement, but we don't implement it?
			// The rule says "No Single-Implementation Interfaces".
			// Let's just flag it.
			implStr := "none"
			if len(implNames) > 0 {
				implStr = implNames[0]
			}
			
			findings = append(findings, OOPFinding{
				File:    loc.file,
				Line:    loc.line,
				Message: fmt.Sprintf("Anti-OOP Violation: Interface has %d concrete implementations (found: %s). Must have >= 2 to justify abstraction.", implementationCount, implStr),
			})
		}
	}

	return findings, nil
}

// extractInterfaceMethods extracts and normalizes the method signatures of an interface.
func extractInterfaceMethods(fset *token.FileSet, iface *ast.InterfaceType) map[string]bool {
	methods := make(map[string]bool)
	if iface.Methods == nil {
		return methods
	}
	for _, field := range iface.Methods.List {
		if funcType, ok := field.Type.(*ast.FuncType); ok {
			for _, name := range field.Names {
				sig := name.Name + stringifyFuncType(fset, funcType)
				methods[sig] = true
			}
		} else {
			// Handle embedded interfaces by ignoring them for now, or just recording their name.
			// A true AST scanner would resolve embedded interfaces, but this heuristic focuses on direct methods.
		}
	}
	return methods
}

// stringifyFuncType normalizes a function signature by extracting parameter and return types (stripping names).
func stringifyFuncType(fset *token.FileSet, funcType *ast.FuncType) string {
	params := stringifyFieldList(fset, funcType.Params)
	results := stringifyFieldList(fset, funcType.Results)
	return fmt.Sprintf("(%s) (%s)", params, results)
}

func stringifyFieldList(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var types []string
	for _, field := range fl.List {
		typeStr := nodeToString(fset, field.Type)
		count := 1
		if len(field.Names) > 0 {
			count = len(field.Names)
		}
		for i := 0; i < count; i++ {
			types = append(types, typeStr)
		}
	}
	return strings.Join(types, ", ")
}

func nodeToString(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, node)
	return buf.String()
}

func getStructName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// runAntiOOPCheck executes the AST OOP audit.
func runAntiOOPCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot

	// Run across all files in the active workspace
	findings, err := analyzeASTInterfaces(root, false)
	if err != nil {
		return StageResult{Passed: false, Message: "Failed to parse AST for OOP rules"}, err
	}

	if len(findings) == 0 {
		return StageResult{Passed: true, Message: "No Anti-OOP violations detected."}, nil
	}

	errDetails := "Anti-OOP Violations Found (Single-Implementation Interfaces):\n"
	for _, f := range findings {
		errDetails += fmt.Sprintf(" - %s:%d %s\n", f.File, f.Line, f.Message)
	}

	return StageResult{
		Passed: false,
	}, fmt.Errorf("anti-OOP constraints violated:\n%s", errDetails)
}
