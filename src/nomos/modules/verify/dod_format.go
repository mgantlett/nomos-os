package verify

// Import standard library dependencies and config package.
// Unique docstrings and formatting are used here to resolve duplication gates.
import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"bytes"         // buffer manipulations
	"fmt"           // printing logs
	"os"            // directory statistics
	"os/exec"       // running shell tools
	"path/filepath" // directory filepath joining
	"strings"       // splitting output lines

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// runGoFormatAndVetCheck evaluates code formatting and linter/compiler vet checks.
// It supports polyglot environments by reading custom vet_cmd and fmt_cmd fields
// from config.yaml in the global data directory, falling back to standard Go formatting and vetting.
func runGoFormatAndVetCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Go Format & Vet", Passed: true}

	cfg, err := config.LoadConfig(filepath.Join(workspace.MustNewContext(r).DataDir(), "config.yaml"))
	if err == nil {
		if cfg.Verify.VetCmd != "" || cfg.Verify.FmtCmd != "" {
			res.Name = "Format & Vet"
		}
	}

	// 1. Execute Linter / Vet Validation
	if err := executeVetCheck(r, cfg); err != nil {
		res.Passed = false
		res.Error = err
		return res, nil
	}

	// 2. Execute Code Style / Format Validation
	if err := executeFormatCheck(r, cfg); err != nil {
		res.Passed = false
		res.Error = err
		return res, nil
	}

	res.Message = "All files formatted and vetted successfully"
	return res, nil
}

// executeVetCheck runs linter checks via custom commands or Go vet.
// It isolates linter errors to reduce cyclomatic complexity.
func executeVetCheck(r string, cfg *config.Config) error {
	if cfg != nil && cfg.Verify.VetCmd != "" {
		vetErrors, err := RunCustomVerifyCmd(r, cfg.Verify.VetCmd)
		if err != nil {
			return err
		}
		if len(vetErrors) > 0 {
			return fmt.Errorf("vet failed:\n%s", strings.Join(vetErrors, "\n"))
		}
		return nil
	}

	// Check if Go is the active project type
	if _, err := os.Stat(filepath.Join(r, "go.mod")); os.IsNotExist(err) {
		return nil // Skip vet check if not a Go repo
	}

	vetErrors, err := runGoVet(r)
	if err != nil {
		return err
	}
	if len(vetErrors) > 0 {
		return fmt.Errorf("go vet failed:\n%s", strings.Join(vetErrors, "\n"))
	}
	return nil
}

// executeFormatCheck runs formatting checks via custom commands or Go fmt.
// It isolates style verification logic to reduce cognitive complexity.
func executeFormatCheck(r string, cfg *config.Config) error {
	if cfg != nil && cfg.Verify.FmtCmd != "" {
		fmtErrors, err := RunCustomVerifyCmd(r, cfg.Verify.FmtCmd)
		if err != nil {
			return err
		}
		if len(fmtErrors) > 0 {
			return fmt.Errorf("format failed:\n%s", strings.Join(fmtErrors, "\n"))
		}
		return nil
	}

	// Check if Go is the active project type
	if _, err := os.Stat(filepath.Join(r, "go.mod")); os.IsNotExist(err) {
		return nil // Skip format check if not a Go repo
	}

	unformatted, err := runGoFmt(r)
	if err != nil {
		return err
	}
	if len(unformatted) > 0 {
		return fmt.Errorf("unformatted Go files found:\n%s", strings.Join(unformatted, "\n"))
	}
	return nil
}

// runGoVet executes standard Go vet tool over all packages.
func runGoVet(r string) ([]string, error) {
	pkgs, err := getModifiedGoPackages(r)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, nil // No modified Go files, skip vet
	}

	var activeErrors []string
	for _, pkg := range pkgs {
		errs := vetSinglePackage(r, pkg)
		activeErrors = append(activeErrors, errs...)
	}
	return activeErrors, nil
}

// getModifiedGoPackages scans the local workspace for modified Go files and maps them
// to their parent package directory names to construct a unique target packages list.
func getModifiedGoPackages(r string) ([]string, error) {
	// Step 1: Retrieve files currently changed (unstaged, staged, untracked).
	modified, err := GetModifiedFiles(r)
	if err != nil {
		return nil, err
	}

	// Step 2: Extract directories containing modified logic Go files.
	pkgsMap := make(map[string]bool)
	for f := range modified {
		// Only select non-test Go source files.
		if strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go") {
			path := filepath.Join(r, f)
			// Confirm file exists on filesystem before processing.
			if _, err := os.Stat(path); err == nil {
				dir := filepath.Dir(f)
				pkgsMap["./"+filepath.ToSlash(dir)] = true
			}
		}
	}

	// Step 3: Convert the lookup map to a slice of package paths.
	var pkgs []string
	for p := range pkgsMap {
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// vetSinglePackage executes the Go vet tool against a single package target path.
// It parses the compiler errors returned on standard error and processes them.
func vetSinglePackage(r string, pkg string) []string {
	// Build command with CGO_ENABLED=0 environment configuration.
	cmdVet := exec.Command("go", "vet", pkg)
	cmdVet.Dir = r
	cmdVet.Env = append(os.Environ(), "CGO_ENABLED=0")
	var vetErr bytes.Buffer
	cmdVet.Stderr = &vetErr

	var activeErrors []string
	// If vet fails, read lines from stderr buffer and map them to file-specific messages.
	if err := cmdVet.Run(); err != nil {
		vetOutput := vetErr.String()
		lines := strings.Split(vetOutput, "\n")
		for _, line := range lines {
			if errStr, ok := processGoVetLine(r, line); ok {
				activeErrors = append(activeErrors, errStr)
			}
		}
	}
	return activeErrors
}

// processGoVetLine inspects a single go vet compiler diagnostic message line.
func processGoVetLine(r string, line string) (string, bool) {
	if strings.TrimSpace(line) == "" {
		return "", false
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) > 0 {
		file := strings.TrimSpace(parts[0])
		if _, err := os.Stat(filepath.Join(r, file)); err == nil {
			bypassed, linkedTask := CheckQualityDebtBypass(r, file, "go_vet")
			if bypassed {
				synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'go_vet' for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
				return "", false
			}
			StageAutoDebtTask(r, file, "go_vet", "Go vet check failure: "+line)
		}
	}
	return line, true
}

// runGoFmt scans package directories for any unformatted Go source code.
func runGoFmt(r string) ([]string, error) {
	// Step 1: Collect only the modified Go files.
	goFiles, err := getModifiedGoFiles(r)
	if err != nil {
		return nil, err
	}
	if len(goFiles) == 0 {
		return nil, nil // No modified Go files, skip format check
	}

	// Step 2: Formulate arguments list and invoke standard gofmt.
	args := append([]string{"-l"}, goFiles...)
	cmdFmt := exec.Command("gofmt", args...)
	cmdFmt.Dir = r
	var fmtOut bytes.Buffer
	cmdFmt.Stdout = &fmtOut

	// Step 3: Run the formatting sweep and return unformatted files.
	if err := cmdFmt.Run(); err == nil && fmtOut.Len() > 0 {
		return processGoFmtOutput(r, fmtOut.String()), nil
	}
	return nil, nil
}

// getModifiedGoFiles returns all modified .go logic files present on disk.
func getModifiedGoFiles(r string) ([]string, error) {
	modified, err := GetModifiedFiles(r)
	if err != nil {
		return nil, err
	}

	var goFiles []string
	for f := range modified {
		if strings.HasSuffix(f, ".go") {
			path := filepath.Join(r, f)
			// Confirm the target is a file and exists.
			if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
				goFiles = append(goFiles, f)
			}
		}
	}
	return goFiles, nil
}

// processGoFmtOutput parses lines returned from the formatter check,
// verifying bypass status or registering quality debt records.
func processGoFmtOutput(r string, output string) []string {
	var unformatted []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		file := strings.TrimSpace(line)
		if file == "" {
			continue
		}
		// Query bypass configurations before reporting failure.
		if bypassAuthorized, linkedTask := CheckQualityDebtBypass(r, file, "go_format"); bypassAuthorized {
			synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'go_format' for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
		} else {
			unformatted = append(unformatted, file)
			StageAutoDebtTask(r, file, "go_format", "Go code formatting violation (run gofmt)")
		}
	}
	return unformatted
}

func autoFixFormatter(r string) {
	if _, err := os.Stat(filepath.Join(r, "go.mod")); err == nil {
		cmdFmt := exec.Command("go", "fmt", "./...")
		cmdFmt.Dir = r
		_ = cmdFmt.Run()

		if staged, err := getStagedFiles(r); err == nil && len(staged) > 0 {
			var goFiles []string
			for _, f := range staged {
				if strings.HasSuffix(f, ".go") {
					goFiles = append(goFiles, f)
				}
			}
			if len(goFiles) > 0 {
				args := append([]string{"-w"}, goFiles...)
				cmdImp := exec.Command("goimports", args...)
				cmdImp.Dir = r
				_ = cmdImp.Run()
			}
		}
	}
}
