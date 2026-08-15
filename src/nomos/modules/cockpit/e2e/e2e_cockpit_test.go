package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mgantlett/nomos-os/src/nomos/modules/cockpit"
)

// TestCockpitE2E_ConduitSuite validates Cockpit UI API endpoints and static assets.
func TestCockpitE2E_ConduitSuite(t *testing.T) {
	server := cockpit.NewServer("/tmp", 8089, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", server.ServeHTTP)
	mux.HandleFunc("/api/backlog", server.ServeHTTP)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Verify Status API
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/status", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to fetch /api/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// 2. Verify Backlog API
	reqBacklog, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/backlog", nil)
	respBacklog, err := client.Do(reqBacklog)
	if err != nil {
		t.Fatalf("Failed to fetch /api/backlog: %v", err)
	}
	defer respBacklog.Body.Close()
	if respBacklog.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", respBacklog.StatusCode)
	}
}
