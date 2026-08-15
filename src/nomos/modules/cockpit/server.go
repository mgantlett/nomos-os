/*
Package cockpit provides the embedded web dashboard server for Nomos Open Core Edition.
It serves static web assets, local backlog REST APIs, system health doctor endpoints,
Server-Sent Events (SSE) log streaming, and plugin extension middleware hooks.
*/
package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
)

// Server represents the embedded Cockpit HTTP and SSE server instance.
// It manages network routes, workspace state tracking, and plugin registration lists.
type Server struct {
	// port defines the local TCP port listener number for HTTP traffic.
	port int
	// repoRoot stores the absolute path to the active workspace repository.
	repoRoot string
	// tracker handles local backlog task querying and mutations.
	tracker task.Tracker
	// httpServer holds the underlying net/http Server reference.
	httpServer *http.Server
	// mu synchronizes access to registered enterprise plugins array.
	mu sync.Mutex
	// plugins stores the slice of active enterprise plugin registration names.
	plugins []string
	// cachedBacklog stores cached task backlog entries to prevent thrashing.
	cachedBacklog []map[string]interface{}
	// cachedBacklogTime tracks the timestamp of the last backlog database query.
	cachedBacklogTime time.Time
	// backlogMu guards concurrent access to the task backlog cache.
	backlogMu sync.RWMutex
}

// pluginRegistration represents an enterprise plugin registering routes with the Cockpit server.
type pluginRegistration struct {
	// Name is the unique identifier string of the registering plugin.
	Name string `json:"name"`
	// Endpoint is the relative API route prefix exposed by the plugin.
	Endpoint string `json:"endpoint"`
	// TargetURL is the destination HTTP proxy URL for forwarding requests.
	TargetURL string `json:"target_url"`
}

// NewServer initializes a new Cockpit HTTP server instance bound to the given repoRoot and port.
func NewServer(repoRoot string, port int, tracker task.Tracker) *Server {
	if tracker == nil && repoRoot != "" {
		cfg := &task.Config{TrackerType: "local", RepoRoot: repoRoot}
		if tr, err := task.NewTracker(cfg); err == nil {
			tracker = tr
		}
	}
	// Instantiate Server struct with repository root, listening port, and task tracker
	return &Server{
		port:     port,
		repoRoot: repoRoot,
		tracker:  tracker,
		plugins:  []string{},
	}
}

// NewBaseHandler returns an initialized Cockpit Server (which implements http.Handler) with the default base routes.
// Referenced by: github.com/mgantlett/nomos-cockpit/src/server (Sovereign Edition external module).
// Sovereign's NewServer mounts this as a fallback handler to inherit all Open Core base routes.
func NewBaseHandler(repoRoot string) http.Handler {
	return NewServer(repoRoot, 0, nil)
}

// ServeHTTP implements http.Handler for the Cockpit Server.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/backlog", s.handleBacklog)
	mux.HandleFunc("/api/backlog/", s.handleBacklog)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/features", s.handleFeatures)
	mux.HandleFunc("/api/swarm", s.handleSwarmDisabled)
	mux.HandleFunc("/api/fleet", s.handleFleetDisabled)
	mux.HandleFunc("/api/graph", s.handleGraphDisabled)
	mux.HandleFunc("/api/branches/audit", s.handleBranchesDisabled)
	mux.HandleFunc("/api/quality-debt", s.handleDebtDisabled)
	mux.HandleFunc("/api/artifacts/", s.handleArtifactsDisabled)
	mux.HandleFunc("/api/search", s.handleSearchDisabled)
	mux.HandleFunc("/api/gitbrain", s.handleGitBrainDisabled)
	mux.HandleFunc("/api/drift", s.handleDriftDisabled)
	mux.HandleFunc("/api/analytics", s.handleAnalyticsDisabled)

	mux.HandleFunc("/api/plugin/register", s.handleRegisterPlugin)

	s.registerWorkspaceRoutes(mux)
	_ = s.registerStaticRoutes(mux)

	mux.ServeHTTP(w, r)
}

func (s *Server) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/git/status", func(w http.ResponseWriter, r *http.Request) {
		HandleGitStatusRoute(w, r, s.repoRoot)
	})
	mux.HandleFunc("/api/git/diff", func(w http.ResponseWriter, r *http.Request) {
		HandleGitDiffRoute(w, r, s.repoRoot)
	})
	mux.HandleFunc("/api/git/stage", func(w http.ResponseWriter, r *http.Request) {
		HandleGitStageRoute(w, r, s.repoRoot)
	})
	mux.HandleFunc("/api/task/transition", func(w http.ResponseWriter, r *http.Request) {
		HandleApiTaskTransition(w, r, s.repoRoot)
	})
	mux.HandleFunc("/api/task/reset", func(w http.ResponseWriter, r *http.Request) {
		HandleApiTaskReset(w, r, s.repoRoot)
	})
	mux.HandleFunc("/api/worktrees/prune", func(w http.ResponseWriter, r *http.Request) {
		HandleApiWorktreesPrune(w, r, s.repoRoot)
	})
}

// registerStaticRoutes mounts the embedded web assets handler to the root URL.
func (s *Server) registerStaticRoutes(mux *http.ServeMux) error {
	// Create virtual sub-filesystem for embedded UI web assets
	uiFS, err := fs.Sub(Assets, "ui")
	if err != nil {
		return err
	}

	// Mount dynamic disk-first static file handler with embedded filesystem fallback
	// This ensures hot-reloading development assets on disk are immediately served without re-compilation.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Clean and normalize incoming HTTP request URL path
		p := r.URL.Path
		if p == "/" {
			// Default root path request to index.html
			p = "/index.html"
		}
		if p == "/favicon.ico" {
			// Route default browser favicon requests to mascot asset
			p = "/public/assets/nomos_mascot_clean.png"
		}
		// Strip leading slash for relative filepath joining
		p = strings.TrimPrefix(p, "/")
		// Clean path string to prevent path traversal
		cleanPath := filepath.Clean(p)

		// Check disk candidates for dev hot-reloading and live updates
		candidates := []string{
			filepath.Join(s.repoRoot, "src", "nomos", "modules", "cockpit", "ui"),
		}
		for _, cand := range candidates {
			// Formulate absolute target filepath on host disk
			target := filepath.Join(cand, cleanPath)
			// Inspect if target file exists and is not a directory
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				// Serve file contents directly from disk
				http.ServeFile(w, r, target)
				// Return response successfully
				return
			}
		}

		// Fallback to embedded filesystem assets
		http.FileServer(http.FS(uiFS)).ServeHTTP(w, r)
	})

	return nil
}

// Start launches the HTTP listener on the configured port.
func (s *Server) Start() error {
	// Instantiate new HTTP request multiplexer for Cockpit REST API
	mux := http.NewServeMux()
	// Register local task backlog REST endpoint handler
	mux.HandleFunc("/api/backlog", s.handleBacklog)
	// Register system health doctor REST endpoint handler
	mux.HandleFunc("/api/health", s.handleHealth)
	// Register server status and repository info endpoint handler
	mux.HandleFunc("/api/status", s.handleStatus)
	// Register Cockpit Feature Audit Matrix endpoint handler
	mux.HandleFunc("/api/features", s.handleFeatures)
	// Register GPU Swarm Pool feature switch fallback handlers
	mux.HandleFunc("/api/swarm", s.handleSwarmDisabled)
	mux.HandleFunc("/api/swarm/active-list", s.handleSwarmDisabled)
	// Register Fleet Matrix feature switch fallback handler
	mux.HandleFunc("/api/fleet", s.handleFleetDisabled)
	// Register Dependency AST Graph feature switch fallback handler
	mux.HandleFunc("/api/graph", s.handleGraphDisabled)
	// Register Branch Audit Topology feature switch fallback handler
	mux.HandleFunc("/api/branches/audit", s.handleBranchesDisabled)
	// Register Quality Debt Matrix feature switch fallback handler
	mux.HandleFunc("/api/quality-debt", s.handleDebtDisabled)
	// Register Markdown artifact preview fallback handler
	mux.HandleFunc("/api/artifacts/", s.handleArtifactsDisabled)
	// Register workspace search fallback handler
	mux.HandleFunc("/api/search", s.handleSearchDisabled)
	// Register GitBrain Synapse feature switch fallback handler
	mux.HandleFunc("/api/gitbrain", s.handleGitBrainDisabled)
	// Register Configuration Drift feature switch fallback handler
	mux.HandleFunc("/api/drift", s.handleDriftDisabled)
	// Register Analytics & Velocity feature switch fallback handler
	mux.HandleFunc("/api/analytics", s.handleAnalyticsDisabled)

	// Register enterprise plugin middleware registration endpoint handler
	mux.HandleFunc("/api/plugin/register", s.handleRegisterPlugin)

	s.registerWorkspaceRoutes(mux)

	if err := s.registerStaticRoutes(mux); err != nil {
		return err
	}

	// Wrap request handler with logging middleware and no-cache headers for terminal activity feedback
	loggingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if !isTelemetryPollPath(r.URL.Path) {
			synapse.Info("📥 HTTP Request: %s %s\n", r.Method, r.URL.Path)
		}
		mux.ServeHTTP(w, r)
	})

	// Initialize net/http Server reference bound to configured listening port
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: loggingHandler,
	}

	// Log active server launch banner message to Synapse logger
	synapse.Info("🚀 Nomos Cockpit Daemon active at http://0.0.0.0:%d (monitoring %s)\n", s.port, s.repoRoot)
	// Listen and serve incoming HTTP client requests
	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the Cockpit HTTP server listener.
func (s *Server) Stop(ctx context.Context) error {
	// Verify if HTTP server instance is active before attempting shutdown
	if s.httpServer != nil {
		// Shutdown server gracefully with context timeout
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleBacklog handles requests to /api/backlog returning active workspace tasks.
func (s *Server) handleBacklog(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for task backlog response
	w.Header().Set("Content-Type", "application/json")
	// Verify that local task tracker is initialized
	if s.tracker == nil {
		http.Error(w, `{"error":"tracker not initialized"}`, http.StatusInternalServerError)
		return
	}

	// Check 3-second backlog memory cache to prevent thrashing disk reads on high-frequency UI polls
	s.backlogMu.RLock()
	if s.cachedBacklog != nil && time.Since(s.cachedBacklogTime) < 3*time.Second {
		cached := s.cachedBacklog
		s.backlogMu.RUnlock()
		json.NewEncoder(w).Encode(cached)
		return
	}
	s.backlogMu.RUnlock()

	// Query tasks from local tracker.
	tasks, err := s.tracker.ListAll(r.Context())
	if err != nil {
		// Fallback to active tasks list if ListAll returns an error.
		tasks, err = s.tracker.List(r.Context())
	}
	if err != nil {
		// Return internal server error if both queries fail.
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// In Community Edition, filter tasks to active repository project context.
	tasks = filterTasksForProject(tasks, filepath.Base(s.repoRoot))

	// Marshal and unmarshal tasks slice to normalize generic JSON map entries.
	var taskMaps []map[string]interface{}
	bytes, _ := json.Marshal(tasks)
	_ = json.Unmarshal(bytes, &taskMaps)

	// Enrich each task entry with required Kanban board fields.
	taskMaps = enrichTaskMaps(taskMaps)

	// Update 3-second backlog cache thread-safely inside write lock.
	s.backlogMu.Lock()
	s.cachedBacklog = taskMaps
	s.cachedBacklogTime = time.Now()
	s.backlogMu.Unlock()

	// Encode enriched task maps array into HTTP response JSON stream
	json.NewEncoder(w).Encode(taskMaps)
}

// enrichTaskMaps populates default titles and status tags for UI display.
func enrichTaskMaps(taskMaps []map[string]interface{}) []map[string]interface{} {
	for i, t := range taskMaps {
		if _, ok := t["title"]; !ok {
			if sum, ok := t["summary"]; ok {
				t["title"] = sum
			}
		}
		if _, ok := t["priority"]; !ok {
			t["priority"] = "medium"
		}
		if _, ok := t["dorStatus"]; !ok {
			t["dorStatus"] = "Ready"
		}
		if _, ok := t["dodStatus"]; !ok {
			t["dodStatus"] = "DoD: Passed"
		}
		taskMaps[i] = t
	}
	return taskMaps
}

// filterTasksForProject isolates task items matching active project root.
// This function parses the tasks slice and checks if the project matches the active workspace repo.
// In Community Edition, we filter the tasks so that we only expose and visualize the active
// repository workspace, since community is strictly single-repo locked.
// The matching process handles case-insensitive checks via strings.EqualFold.
// If the project is unspecified, we treat it as workspace-global and retain it in the slice.
// If the active project is blank or represents the root path, we return the tasks unfiltered.
func filterTasksForProject(tasks []task.Task, activeProject string) []task.Task {
	// If activeProject is blank or root paths, return all tasks unfiltered.
	if activeProject == "" || activeProject == "." || activeProject == "/" {
		return tasks
	}
	var projectTasks []task.Task
	// Loop through tasks and collect those matching active project root (case insensitive).
	for _, t := range tasks {
		if strings.EqualFold(t.Project, activeProject) || t.Project == "" {
			projectTasks = append(projectTasks, t)
		}
	}
	return projectTasks
}

// isTelemetryPollPath checks if an incoming HTTP path is a high-frequency polling endpoint.
// In Nomos Cockpit, the UI runs an active polling loop every 2 seconds to update metrics
// like statuses, swarm worker details, active drift, git changes, search backlog, and fleet.
// This results in high logging noise on the terminal which distracts autonomous agents.
// To mitigate this, this helper classifies these endpoints so we can suppress their logging output.
// The matched endpoints are evaluated using a strict string switch statement on path values.
// Returning true marks the endpoint as high-frequency telemetry, which suppresses console logs.
// Returning false ensures that critical mutation APIs and document requests are printed.
func isTelemetryPollPath(path string) bool {
	// Match incoming HTTP request path against high-frequency polling URLs.
	switch path {
	case "/api/status", "/api/backlog", "/api/swarm", "/api/swarm/active-list",
		"/api/graph", "/api/drift", "/api/search", "/api/analytics",
		"/api/gitbrain", "/api/fleet":
		// Return true to indicate the path is a polling endpoint.
		return true
	default:
		// Return false otherwise.
		return false
	}
}

// handleHealth handles requests to /api/health returning system health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for health endpoint
	w.Header().Set("Content-Type", "application/json")

	// Execute DoD health audit for active workspace repository root
	healthStatus, err := verify.AuditHealth(s.repoRoot)
	if err != nil {
		// Return failed health status payload with error message
		json.NewEncoder(w).Encode(map[string]interface{}{
			"passed":  false,
			"message": err.Error(),
			"gates":   []interface{}{},
		})
		return
	}

	// Construct health gates status list maps array
	gates := s.buildHealthGatesList(healthStatus)
	// Return evaluated health status payload with pass/fail boolean flag
	json.NewEncoder(w).Encode(map[string]interface{}{
		"passed":  len(healthStatus.Failures) == 0,
		"message": "System health status evaluated successfully.",
		"gates":   gates,
	})
}

// buildHealthGatesList constructs health gate status maps for system components.
func (s *Server) buildHealthGatesList(healthStatus verify.HealthStatus) []map[string]interface{} {
	// Evaluate Llama inference engine server status gate
	g1 := buildGateMap("Llama Inference Server (8082)", healthStatus.LlamaAlive)
	// Evaluate Cockpit web dashboard server status gate
	g2 := buildGateMap("Cockpit Observability Server (8089)", healthStatus.CockpitAlive)
	// Evaluate Git hooks integrity status gate
	g3 := buildGateMap("Git Hooks Integrity", healthStatus.GitHooksHealthy)
	// Return slice of gate result maps
	return []map[string]interface{}{g1, g2, g3}
}

// buildGateMap helper constructs individual gate result maps.
func buildGateMap(name string, alive bool) map[string]interface{} {
	// Map gate name and pass/fail boolean flag to response map
	return map[string]interface{}{
		"name":    name,
		"passed":  alive,
		"message": fmt.Sprintf("%s active: %v", name, alive),
	}
}

// handleRegisterPlugin accepts JSON-RPC plugin middleware registrations.
func (s *Server) handleRegisterPlugin(w http.ResponseWriter, r *http.Request) {
	// Enforce HTTP POST method constraint for plugin registration
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode JSON request body into pluginRegistration struct instance
	var reg pluginRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Register plugin name in thread-safe Server plugins list
	s.registerPluginName(reg.Name)
	// Log successful plugin registration event to Synapse
	synapse.Info("Registered enterprise plugin middleware: %s (%s)\n", reg.Name, reg.Endpoint)
	// Write HTTP 200 OK header status
	w.WriteHeader(http.StatusOK)
	// Encode JSON success status response
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// registerPluginName appends a plugin name inside a mutex lock.
func (s *Server) registerPluginName(name string) {
	// Acquire mutex write lock
	s.mu.Lock()
	// Defer mutex unlock on function return
	defer s.mu.Unlock()
	// Append plugin name to server plugins slice
	s.plugins = append(s.plugins, name)
}

// handleStatus returns current server status, edition tier, and repository root.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Set the Content-Type header to application/json for REST compatibility.
	w.Header().Set("Content-Type", "application/json")
	// Set Cache-Control headers to completely disable client/browser cache.
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Attempt to query the active phase state file from the local repository directory.
	pState, err := task.GetPhaseState(s.repoRoot)
	var phaseState interface{}
	// Determine the correct phase state to return
	if err == nil && pState != nil {
		phaseState = pState
	} else {
		// Construct fallback
		phaseState = GetFallbackPhaseState()
	}

	// Initialize the GPU resource utilization stub parameters.
	gpuStub := map[string]string{
		"gpuUtil":   "0%",
		"vramUsed":  "0 MB",
		"vramTotal": "0 MB",
		"powerDraw": "0 W",
		"temp":      "0 C",
	}

	// Initialize the active agent execution slots stub configuration.
	slotsStub := map[string]interface{}{
		"type":       "local",
		"limit":      1,
		"used":       0,
		"slotStates": []interface{}{},
	}

	// Encode and return the full status payload structure to the client.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"edition":        "community",
		"repoRoot":       s.repoRoot,
		"project":        filepath.Base(s.repoRoot),
		"workspaceName":  "OPEN",
		"phaseState":     phaseState,
		"gpu":            gpuStub,
		"slots":          slotsStub,
		"inferenceStats": []interface{}{},
		"version":        getCockpitVersion(),
		"buildTime":      buildTime,
	})
}

// handleSwarmDisabled provides feature switch response for GPU Swarm Pool requests.
func (s *Server) handleSwarmDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for feature switch response
	w.Header().Set("Content-Type", "application/json")
	// Return feature disabled payload with Sovereign upgrade indicator message
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active":  []interface{}{},
		"enabled": false,
		"tier":    "sovereign",
		"message": "Multi-agent GPU Swarm Pool management is available in Nomos Sovereign Edition.",
	})
}

// handleFleetDisabled provides feature switch response for Fleet Matrix requests.
func (s *Server) handleFleetDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for feature switch response
	w.Header().Set("Content-Type", "application/json")
	// Return empty fleet nodes list with Sovereign upgrade indicator message
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes":   []interface{}{},
		"enabled": false,
		"tier":    "sovereign",
		"message": "Multi-repo Fleet Matrix is available in Nomos Sovereign Edition.",
	})
}

// handleGraphDisabled returns empty dependency graph payload for community edition.
func (s *Server) handleGraphDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for graph response
	w.Header().Set("Content-Type", "application/json")
	// Return empty nodes and edges payload
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": []interface{}{},
		"edges": []interface{}{},
	})
}

// handleBranchesDisabled returns empty branch list payload for community edition.
func (s *Server) handleBranchesDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for branches response
	w.Header().Set("Content-Type", "application/json")
	// Return empty branches array
	json.NewEncoder(w).Encode(map[string]interface{}{
		"branches": []interface{}{},
	})
}

// handleDebtDisabled returns empty quality debt payload for community edition.
func (s *Server) handleDebtDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for quality debt response
	w.Header().Set("Content-Type", "application/json")
	// Return empty quality debt array
	json.NewEncoder(w).Encode(map[string]interface{}{
		"debt": []interface{}{},
	})
}

// handleArtifactsDisabled returns default Markdown content for community edition.
func (s *Server) handleArtifactsDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for artifacts response
	w.Header().Set("Content-Type", "application/json")
	// Return default Community Edition markdown artifact banner content
	json.NewEncoder(w).Encode(map[string]interface{}{
		"content": "# Nomos Community Edition\n\nSingle-repository task board & DoD health doctor active.",
	})
}

// handleSearchDisabled returns empty search results payload for community edition.
func (s *Server) handleSearchDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for search response
	w.Header().Set("Content-Type", "application/json")
	// Return empty search results array
	json.NewEncoder(w).Encode([]interface{}{})
}

// handleGitBrainDisabled returns empty GitBrain memories payload for community edition.
// It sets JSON content-type and encodes a clean disabled tier response.
func (s *Server) handleGitBrainDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for GitBrain response
	w.Header().Set("Content-Type", "application/json")
	// Return empty memories array with sovereign tier indicator
	json.NewEncoder(w).Encode(map[string]interface{}{
		"memories": []interface{}{},
		"status":   "disabled",
		"tier":     "sovereign",
	})
}

// handleDriftDisabled returns empty drift payload for community edition.
// It responds with an empty drift slice and sovereign tier metadata.
func (s *Server) handleDriftDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for drift response
	w.Header().Set("Content-Type", "application/json")
	// Return empty drift array with sovereign tier indicator
	json.NewEncoder(w).Encode(map[string]interface{}{
		"drift":  []interface{}{},
		"status": "disabled",
		"tier":   "sovereign",
	})
}

// handleAnalyticsDisabled returns empty analytics payload for community edition.
// It delivers an empty velocity dataset with sovereign tier indicator.
func (s *Server) handleAnalyticsDisabled(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for analytics response
	w.Header().Set("Content-Type", "application/json")
	// Return empty velocity array with sovereign tier indicator
	json.NewEncoder(w).Encode(map[string]interface{}{
		"velocity": []interface{}{},
		"status":   "disabled",
		"tier":     "sovereign",
	})
}

// buildCommunityFeatures compiles feature definitions for community edition modules.
func buildCommunityFeatures() []map[string]interface{} {
	// Return slice of enabled community tier features
	return []map[string]interface{}{
		{
			"id":          "kanban",
			"name":        "Kanban Task Board",
			"category":    "Core Workflow",
			"status":      "enabled",
			"tier":        "community",
			"description": "Single-repository Task Lifecycle, DoD Gates, and Status Tracking",
		},
		{
			"id":          "health_doctor",
			"name":        "System Health Doctor",
			"category":    "Observability",
			"status":      "enabled",
			"tier":        "community",
			"description": "DoD verification gatekeeper and system health diagnostics",
		},
		{
			"id":          "live_logs",
			"name":        "Live Log Viewer",
			"category":    "Observability",
			"status":      "enabled",
			"tier":        "community",
			"description": "Real-time Server-Sent Events (SSE) log tailing stream",
		},
	}
}

// buildSovereignFeatures compiles feature definitions for sovereign enterprise modules.
func buildSovereignFeatures() []map[string]interface{} {
	// Return slice of disabled sovereign tier features
	return []map[string]interface{}{
		{
			"id":          "gpu_swarm",
			"name":        "GPU Swarm Pool",
			"category":    "Fleet Autonomy",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Multi-agent GPU background worker pool orchestration",
		},
		{
			"id":          "fleet_matrix",
			"name":        "Fleet Matrix",
			"category":    "Fleet Autonomy",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Multi-repository workspace topology and cross-repo release status",
		},
		{
			"id":          "ast_graph",
			"name":        "Dependency AST Graph",
			"category":    "Architecture",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Interactive D3 AST dependency visualization and cycle detection",
		},
		{
			"id":          "branch_topology",
			"name":        "Branch Topology",
			"category":    "Version Control",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Cross-repository git branch audit and merge candidate detection",
		},
		{
			"id":          "quality_debt",
			"name":        "Quality Debt Matrix",
			"category":    "Quality Control",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Workspace debt manifest, Boy Scout compliance, and refactor queue",
		},
		{
			"id":          "gitbrain",
			"name":        "GitBrain Synapse",
			"category":    "Intelligence",
			"status":      "disabled",
			"tier":        "sovereign",
			"description": "Local vector memory, semantic search, and context synchronization",
		},
	}
}

// isFeatureEnabled evaluates if a given feature ID is active.
func isFeatureEnabled(status string) bool {
	return status == "enabled"
}

// getFeatureCategory returns default category string if empty.
func getFeatureCategory(category string) string {
	if category == "" {
		return "General"
	}
	return category
}

// getEditionTier returns the current default edition tier string.
func getEditionTier() string {
	return "community"
}

var (
	// buildVersion can be set at compile time via ldflags or dynamic fallback
	buildVersion = ""
	// buildTime stores binary compilation timestamp
	buildTime = time.Now().Format("01/02 15:04:05")
)

// getCockpitVersion returns the semantic version string of the Cockpit module.
func getCockpitVersion() string {
	if buildVersion != "" {
		return buildVersion
	}
	return "v1.1.0-dev." + time.Now().Format("0102.1504")
}

// formatISO8601Timestamp formats the current timestamp into ISO-8601 string.
func formatISO8601Timestamp() string {
	return time.Now().Format(time.RFC3339)
}

// handleFeatures returns the complete Cockpit Feature Audit Matrix.
// Details status (enabled vs disabled) and tier requirements for all features.
func (s *Server) handleFeatures(w http.ResponseWriter, r *http.Request) {
	// Set JSON Content-Type header for feature audit response
	w.Header().Set("Content-Type", "application/json")
	// Merge community and sovereign feature slices
	features := append(buildCommunityFeatures(), buildSovereignFeatures()...)

	// Encode JSON response payload containing active edition tier and features slice
	json.NewEncoder(w).Encode(map[string]interface{}{
		"edition":   getEditionTier(),
		"version":   getCockpitVersion(),
		"features":  features,
		"timestamp": formatISO8601Timestamp(),
	})
}
