package task

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/go-playground/validator/v10"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// PhaseState tracks current agent phase and validation states.
// This structure holds essential metadata required to govern agent workflow,
// including approval gates, phase transitions, and lifecycle timestamps.
// The PhaseToken field is uniquely generated upon entering the EDIT phase
// and is required by the LD_PRELOAD substrate lock to authorize file mutations.
// PhaseState represents the persistent state of the workspace.
// This forms the core source of truth for the autonomous state machine and controls
// the security boundaries for write-access across the entire workspace via the phase_token.
// It maps directly to the active phase of the sprint or execution context, ensuring
// that automated Tier 1 and Tier 2 agents respect human-in-the-loop approvals.
// This structure must not be manually modified; all state transitions should be done
// via the CLI 'nomos task transition' and 'nomos task approve' commands to maintain
// the Data Integrity Gate cryptographic hashes.
type PhaseState struct {
	Agent                   string               `json:"agent" validate:"omitempty"`
	AgentTier               state.AgentTier      `json:"agent_tier" validate:"omitempty,oneof=high low"`
	AgentType               string               `json:"agent_type" validate:"omitempty"`
	CommitApproved          string               `json:"commit_approved" validate:"required,oneof=true false"`
	CurrentPhase            state.WorkspacePhase `json:"current_phase" validate:"required,oneof=PLAN EDIT REVIEW IDLE"`
	DodFailureCount         int                  `json:"dod_failure_count" validate:"gte=0"`
	PhaseEnteredAt          string               `json:"phase_entered_at" validate:"required,datetime=2006-01-02T15:04:05Z07:00"`
	PlanApproved            string               `json:"plan_approved" validate:"required,oneof=true false"`
	PrevPhase               state.WorkspacePhase `json:"prev_phase" validate:"omitempty,oneof=PLAN EDIT REVIEW IDLE"`
	SessionCommits          int                  `json:"session_commits" validate:"gte=0"`
	SessionStartedAt        string               `json:"session_started_at" validate:"required,datetime=2006-01-02T15:04:05Z07:00"`
	TaskId                  string               `json:"task_id" validate:"omitempty"`
	WaitingOnHuman          string               `json:"waiting_on_human" validate:"required,oneof=true false"`
	TasksCompletedInSession int                  `json:"tasks_completed_in_session" validate:"gte=0"`
	CompactContext          bool                 `json:"compact_context" validate:"omitempty"`
	PhaseToken              string               `json:"phase_token" validate:"omitempty"`
}

// GetPhaseState reads, validates, and returns the current PhaseState of the workspace.
// The file is parsed from `.phase_state.json` inside the `.nomos/data/<project>/state` directory.
// This forms the core source of truth for the autonomous state machine and controls
// the security boundaries for write-access across the entire workspace via the phase_token.
func GetPhaseState(ctx *workspace.WorkspaceContext) (*PhaseState, error) {
	repoRoot := ctx.RepoRoot
	phaseStatePath := config.PhaseStatePath(repoRoot)
	data, err := os.ReadFile(phaseStatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read phase state file: %w", err)
	}
	var state PhaseState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse phase state: %w", err)
	}

	// Validate phase state invariants using go-playground validator
	// We mandate strict schema compliance (e.g. valid ISO8601 timestamps, enum enforcement
	// on agent tiers, true/false strictness) to prevent silent orchestration drift.
	validate := validator.New()
	if err := validate.Struct(&state); err != nil {
		return nil, fmt.Errorf("phase state validation failed: %w", err)
	}

	return &state, nil
}

// TransitionPhase changes the workspace phase and executes phase change hooks
func TransitionPhase(ctx *workspace.WorkspaceContext, nextPhase state.WorkspacePhase) error {
	repoRoot := ctx.RepoRoot
	phaseStatePath := config.PhaseStatePath(repoRoot)

	var state PhaseState
	if data, err := os.ReadFile(phaseStatePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}

	taskIdToPost := updateStateForPhase(ctx, &state, nextPhase)

	if err := persistPhaseState(ctx, phaseStatePath, &state); err != nil {
		return err
	}

	enforceSubstrateLock(repoRoot, nextPhase)

	runPhaseTransitionSideEffects(ctx, state, taskIdToPost, nextPhase)
	return nil
}

func persistPhaseState(ctx *workspace.WorkspaceContext, phaseStatePath string, state *PhaseState) error {
	repoRoot := ctx.RepoRoot
	phaseBytes, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal phase state: %w", err)
	}

	if err := os.MkdirAll(config.StateDir(repoRoot), 0755); err != nil {
		return fmt.Errorf("failed to create agent state dir: %w", err)
	}

	if err := savePhaseStateJSON(phaseStatePath, phaseBytes); err != nil {
		return fmt.Errorf("failed to write phase state: %w", err)
	}

	hash := CalculatePhaseStateHash(phaseBytes)
	if err := PersistPhaseStateHash(ctx, hash); err != nil {
		return fmt.Errorf("failed to persist phase state signature: %w", err)
	}
	_ = UpdateWorkspaceStateHash(ctx)
	return nil
}

func enforceSubstrateLock(repoRoot string, nextPhase state.WorkspacePhase) {
	if nextPhase == state.PhaseEdit {
		_ = nomosexec.UnlockSubstrate(repoRoot)
	} else {
		_ = nomosexec.LockSubstrate(repoRoot)
	}

	if nextPhase == state.PhaseIdle {
		_ = os.Remove(filepath.Join(config.TmpDir(repoRoot), "task.md"))
	}
}

// savePhaseStateJSON modifies state file permission flags and writes the JSON document.
// This function ensures the file is temporarily writable before locking it down to 0440.
func savePhaseStateJSON(phaseStatePath string, phaseBytes []byte) error {
	// If the state file already exists, open permissions to writable
	if _, err := os.Stat(phaseStatePath); err == nil {
		_ = os.Chmod(phaseStatePath, 0600)
	}
	// Write the byte stream directly to the target state file location
	if err := os.WriteFile(phaseStatePath, phaseBytes, 0440); err != nil {
		return err
	}
	// Seal the state file to read-only for owner/group permissions (0440)
	_ = os.Chmod(phaseStatePath, 0440)
	return nil
}

// runPhaseTransitionSideEffects emits lifecycle events, triggers script hooks, and posts comment notifications.
// This centralizes post-transition activities without bloating the main transition logic.
func runPhaseTransitionSideEffects(ctx *workspace.WorkspaceContext, state PhaseState, taskIdToPost string, nextPhase state.WorkspacePhase) {
	repoRoot := ctx.RepoRoot
	// Telemetry: record event tracking the workspace transition phase swap
	_ = telemetry.EmitEvent(repoRoot, "phase_transition", string(nextPhase))
	// Execute hooks from templates directory
	TriggerPhaseHooks(filepath.Join(repoRoot, ".agent", "templates", "hooks", "phase"), state.TaskId, nextPhase)
	// Execute hooks from user overrides directory
	TriggerPhaseHooks(filepath.Join(repoRoot, ".agent", "hooks", "phase"), state.TaskId, nextPhase)
	// Post lifecycle update messages to comments if configured
	if taskIdToPost != "" {
		PostPhaseComment(ctx, taskIdToPost, nextPhase)
	}
}

type phaseTransitionFn func(repoRoot string, state *PhaseState) string

var phaseHandlers = map[state.WorkspacePhase]phaseTransitionFn{
	state.PhasePlan: func(repoRoot string, state *PhaseState) string {
		state.PlanApproved = "false"
		state.CommitApproved = "false"
		state.WaitingOnHuman = "true"
		return ""
	},
	// This cryptographic token ensures that the workspace state cannot be tampered with.
	// It is authenticated via HMAC-SHA256 and checked during commits to enforce the cognitive firewall.
	state.PhaseEdit: func(repoRoot string, state *PhaseState) string {
		state.PlanApproved = "true"
		state.CommitApproved = "false"

		head := ""
		// Capture the current HEAD commit hash to embed in the phase token.
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = repoRoot
		if out, err := cmd.Output(); err == nil {
			head = strings.TrimSpace(string(out))
		}

		// The payload contains the task ID and the baseline head hash.
		payload := map[string]string{
			"task_id": state.TaskId,
			"head":    head,
		}
		b, _ := json.Marshal(payload)
		b64Payload := base64.StdEncoding.EncodeToString(b)

		// Load the HMAC secret to sign the token payload.
		dbPath := config.ResolveStateDbPath(repoRoot)
		secret, _ := db.GetOrCreatePhaseSecret(dbPath)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(b)
		sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		state.PhaseToken = b64Payload + "." + sig
		return ""
	},
	state.PhaseReview: func(repoRoot string, st *PhaseState) string {
		st.PlanApproved = "true"
		st.CommitApproved = "false"
		st.WaitingOnHuman = "true"
		st.PhaseToken = ""
		// If we are transitioning to REVIEW from an active task (EDIT), it counts as a task completion workflow step
		if st.CurrentPhase == state.PhaseEdit || st.CurrentPhase == state.PhasePlan {
			st.TasksCompletedInSession++
		}
		return st.TaskId
	},
	state.PhaseIdle: func(repoRoot string, st *PhaseState) string {
		st.PlanApproved = "false"
		st.CommitApproved = "false"
		st.PhaseToken = ""
		// If transitioning directly to IDLE from an active task (e.g., fast-tracked closing), increment completion count
		if st.CurrentPhase == state.PhaseEdit || st.CurrentPhase == state.PhasePlan || st.CurrentPhase == state.PhaseReview {
			st.TasksCompletedInSession++
		}
		st.TaskId = ""
		return ""
	},
}

// updateStateForPhase uses a registry pattern to apply phase transitions
// dynamically, eliminating cyclomatic complexity from nested control flows.
func updateStateForPhase(ctx *workspace.WorkspaceContext, state *PhaseState, nextPhase state.WorkspacePhase) string {
	repoRoot := ctx.RepoRoot
	state.PrevPhase = state.CurrentPhase
	state.CurrentPhase = nextPhase
	state.PhaseEnteredAt = time.Now().Format(time.RFC3339)
	state.WaitingOnHuman = "false"

	var taskIdToPost string
	if handler, exists := phaseHandlers[nextPhase]; exists {
		taskIdToPost = handler(repoRoot, state)
	}
	return taskIdToPost
}

// executePhaseHook runs a single script hook if it matches the on_*.sh naming pattern.
func executePhaseHook(hooksDir string, entry os.DirEntry, taskId string, phase state.WorkspacePhase) {
	// Skip directories and non-hook files
	if entry.IsDir() || !strings.HasPrefix(entry.Name(), "on_") {
		return
	}
	hookPath := filepath.Join(hooksDir, entry.Name())
	cmd := exec.Command(hookPath)
	// Set command working directory to the top-level repository root
	cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(hooksDir)))
	// Inject active task ID and current phase environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NOMOS_ACTIVE_TASK=%s", taskId),
		fmt.Sprintf("NOMOS_CURRENT_PHASE=%s", phase),
	)
	_ = cmd.Run()
}

// TriggerPhaseHooks executes hooks matching on_*.sh or binaries under hooksDir.
// It iterates through the target directory and invokes each hook with environment variables.
func TriggerPhaseHooks(hooksDir string, taskId string, phase state.WorkspacePhase) {
	// Verify target hooks directory exists on disk
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		return
	}

	// Read directory entries from target hooks folder
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return
	}

	// Iterate through entries and execute matching phase hooks
	for _, entry := range entries {
		executePhaseHook(hooksDir, entry, taskId, phase)
	}
}

// CalculatePhaseStateHash computes the SHA-256 cryptographic signature of the phase state byte slice.
// This signature ensures that the phase state has not been manually altered outside of the program.
func CalculatePhaseStateHash(data []byte) string {
	// Initialize a new SHA-256 hashing context
	h := sha256.New()
	// Write the raw state file bytes into the hashing algorithm
	h.Write(data)
	// Return the final hash encoded as a hexadecimal string
	return hex.EncodeToString(h.Sum(nil))
}

// PersistPhaseStateHash records the phase signature hash to a flat file.
// This registry allows pre-commit gates to verify the integrity of the active state file.
func PersistPhaseStateHash(ctx *workspace.WorkspaceContext, hash string) error {
	repoRoot := ctx.RepoRoot
	hashPath := filepath.Join(config.TmpDir(repoRoot), ".phase_hash.txt")
	dir := filepath.Dir(hashPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create tmp directory: %w", err)
	}
	err := os.WriteFile(hashPath, []byte(hash), 0644)
	if err != nil {
		return fmt.Errorf("failed to write phase state hash to file: %w", err)
	}
	return nil
}

// GetPersistedPhaseStateHash reads the registered phase signature hash from a flat file.
// It retrieves the hash for verification against the current filesystem state.
func GetPersistedPhaseStateHash(ctx *workspace.WorkspaceContext) (string, error) {
	repoRoot := ctx.RepoRoot
	hashPath := filepath.Join(config.TmpDir(repoRoot), ".phase_hash.txt")
	if _, err := os.Stat(hashPath); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(hashPath)
	if err != nil {
		return "", fmt.Errorf("failed to read phase state hash from file: %w", err)
	}
	return string(content), nil
}

// ValidatePhaseToken parses and verifies the cryptographic signature of the PhaseToken.
func ValidatePhaseToken(ctx *workspace.WorkspaceContext, pState *PhaseState) error {
	if pState.CurrentPhase != state.PhaseEdit {
		return fmt.Errorf("workspace is not in EDIT phase (current: %s)", pState.CurrentPhase)
	}
	if pState.PhaseToken == "" {
		return fmt.Errorf("missing phase token")
	}

	parts := strings.Split(pState.PhaseToken, ".")
	if len(parts) != 2 {
		return fmt.Errorf("malformed phase token format")
	}

	return verifyTokenSignature(ctx, parts[0], parts[1], pState.TaskId)
}

func verifyTokenSignature(ctx *workspace.WorkspaceContext, b64Payload, sig, expectedTaskId string) error {
	repoRoot := ctx.RepoRoot
	b, err := base64.StdEncoding.DecodeString(b64Payload)
	if err != nil {
		return fmt.Errorf("failed to decode phase token payload: %w", err)
	}

	dbPath := config.ResolveStateDbPath(repoRoot)
	secret, err := db.GetOrCreatePhaseSecret(dbPath)
	if err != nil {
		return fmt.Errorf("failed to retrieve phase secret: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(b)
	expectedSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if sig != expectedSig {
		return fmt.Errorf("phase token signature verification failed (forged or tampered)")
	}

	var payload map[string]string
	if err := json.Unmarshal(b, &payload); err != nil {
		return fmt.Errorf("invalid phase token payload format")
	}

	if payload["task_id"] != expectedTaskId {
		return fmt.Errorf("phase token task ID mismatch (expected %s, got %s)", expectedTaskId, payload["task_id"])
	}

	return nil
}

// FindActivePhaseStateAcrossProjects scans all project directories under ~/Projects
// to locate any active (non-IDLE) task phase state across all projects in the workspace.
func FindActivePhaseStateAcrossProjects() *PhaseState {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	projectsDir := filepath.Join(home, "Projects")

	var activeState *PhaseState
	_ = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "node_modules" || name == ".git" || name == "archive" {
			return filepath.SkipDir
		}

		if name == ".nomos" {
			repoRoot := filepath.Dir(path)
			ctx, _ := workspace.NewContext(repoRoot)
			if st := checkActivePhaseStateInRepo(ctx); st != nil {
				activeState = st
				return filepath.SkipAll
			}
			return filepath.SkipDir
		}
		return nil
	})
	return activeState
}

// checkActivePhaseStateInRepo checks a single repo for an active phase state.
func checkActivePhaseStateInRepo(ctx *workspace.WorkspaceContext) *PhaseState {
	if st, err := GetPhaseState(ctx); err == nil {
		if st.TaskId != "" && st.CurrentPhase != state.PhaseIdle {
			return st
		}
	}
	return nil
}
