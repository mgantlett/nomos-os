package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"os"
	"path/filepath"
	"testing"
)

func TestVerifyGoContract(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	srcCode := `package testpkg

import "context"

type DummyInterface interface {
	DoSomething(ctx context.Context, id string) error
}

func StandardFunction(name string, count int) (bool, error) {
	return true, nil
}
`
	srcPath := filepath.Join(tempDir, "source.go")
	if err := os.WriteFile(srcPath, []byte(srcCode), 0644); err != nil {
		t.Fatalf("failed to write source code file: %v", err)
	}

	tests := []struct {
		name        string
		symbols     []SymbolContract
		expectDrift bool
	}{
		{
			name: "Success - Interface matches exactly",
			symbols: []SymbolContract{
				{
					Name: "DummyInterface",
					Kind: "interface",
					Methods: []MethodContract{
						{
							Name:    "DoSomething",
							Params:  []string{"context.Context", "string"},
							Returns: []string{"error"},
						},
					},
				},
			},
			expectDrift: false,
		},
		{
			name: "Success - Function signature matches exactly",
			symbols: []SymbolContract{
				{
					Name:    "StandardFunction",
					Kind:    "function",
					Params:  []string{"string", "int"},
					Returns: []string{"bool", "error"},
				},
			},
			expectDrift: false,
		},
		{
			name: "Failure - Interface missing method",
			symbols: []SymbolContract{
				{
					Name: "DummyInterface",
					Kind: "interface",
					Methods: []MethodContract{
						{
							Name:    "NonExistentMethod",
							Params:  []string{"string"},
							Returns: []string{"error"},
						},
					},
				},
			},
			expectDrift: true,
		},
		{
			name: "Failure - Method parameter mismatch",
			symbols: []SymbolContract{
				{
					Name: "DummyInterface",
					Kind: "interface",
					Methods: []MethodContract{
						{
							Name:    "DoSomething",
							Params:  []string{"context.Context", "int"}, // int instead of string
							Returns: []string{"error"},
						},
					},
				},
			},
			expectDrift: true,
		},
		{
			name: "Failure - Function parameter mismatch",
			symbols: []SymbolContract{
				{
					Name:    "StandardFunction",
					Kind:    "function",
					Params:  []string{"string", "string"}, // string instead of int
					Returns: []string{"bool", "error"},
				},
			},
			expectDrift: true,
		},
		{
			name: "Failure - Symbol missing entirely",
			symbols: []SymbolContract{
				{
					Name: "MissingInterface",
					Kind: "interface",
				},
			},
			expectDrift: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := verifyGoContract(srcPath, tc.symbols)
			if tc.expectDrift {
				if len(errs) == 0 {
					t.Errorf("expected signature drift errors, got none")
				}
			} else {
				if len(errs) > 0 {
					t.Errorf("expected no drift errors, got: %v", errs)
				}
			}
		})
	}
}

func TestVerifyPolyglotContract(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	polyglotCode := `
export interface ITracker {
  start(context: any, key: string, assignee: string): Promise<void>;
  close(context: any, key: string, comment: string): Promise<void>;
}
`
	filePath := filepath.Join(tempDir, "tracker.ts")
	if err := os.WriteFile(filePath, []byte(polyglotCode), 0644); err != nil {
		t.Fatalf("failed to write polyglot file: %v", err)
	}

	tests := []struct {
		name        string
		symbols     []SymbolContract
		expectDrift bool
	}{
		{
			name: "Success - Patterns match",
			symbols: []SymbolContract{
				{
					Name: "ITracker",
					Patterns: []string{
						"interface ITracker",
						`start\(context: any, key: string, assignee: string\): Promise<void>`,
					},
				},
			},
			expectDrift: false,
		},
		{
			name: "Failure - Pattern missing",
			symbols: []SymbolContract{
				{
					Name: "ITracker",
					Patterns: []string{
						"interface ITracker",
						`start\(context: string, key: string\)`, // mismatch param type/count
					},
				},
			},
			expectDrift: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := verifyPolyglotContract(filePath, tc.symbols)
			if tc.expectDrift {
				if len(errs) == 0 {
					t.Errorf("expected signature drift, got none")
				}
			} else {
				if len(errs) > 0 {
					t.Errorf("expected no drift, got: %v", errs)
				}
			}
		})
	}
}

func TestContractsYamlValidation(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		expectPass  bool
	}{
		{
			name: "Success - Valid contracts spec",
			yamlContent: `
contracts:
  - file: "src/main.go"
    language: "go"
    symbols:
      - name: "MainFunc"
        kind: "function"
        params: ["string"]
        returns: ["error"]
`,
			expectPass: true,
		},
		{
			name: "Failure - Missing file field",
			yamlContent: `
contracts:
  - language: "go"
    symbols:
      - name: "MainFunc"
        kind: "function"
`,
			expectPass: false,
		},
		{
			name: "Failure - Missing language field",
			yamlContent: `
contracts:
  - file: "src/main.go"
    symbols:
      - name: "MainFunc"
        kind: "function"
`,
			expectPass: false,
		},
		{
			name: "Failure - Go symbol missing kind",
			yamlContent: `
contracts:
  - file: "src/main.go"
    language: "go"
    symbols:
      - name: "MainFunc"
`,
			expectPass: false,
		},
		{
			name: "Failure - Go symbol invalid kind",
			yamlContent: `
contracts:
  - file: "src/main.go"
    language: "go"
    symbols:
      - name: "MainFunc"
        kind: "invalid-kind"
`,
			expectPass: false,
		},
		{
			name: "Failure - Polyglot symbol missing patterns",
			yamlContent: `
contracts:
  - file: "src/index.ts"
    language: "typescript"
    symbols:
      - name: "ITracker"
`,
			expectPass: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			var err error
			_ = err
			if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
				t.Fatalf("failed to create .nomos dir: %v", err)
			}

			// Write contracts.yaml spec file.
			specPath := workspace.MustNewContext(tempDir).DataPath("contracts.yaml")
			os.MkdirAll(filepath.Dir(specPath), 0755)
			if err := os.WriteFile(specPath, []byte(tc.yamlContent), 0644); err != nil {
				t.Fatalf("failed to write contracts.yaml: %v", err)
			}

			// We need the referenced file to exist so it doesn't fail on verifySingleContract.
			// Let's create dummy files for the test:
			dummyGo := filepath.Join(tempDir, "src/main.go")
			_ = os.MkdirAll(filepath.Dir(dummyGo), 0755)
			_ = os.WriteFile(dummyGo, []byte("package main\nfunc MainFunc(s string) error { return nil }\n"), 0644)

			dummyTs := filepath.Join(tempDir, "src/index.ts")
			_ = os.WriteFile(dummyTs, []byte(""), 0644)

			res, err := runContractFirstCheck(&workspace.WorkspaceContext{RepoRoot: tempDir})
			if err != nil {
				t.Fatalf("unexpected handler error: %v", err)
			}

			if tc.expectPass {
				if !res.Passed {
					t.Errorf("expected validation to pass, but failed: %v", res.Error)
				}
			} else {
				if res.Passed {
					t.Errorf("expected validation to fail, but it passed successfully")
				}
			}
		})
	}
}
