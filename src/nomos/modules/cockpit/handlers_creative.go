package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/llm"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

type DiscoveryRequest struct {
	Vision string `json:"vision"`
}

type DiscoverySchema struct {
	Questions []string `json:"questions"`
}

func (s *Server) handleCreativeDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	client := llm.NewClient()
	messages := []llm.Message{
		{Role: "system", Content: "You are a senior Product Manager. Given a product vision, return 3-5 hard-hitting edge-case discovery questions to flesh out constraints, scale, and requirements."},
		{Role: "user", Content: req.Vision},
	}

	outStr, err := client.ChatStructured("gpt-4o", messages, "discovery_questions", DiscoverySchema{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(outStr))
}

type ArchitectRequest struct {
	Vision  string `json:"vision"`
	Answers string `json:"answers"` // Formatted Q&A string
}

type TaskDef struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"` // Names/Titles of tasks that block this one
}

type ArchitectSchema struct {
	Blueprint string    `json:"blueprint"`
	Tasks     []TaskDef `json:"tasks"`
}

func (s *Server) handleCreativeArchitect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ArchitectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	client := llm.NewClient()
	prompt := fmt.Sprintf("Vision:\n%s\n\nQ&A:\n%s", req.Vision, req.Answers)
	messages := []llm.Message{
		{Role: "system", Content: "You are a Solution Architect and Engineering Lead. Produce a comprehensive Architectural Blueprint (Markdown format). Then, break down the execution into a Directed Acyclic Graph (DAG) of atomic tasks, specifying dependencies using task titles."},
		{Role: "user", Content: prompt},
	}

	outStr, err := client.ChatStructured("gpt-4o", messages, "architectural_dag", ArchitectSchema{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(outStr))
}

type CommitRequest struct {
	Vision    string    `json:"vision"`
	Blueprint string    `json:"blueprint"`
	Tasks     []TaskDef `json:"tasks"`
}

func (s *Server) handleCreativeCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.tracker == nil {
		http.Error(w, "Task tracker not initialized", http.StatusInternalServerError)
		return
	}

	// Save Blueprint
	specDir := filepath.Join(s.repoRoot, ".nomos", "data", "specs")
	os.MkdirAll(specDir, 0755)
	
	// Create a safe filename from vision (simplified)
	safeName := "blueprint.md"
	if len(req.Vision) > 0 {
		safeName = "blueprint.md" // Kept simple for now
	}
	
	os.WriteFile(filepath.Join(specDir, safeName), []byte(req.Blueprint), 0644)

	ctx := context.Background()
	// 1. Create Cycle
	cycleKey, err := s.tracker.Create(ctx, "Cycle: "+req.Vision, "Auto-generated overarching cycle.", []string{"epic"}, "", "", task.TypeBatch, false, task.StatusBacklog)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Create Tasks and map titles to keys for dependencies
	titleToKey := make(map[string]string)
	var createdTaskKeys []string

	for _, t := range req.Tasks {
		key, err := s.tracker.Create(ctx, t.Title, t.Description, []string{"creative"}, cycleKey, "", task.TypeTask, false, task.StatusBacklog)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		titleToKey[t.Title] = key
		createdTaskKeys = append(createdTaskKeys, key)
	}

	// 3. Update BlockedBy
	for i, t := range req.Tasks {
		var blockedBy []string
		for _, dep := range t.Dependencies {
			if key, ok := titleToKey[dep]; ok {
				blockedBy = append(blockedBy, key)
			}
		}
		if len(blockedBy) > 0 {
			err := s.tracker.Edit(ctx, createdTaskKeys[i], nil, nil, nil, nil, nil, blockedBy, nil, nil)
			if err != nil {
				fmt.Printf("Failed to update BlockedBy for %s: %v\n", createdTaskKeys[i], err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}
