package cockpit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCockpitServerEndpoints(t *testing.T) {
	server := NewServer(t.TempDir(), 8089, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", server.handleHealth)
	mux.HandleFunc("/api/status", server.handleStatus)
	mux.HandleFunc("/api/features", server.handleFeatures)
	mux.HandleFunc("/api/swarm", server.handleSwarmDisabled)
	mux.HandleFunc("/api/fleet", server.handleFleetDisabled)
	mux.HandleFunc("/api/graph", server.handleGraphDisabled)
	mux.HandleFunc("/api/branches/audit", server.handleBranchesDisabled)
	mux.HandleFunc("/api/quality-debt", server.handleDebtDisabled)
	mux.HandleFunc("/api/artifacts/", server.handleArtifactsDisabled)
	mux.HandleFunc("/api/search", server.handleSearchDisabled)
	mux.HandleFunc("/api/gitbrain", server.handleGitBrainDisabled)
	mux.HandleFunc("/api/drift", server.handleDriftDisabled)
	mux.HandleFunc("/api/analytics", server.handleAnalyticsDisabled)
	mux.HandleFunc("/api/plugin/register", server.handleRegisterPlugin)

	routesToTest := []string{
		"/api/health",
		"/api/status",
		"/api/features",
		"/api/swarm",
		"/api/fleet",
		"/api/graph",
		"/api/branches/audit",
		"/api/quality-debt",
		"/api/artifacts/implementation_plan",
		"/api/search",
		"/api/gitbrain",
		"/api/drift",
		"/api/analytics",
	}

	for _, route := range routesToTest {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200 for endpoint %s, got %d", route, w.Code)
		}
	}

	// Test /api/plugin/register
	payload := `{"name":"test-plugin","endpoint":"/api/test","target_url":"http://localhost:9999"}`
	reqPlugin := httptest.NewRequest(http.MethodPost, "/api/plugin/register", strings.NewReader(payload))
	wPlugin := httptest.NewRecorder()
	mux.ServeHTTP(wPlugin, reqPlugin)

	if wPlugin.Code != http.StatusOK {
		t.Fatalf("expected status 200 for plugin registration, got %d", wPlugin.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(wPlugin.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "registered" {
		t.Fatalf("expected status 'registered', got %s", resp["status"])
	}
}
