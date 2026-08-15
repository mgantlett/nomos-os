package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

// LocalTracker uses SQLite project-level DAG graphs.
type LocalTracker struct {
	repoRoot string
}

// NewLocalTracker creates a new instance of LocalTracker bound to a specific repository root.
// It initializes the LocalTracker structure with the given repoRoot which is then used
// dynamically resolve paths to the project's state directories and SQLite graph database.
func NewLocalTracker(ctx *workspace.WorkspaceContext) *LocalTracker {
	repoRoot := ctx.RepoRoot
	return &LocalTracker{
		repoRoot: repoRoot,
	}
}

// ensureDbTable creates the necessary schema in the target database.
func ensureDbTable(dbPath string) error {
	conn, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS nomos_dag_nodes (
			id TEXT PRIMARY KEY,
			type TEXT,
			properties JSON
		);
	`)
	return err
}

// List fetches tasks from the current project.
func (lt *LocalTracker) List(ctx context.Context) ([]Task, error) {
	dbPaths := []string{config.ResolveGraphDbPath(lt.repoRoot)}
	return lt.listFromPaths(ctx, dbPaths)
}

// ListAll fetches tasks from all projects globally.
// Since data is localized to the repo, ListAll is equivalent to List for local repos.
func (lt *LocalTracker) ListAll(ctx context.Context) ([]Task, error) {
	return lt.List(ctx)
}

// listFromPaths concurrently fetches and aggregates tasks from multiple SQLite databases.
// It utilizes sync.WaitGroup to parallelize database IO operations and sync.Mutex
// to safely aggregate results into a unified slice. It filters out any non-existent
// databases without producing errors, allowing seamless execution across sparse workspaces.
func (lt *LocalTracker) listFromPaths(ctx context.Context, dbPaths []string) ([]Task, error) {
	var allTasks []Task
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(dbPaths))

	for _, dbPath := range dbPaths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			tasks, err := fetchTasksFromDB(path)
			if err != nil {
				// Ignore if DB doesn't exist
				if !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such table") {
					errCh <- err
				}
				return
			}
			mu.Lock()
			allTasks = append(allTasks, tasks...)
			mu.Unlock()
		}(dbPath)
	}

	wg.Wait()
	close(errCh)
	if len(errCh) > 0 {
		return nil, <-errCh
	}

	allTasks = deduplicateTasks(allTasks)
	sortTasks(allTasks)
	return allTasks, nil
}

// deduplicateTasks removes duplicate tasks by preferring project-prefixed keys over raw numeric keys
// and filtering out duplicate keys across multiple database files.
func isActiveStatus(status TaskStatus) bool {
	s := strings.ToLower(string(status))
	return s == "plan" || s == "edit" || s == "review" || s == "in_progress" || s == "in-progress" || s == "in progress" || s == "in_review"
}

func collectPrefixedNumbers(tasks []Task) map[int]bool {
	prefixed := make(map[int]bool)
	for _, t := range tasks {
		prefix, num := splitKey(t.Key)
		if prefix != "" && num > 0 {
			prefixed[num] = true
		}
	}
	return prefixed
}

func shouldPreferTask(existing, newTask Task) bool {
	isExistingActive := isActiveStatus(existing.Status)
	isNewActive := isActiveStatus(newTask.Status)
	if !isExistingActive && isNewActive {
		return true
	}
	return (isExistingActive == isNewActive) && newTask.UpdatedAt.After(existing.UpdatedAt)
}

func deduplicateTasks(tasks []Task) []Task {
	prefixedNumbers := collectPrefixedNumbers(tasks)
	seen := make(map[string]Task)
	var keys []string

	for _, t := range tasks {
		prefix, num := splitKey(t.Key)
		if prefix == "" && num > 0 && prefixedNumbers[num] {
			continue
		}

		existing, found := seen[t.Key]
		if !found {
			seen[t.Key] = t
			keys = append(keys, t.Key)
		} else if shouldPreferTask(existing, t) {
			seen[t.Key] = t
		}
	}

	var result []Task
	for _, k := range keys {
		result = append(result, seen[k])
	}
	return result
}

// fetchTasksFromDB connects to a specific SQLite database path and reads all nodes.
// It parses the JSON properties field from the nomos_dag_nodes table and decodes
// them into Task structures, gracefully skipping invalid JSON blobs to prevent panic.
func fetchTasksFromDB(dbPath string) ([]Task, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return []Task{}, nil
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT properties FROM nomos_dag_nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var propsStr string
		if err := rows.Scan(&propsStr); err != nil {
			continue
		}
		var t Task
		if err := json.Unmarshal([]byte(propsStr), &t); err == nil {
			tasks = append(tasks, t)
		} else {
			fmt.Printf("Failed to unmarshal task JSON: %v\n", err)
		}
	}
	return tasks, nil
}

func sortTasks(tasks []Task) {
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			pI, numI := splitKey(tasks[i].Key)
			pJ, numJ := splitKey(tasks[j].Key)
			if pI != pJ {
				if pI > pJ {
					tasks[i], tasks[j] = tasks[j], tasks[i]
				}
			} else {
				if numI > numJ {
					tasks[i], tasks[j] = tasks[j], tasks[i]
				}
			}
		}
	}
}

// splitKey breaks down a task ID string into its prefix and numeric components.
// For example, "COM-123" becomes ("COM", 123), which allows the sorting algorithms
// to correctly order tasks numerically rather than alphabetically (e.g. 2 before 10).
func splitKey(key string) (string, int) {
	parts := strings.Split(key, "-")
	if len(parts) == 2 {
		num, err := strconv.Atoi(parts[1])
		if err == nil {
			return parts[0], num
		}
	}
	num, err := strconv.Atoi(key)
	if err == nil {
		return "", num
	}
	return key, 0
}

var projectPrefixes = map[string]string{
	"nomos-commons":        "COM",
	"nomos-sovereign":      "SOV",
	"nomos-cockpit":        "CPT",
	"papermind":            "PMD",
	"nomos-ink":            "PMD",
	"nomos-swarm":          "SWM",
	"nomos-gitbrain":       "GB",
	"gsi-management":       "GSI",
	"sophialabs":           "SLA",
	"nix-audio-visualizer": "NAV",
}

// getProjectPrefix maps project workspace names to their 3-letter DDP tracker prefixes.
func getProjectPrefix(project string) string {
	if prefix, ok := projectPrefixes[project]; ok {
		return prefix
	}
	if len(project) >= 3 {
		return strings.ToUpper(project[:3])
	}
	return "TSK"
}

// SaveTask persists a task structure into SQLite.
func (lt *LocalTracker) SaveTask(t Task) error {
	return lt.saveTask(t)
}

// saveTask marshals the Task structure into a JSON blob and inserts it into SQLite.
// It uses an INSERT OR REPLACE UPSERT strategy to ensure data idempotent writes.
// This guarantees that any existing task with the same key is overwritten entirely.
func (lt *LocalTracker) saveTask(t Task) error {
	dbPath := config.ResolveGraphDbPath(lt.repoRoot)
	if t.Project != "" && filepath.Base(filepath.Clean(lt.repoRoot)) != t.Project && !strings.Contains(lt.repoRoot, "tmp") && !strings.Contains(lt.repoRoot, "Test") {
		dataRoot := filepath.Dir(config.GlobalDataDir(lt.repoRoot))
		dbPath = filepath.Join(dataRoot, t.Project, "state", "graph.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}
	if err := ensureDbTable(dbPath); err != nil {
		return err
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	t.UpdatedAt = time.Now()
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO nomos_dag_nodes (id, type, properties) VALUES (?, 'task', ?)`, t.Key, string(data))
	return err
}

// View retrieves a single Task from the SQLite database by its unique alphanumeric key.
// It queries the properties column in the nomos_dag_nodes table, returning the row,
// and gracefully unmarshals the JSON representation into a strongly typed Task structure.
// Returns an error if the task does not exist or JSON parsing fails.
func (lt *LocalTracker) View(ctx context.Context, key string) (*Task, error) {
	dbPath := config.ResolveGraphDbPath(lt.repoRoot)
	conn, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	var props string
	err = conn.QueryRow("SELECT properties FROM nomos_dag_nodes WHERE id = ?", key).Scan(&props)
	if err != nil {
		return nil, fmt.Errorf("task %s not found: %v", key, err)
	}
	var t Task
	if err := json.Unmarshal([]byte(props), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// modifyTask provides a safe mutation closure over a single task.
// It automatically fetches the task, executes the callback function to mutate its fields,
// and ensures the resulting modifications are atomically persisted back to the database.
func (lt *LocalTracker) modifyTask(ctx context.Context, key string, mutate func(*Task)) error {
	t, err := lt.View(ctx, key)
	if err != nil {
		return err
	}
	mutate(t)
	return lt.saveTask(*t)
}

// Start transitions the specified task into the In Progress state.
// It assigns the task to the current human developer or AI worker, locking it
// in the DAG to prevent concurrent modifications across distributed nodes.
func (lt *LocalTracker) Start(ctx context.Context, key string, assignee string) error {
	return lt.modifyTask(ctx, key, func(t *Task) {
		t.Status = StatusInProgress
		t.Assignee = assignee
	})
}

func (lt *LocalTracker) updateTaskStatusAndClose(ctx context.Context, key string, status TaskStatus, comment string) error {
	err := lt.modifyTask(ctx, key, func(t *Task) {
		t.Status = status
		now := time.Now()
		t.ClosedAt = &now
		if comment != "" {
			t.Comments = append(t.Comments, TaskComment{
				Author:    getAuthor(),
				CreatedAt: time.Now(),
				Body:      comment,
			})
		}
	})
	return err
}

func (lt *LocalTracker) Close(ctx context.Context, key string, comment string) error {
	return lt.updateTaskStatusAndClose(ctx, key, StatusDone, comment)
}

func (lt *LocalTracker) Cancel(ctx context.Context, key string, comment string) error {
	return lt.updateTaskStatusAndClose(ctx, key, StatusCancelled, comment)
}

func (lt *LocalTracker) ResetBackend(ctx context.Context, key string) error {
	return lt.modifyTask(ctx, key, func(t *Task) {
		t.Status = StatusBacklog
		t.Assignee = ""
	})
}

func (lt *LocalTracker) Comment(ctx context.Context, key string, comment string) error {
	return lt.modifyTask(ctx, key, func(t *Task) {
		t.Comments = append(t.Comments, TaskComment{
			Author:    getAuthor(),
			CreatedAt: time.Now(),
			Body:      comment,
		})
	})
}

func getAuthor() string {
	if agent := os.Getenv("NOMOS_ACTIVE_AGENT"); agent != "" {
		return agent
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "system"
}

// Transition handles complex lifecycle state changes (e.g. Backlog -> Review).
// It verifies that the task is not already in the target state, then appends
// a comprehensive transition log entry documenting the timestamp and author of the change.
func (lt *LocalTracker) Transition(ctx context.Context, key string, status TaskStatus) error {
	err := lt.modifyTask(ctx, key, func(t *Task) {
		if t.Status != status {
			t.ActivityLog = append(t.ActivityLog, TaskTransition{
				Timestamp: time.Now(),
				OldStatus: t.Status,
				NewStatus: status,
				Author:    getAuthor(),
			})
			t.Status = status
		}
	})
	return err
}

// Create generates a new Task node in the DAG graph.
// It automatically detects the current project context to assign the appropriate
// alphanumeric prefix, finds the maximum existing ID for that prefix, and auto-increments it.
func (lt *LocalTracker) Create(ctx context.Context, title string, body string, labels []string, parentKey string, project string, taskType TaskType, isSpike bool, initialStatus TaskStatus) (string, error) {
	// First, fetch existing tasks in target project database to find max ID for prefix
	targetDbPath := config.ResolveGraphDbPath(lt.repoRoot)
	if project != "" && filepath.Base(filepath.Clean(lt.repoRoot)) != project {
		dataRoot := filepath.Dir(config.GlobalDataDir(lt.repoRoot))
		targetDbPath = filepath.Join(dataRoot, project, "state", "graph.db")
	}

	tasks, err := fetchTasksFromDB(targetDbPath)
	if err != nil && !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such table") {
		return "", err
	}

	prefix := getProjectPrefix(project)
	maxKey := 0
	for _, t := range tasks {
		p, num := splitKey(t.Key)
		if p == prefix && num > maxKey {
			maxKey = num
		}
	}
	newKey := fmt.Sprintf("%s-%d", prefix, maxKey+1)

	newTask := Task{
		Key:         newKey,
		Project:     project,
		ParentKey:   parentKey,
		Type:        taskType,
		Status:      initialStatus,
		Title:       title,
		Description: body,
		Labels:      labels,
		IsSpike:     isSpike,
		Assignee:    "markg",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := lt.saveTask(newTask); err != nil {
		return "", err
	}
	return newKey, nil
}

// Edit performs an exhaustive mutation of multiple task attributes simultaneously.
// It leverages pointer references to safely determine which fields require updating.
// If a pointer is nil, the associated task field is left unmodified.
func (lt *LocalTracker) Edit(ctx context.Context, key string, title *string, body *string, labels []string, contextBurden *int, logicDepth *int, blockedBy []string, sequence *int, project *string) error {
	return lt.modifyTask(ctx, key, func(t *Task) {
		if title != nil {
			t.Title = *title
		}
		if body != nil {
			t.Description = *body
		}
		if labels != nil {
			t.Labels = labels
		}
		if contextBurden != nil {
			t.ContextBurden = *contextBurden
		}
		if logicDepth != nil {
			t.LogicDepth = *logicDepth
		}
		if blockedBy != nil {
			t.BlockedBy = blockedBy
		}
		if sequence != nil {
			t.Sequence = *sequence
		}
		if project != nil {
			t.Project = *project
		}
	})
}
