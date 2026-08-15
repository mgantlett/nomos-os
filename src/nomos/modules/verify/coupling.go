package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	"github.com/mgantlett/nomos-os/src/nomos/engine"
)

// runCouplingAnalysisCheck executes package coupling and cohesion checks.
// It detects cyclic package dependencies and lists Ca (Afferent) and Ce (Efferent) coupling.
// The checks enforce a unidirectional dependency graph across all nomos internal modules.
// Returns a StageResult encapsulating the execution outcome, error flags, and recorded metrics.
// This metric prevents the codebase from deteriorating into a monolithic 'big ball of mud'.
func runCouplingAnalysisCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	res := StageResult{Name: "Coupling Analysis Check", Passed: true}

	// 1. Discover all internal Go source files
	goFiles, err := getInternalGoFiles(root)
	if err != nil {
		return res, nil
	}

	// 2. Build internal package dependency maps
	dependsOn, dependedOnBy, err := buildDependencyMaps(root, goFiles)
	if err != nil {
		return res, nil
	}

	// 3. Scan for dependency cycles
	cycle := findDependencyCycle(dependsOn)
	if len(cycle) > 0 {
		cycleStr := strings.Join(cycle, " -> ")
		res.Passed = false
		res.Error = fmt.Errorf("package coupling cycle detected: %s", cycleStr)

		// Autonomous Auto-Remediation hook:
		// Dispatch Swarm worker to heal the cyclic dependency tree
		engine.RemediateASTCycle(root, cycleStr)

		return res, nil
	}

	// 4. Print stats and results
	var stats []string
	var maxCa, maxCe int
	for pkg := range dependsOn {
		ce := len(dependsOn[pkg])
		ca := len(dependedOnBy[pkg])
		if ca > maxCa {
			maxCa = ca
		}
		if ce > maxCe {
			maxCe = ce
		}
		stats = append(stats, fmt.Sprintf("   ↳ %s: Afferent Coupling (Ca) = %d, Efferent Coupling (Ce) = %d", pkg, ca, ce))
	}

	res.Metrics = map[string]interface{}{
		"max_afferent_coupling": maxCa,
		"max_efferent_coupling": maxCe,
	}

	res.Message = "All package coupling checks passed (No cycles detected).\n" + strings.Join(stats, "\n")
	return res, nil
}

// buildDependencyMaps constructs outgoing and incoming dependency tables from a set of files.
func buildDependencyMaps(root string, goFiles []string) (map[string]map[string]bool, map[string]map[string]bool, error) {
	dependsOn := make(map[string]map[string]bool)
	dependedOnBy := make(map[string]map[string]bool)

	for _, file := range goFiles {
		err := processFileImports(root, file, dependsOn, dependedOnBy)
		if err != nil {
			continue
		}
	}
	return dependsOn, dependedOnBy, nil
}

// processFileImports extracts and maps imports for a single Go file.
func processFileImports(root, file string, dependsOn, dependedOnBy map[string]map[string]bool) error {
	relDir := filepath.Dir(file)
	pkgPath := filepath.ToSlash(relDir)

	imports, err := ast.ParseImports(filepath.Join(root, file))
	if err != nil {
		return err
	}

	if _, ok := dependsOn[pkgPath]; !ok {
		dependsOn[pkgPath] = make(map[string]bool)
	}

	for _, imp := range imports {
		// Filter and map internal imports
		if strings.HasPrefix(imp, "github.com/mgantlett/nomos-commons/") {
			targetPkg := strings.TrimPrefix(imp, "github.com/mgantlett/nomos-commons/")
			targetPkg = strings.TrimPrefix(targetPkg, "src/")
			targetPkg = "src/" + filepath.Clean(targetPkg)

			if targetPkg != pkgPath {
				dependsOn[pkgPath][targetPkg] = true
				if _, ok := dependedOnBy[targetPkg]; !ok {
					dependedOnBy[targetPkg] = make(map[string]bool)
				}
				dependedOnBy[targetPkg][pkgPath] = true
			}
		}
	}
	return nil
}

// getInternalGoFiles returns a slice of all non-test Go source files in the project.
// It searches under the src directory and excludes any file names ending with _test.go.
func getInternalGoFiles(root string) ([]string, error) {
	var files []string
	// Walk the project's source directory recursively
	err := filepath.Walk(filepath.Join(root, "src"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories and test files, only matching standard Go source files
		if !info.IsDir() && filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})
	return files, err
}

// cycleDetector implements topological DFS cycle detection on a graph map.
// It tracks node visiting states to determine whether a cycle exists in the dependency tree.
type cycleDetector struct {
	dependsOn map[string]map[string]bool
	visited   map[string]int // 0: unvisited, 1: visiting, 2: visited (fully processed)
	path      []string       // active recursive call stack path for cycle reconstruction
}

// dfs recursively traverses package dependencies to find cyclic loops.
// It returns true if a cycle is detected, tracing back edges using DFS.
func (cd *cycleDetector) dfs(node string) bool {
	// Mark current node as visiting (entering recursive stack)
	cd.visited[node] = 1
	cd.path = append(cd.path, node)

	for neighbor := range cd.dependsOn[node] {
		// Back edge found (neighbor is already in the active recursion stack)
		if cd.visited[neighbor] == 1 {
			for _, val := range cd.path {
				if val == neighbor {
					return true
				}
			}
		}
		// Recursively visit unvisited neighbors
		if cd.visited[neighbor] == 0 {
			if cd.dfs(neighbor) {
				return true
			}
		}
	}

	// Mark node as fully processed (exiting recursive stack)
	cd.visited[node] = 2
	cd.path = cd.path[:len(cd.path)-1]
	return false
}

// findDependencyCycle analyzes package dependency links to verify no cycles exist.
func findDependencyCycle(dependsOn map[string]map[string]bool) []string {
	cd := &cycleDetector{
		dependsOn: dependsOn,
		visited:   make(map[string]int),
	}

	for node := range dependsOn {
		if cd.visited[node] == 0 {
			if cd.dfs(node) {
				return cd.path
			}
		}
	}

	return nil
}

// PackageCouplingStats holds incoming/outgoing counts and raw import lists for a package.
type PackageCouplingStats struct {
	PackageName string   `json:"package"`
	Afferent    int      `json:"afferent"`
	Efferent    int      `json:"efferent"`
	Imports     []string `json:"imports"`
	ImportedBy  []string `json:"importedBy"`
}

// CouplingReport aggregates the statistics for all internal packages and highlights any cycles.
type CouplingReport struct {
	Packages []PackageCouplingStats `json:"packages"`
	Cycle    []string               `json:"cycle"`
}

// GetCouplingReport calculates the afferent and efferent package coupling report.
func GetCouplingReport(root string) (*CouplingReport, error) {
	goFiles, err := getInternalGoFiles(root)
	if err != nil {
		return nil, err
	}

	dependsOn, dependedOnBy, err := buildDependencyMaps(root, goFiles)
	if err != nil {
		return nil, err
	}

	cycle := findDependencyCycle(dependsOn)

	var pkgs []PackageCouplingStats
	for pkg := range dependsOn {
		var imps []string
		for imp := range dependsOn[pkg] {
			imps = append(imps, imp)
		}
		sort.Strings(imps)

		impBy := []string{}
		for imp := range dependedOnBy[pkg] {
			impBy = append(impBy, imp)
		}
		sort.Strings(impBy)

		pkgs = append(pkgs, PackageCouplingStats{
			PackageName: pkg,
			Afferent:    len(dependedOnBy[pkg]),
			Efferent:    len(dependsOn[pkg]),
			Imports:     imps,
			ImportedBy:  impBy,
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].PackageName < pkgs[j].PackageName
	})

	return &CouplingReport{
		Packages: pkgs,
		Cycle:    cycle,
	}, nil
}
