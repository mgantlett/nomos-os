package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command which allows the user to quickly view
// the current phase state and active task in the current workspace without manually
// parsing the json files.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current workspace phase and active task status",
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, root, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		// 1. Get Active Phase
		// We read the SSoT phase state document to determine if the workspace
		// is in an EDIT, REVIEW, or IDLE phase. Defaults to IDLE on error.
		// This state file dictates what actions are permitted on the repository.
		phase := getActivePhase(root)

		// 2. Get Substrate State
		substrate := "Unlocked (Writable)"
		if phase == string(statepkg.PhaseReview) || phase == string(statepkg.PhaseIdle) {
			substrate = "Locked (Read-Only)"
		}

		// 3. Get Active Task
		// If a task is active, its ID is stored here. We load it and then
		// fetch the rich Task object from the tracker to display the title.
		// If the task cannot be loaded, we display its ID instead.
		taskTitle := getActiveTaskTitle(root, tracker)

		synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
		synapse.Info("📦 Workspace Phase : %s\n", phase)
		synapse.Info("🔒 Substrate State : %s\n", substrate)
		synapse.Info("📋 Active Task     : %s\n", taskTitle)
		synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))

		return nil
	},
}

// getActivePhase reads the current phase state.
// It returns IDLE if the file cannot be read or parsed.
func getActivePhase(root string) string {
	phase := string(statepkg.PhaseIdle)
	phasePath := config.PhaseStatePath(root)
	if data, err := os.ReadFile(phasePath); err == nil {
		var stateData struct {
			CurrentPhase statepkg.WorkspacePhase `json:"current_phase"`
		}
		if err := json.Unmarshal(data, &stateData); err == nil {
			phase = string(stateData.CurrentPhase)
		}
	}
	return phase
}

// getActiveTaskTitle fetches the title of the active task.
// If the task cannot be loaded, it returns the task ID instead.
func getActiveTaskTitle(root string, tracker task.Tracker) string {
	taskTitle := "None"
	taskIdPath := config.StateTaskIdPath(root)
	if idData, err := os.ReadFile(taskIdPath); err == nil {
		taskId := strings.TrimSpace(string(idData))
		if taskId != "" {
			t, err := tracker.View(context.Background(), taskId)
			if err == nil && t != nil {
				taskTitle = fmt.Sprintf("%s - %s", t.Key, t.Title)
			} else {
				taskTitle = taskId
			}
		}
	}
	return taskTitle
}

func init() {
	taskCmd.AddCommand(statusCmd)
}
