/*
Package cmd provides the CLI commands for the Nomos orchestrator.
This file specifically houses the 'task create' command, which is responsible for ingesting
new user tasks, parsing metadata like CLI blast radius, priority, and task type, and
then using the underlying local tracker to persist the ticket.
It plays a critical role in enforcing the initial rigor of incoming work.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskCreateCmd initiates ticket creation requests to the active tracker backend.
// It reads optional markdown body payloads, validates schema headers, and tags appropriate sprint tags.
var taskCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create a new backlog task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Extract task title from positional arguments
		title := args[0]
		// Retrieve flag options for markdown file path, burden points, depth estimation, and labels
		fileVal, _ := cmd.Flags().GetString("file")
		schemaVal, _ := cmd.Flags().GetString("schema")
		burdenVal, _ := cmd.Flags().GetInt("burden")
		depthVal, _ := cmd.Flags().GetInt("depth")

		// Warn if total estimated complexity exceeds micro-tasking threshold (5 story points)
		if burdenVal+depthVal > 5 {
			fmt.Printf("⚠️  [Micro-Task Warning] Estimated complexity (%d size) exceeds recommended threshold (<= 5 size). Consider splitting for rapid verification cycles.\n", burdenVal+depthVal)
		}

		labelVal, _ := cmd.Flags().GetString("label")

		// Load task tracker instance and active workspace root directory
		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		// Enforce Tier 2 agent restrictions: sub-agents cannot create new backlog tasks
		if pState, err := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); err == nil && pState.AgentTier == statepkg.Tier2 {
			return fmt.Errorf("Tier 2 atomic rigidity violation: agents are explicitly forbidden from creating new tasks")
		}

		// Read markdown body payload if description file option was passed
		body := ""
		if fileVal != "" {
			data, err := os.ReadFile(filepath.Join(repoRoot, fileVal))
			if err != nil {
				data, err = os.ReadFile(fileVal)
				if err != nil {
					return fmt.Errorf("failed to read description file: %w", err)
				}
			}
			body = string(data)
		}

		// Validate markdown file content against schema definitions.
		// The task schema parser verifies that mandatory sections like Execution Unit,
		// Acceptance Criteria, Technical Notes, and Rigor Boundary are populated.
		parsedSchema, err := schema.ParseTaskSchema(body, schemaVal)

		var allErrors []string

		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("task creation rejected: markdown file fails schema parsing: %v", err))
		} else {
			// Execute strict validation on parsed task schema structure
			if err := parsedSchema.Validate(schemaVal); err != nil {
				allErrors = append(allErrors, err.Error())
			}
		}

		// Parse comma-separated label string into slice of label strings
		var labels []string
		if labelVal != "" {
			parts := strings.Split(labelVal, ",")
			for _, p := range parts {
				labels = append(labels, strings.TrimSpace(p))
			}
		}

		// Validate and extract relative priority label from the command line flags or description body.
		// Valid priority tags are priority:critical, priority:high, priority:medium, or priority:low.
		hasPriority := false
		for _, l := range labels {
			if l == "priority:critical" || l == "priority:high" || l == "priority:medium" || l == "priority:low" {
				hasPriority = true
				break
			}
		}

		// Extract priority tag from markdown body regex match if missing from flags
		if !hasPriority && body != "" {
			rxPriority := regexp.MustCompile(`(?i)priority:(critical|high|medium|low)`)
			if match := rxPriority.FindStringSubmatch(body); len(match) > 1 {
				prio := "priority:" + strings.ToLower(match[1])
				labels = append(labels, prio)
				hasPriority = true
			}
		}

		if !hasPriority {
			allErrors = append(allErrors, "priority label (priority:critical, priority:high, priority:medium, or priority:low) is missing.\n\nPlease run the /groom workflow to analyze relative priority compared to the existing backlog and append the tag.")
		}

		// Check if CLI coupling or Blast radius labels are explicitly provided.
		// These labels inform the workflow of potential cross-boundary risk.
		hasCLI := false
		hasBlast := false
		for _, l := range labels {
			if strings.HasPrefix(l, "cli:") {
				hasCLI = true
			}
			if strings.HasPrefix(l, "blast:") {
				hasBlast = true
			}
		}

		// Fallback to searching the markdown description body for the cli: tag.
		if !hasCLI && body != "" {
			rxCLI := regexp.MustCompile(`(?i)cli:(low|medium|high)`)
			if match := rxCLI.FindStringSubmatch(body); len(match) > 1 {
				labels = append(labels, "cli:"+strings.ToLower(match[1]))
				hasCLI = true
			}
		}

		if !hasCLI {
			allErrors = append(allErrors, "CLI coupling blast radius (cli:high, cli:medium, cli:low) is missing.\n\nPlease define the CLI risk manually using a label or embedded in the task file via `cli:low/medium/high`.")
		}

		// Search markdown description body for blast: risk classification tag
		if !hasBlast && body != "" {
			rxBlast := regexp.MustCompile(`(?i)blast:(low|medium|high)`)
			if match := rxBlast.FindStringSubmatch(body); len(match) > 1 {
				labels = append(labels, "blast:"+strings.ToLower(match[1]))
				hasBlast = true
			}
		}

		if !hasBlast {
			allErrors = append(allErrors, "blast label (blast:low, blast:medium, or blast:high) is missing.\n\nPlease run the /groom workflow to analyze blast radius.")
		}

		if len(allErrors) > 0 {
			return fmt.Errorf("task creation rejected:\n\n%s", strings.Join(allErrors, "\n\n"))
		}

		// Extract optional parent key, task type, project override, and spike flags
		parentVal, _ := cmd.Flags().GetString("parent")
		typeVal, _ := cmd.Flags().GetString("type")
		if typeVal != "" {
			validTypes := map[string]bool{
				string(task.TypeBatch): true,
				string(task.TypeTask):  true,
				string(task.TypeBug):   true,
				string(task.TypeDebt):  true,
			}
			if !validTypes[typeVal] {
				return fmt.Errorf("invalid task type: %s (must be Batch, Task, Bug, or Debt)", typeVal)
			}
		}
		projectVal, _ := cmd.Flags().GetString("project")
		isSpike, _ := cmd.Flags().GetBool("spike")
		triageVal, _ := cmd.Flags().GetBool("triage")

		ctx := context.Background()

		forceVal, _ := cmd.Flags().GetBool("force")
		// Create the task entry in the configured tracking engine store.
		project := filepath.Base(repoRoot)
		if cwd, err := os.Getwd(); err == nil {
			if filepath.Base(cwd) != project {
				project = filepath.Base(cwd)
			}
		}
		if projectVal != "" {
			project = projectVal
		}

		// Perform duplicate task detection using vector embeddings or title similarity matching
		dups, dupErr := task.CheckDuplicateNewTask(ctx, tracker, title, body, project)
		if dupErr == nil && len(dups) > 0 {
			dupMsg := fmt.Sprintf("⚠️  Possible duplicate tasks detected in backlog: %s", strings.Join(dups, ", "))
			if !forceVal {
				return fmt.Errorf("%s\nTask creation rejected. Use --force if you are sure you want to create a duplicate.", dupMsg)
			}
			fmt.Printf("%s\n", dupMsg)
			fmt.Print("Force flag detected. Do you want to proceed and create anyway? (y/N): ")
			var resp string
			fmt.Scanln(&resp)
			if strings.ToLower(strings.TrimSpace(resp)) != "y" {
				return fmt.Errorf("task creation aborted by Product Owner")
			}
		}

		// Determine initial task status (BACKLOG by default, or TRIAGE if requested)
		initialStatus := task.StatusBacklog
		if triageVal {
			initialStatus = task.StatusTriage
		}

		// Invoke local tracker Create method to persist task entry and return generated key
		newKey, err := tracker.Create(ctx, title, body, labels, parentVal, project, task.TaskType(typeVal), isSpike, initialStatus)
		if err != nil {
			return err
		}

		// Update context burden and logic depth ratings if provided via flags
		if burdenVal > 0 || depthVal > 0 {
			_ = tracker.Edit(ctx, newKey, nil, nil, nil, &burdenVal, &depthVal, nil, nil, nil)
		}

		fmt.Printf("Successfully created task: %s (estimated size: %d)\n", newKey, burdenVal+depthVal)

		// Clean up temporary description file if stored inside global or repository tmp directory
		if fileVal != "" {
			absPath, absErr := filepath.Abs(fileVal)
			if absErr == nil {
				tmpDirAbs, tmpDirErr := filepath.Abs(filepath.Join(config.GlobalDataDir(repoRoot), "tmp"))
				if tmpDirErr == nil && strings.HasPrefix(absPath, tmpDirAbs) {
					_ = os.Remove(absPath)
				}
			}
		}

		return nil
	},
}

// Register taskCreateCmd flags and bind subcommand to parent taskCmd
func init() {
	taskCreateCmd.Flags().StringP("file", "F", "", "Path to markdown file containing description/body")
	taskCreateCmd.Flags().String("schema", "code", "Task schema identifier (e.g. code, operations)")
	taskCreateCmd.Flags().Int("burden", 0, "Context burden estimation")
	taskCreateCmd.Flags().Int("depth", 0, "Logic depth estimation")
	taskCreateCmd.Flags().String("label", "", "Comma-separated list of labels to add")
	taskCreateCmd.Flags().String("parent", "", "Parent task key (e.g., Epic ID) for backlog hierarchy")
	taskCreateCmd.Flags().String("type", "", "Task type (Batch, Task, Bug, Debt)")
	taskCreateCmd.Flags().StringP("project", "p", "", "Override the project assignment (defaults to current directory name)")
	taskCreateCmd.Flags().Bool("spike", false, "Mark task as an unconstrained Spike")

	taskCreateCmd.Flags().Bool("triage", false, "Force task into TRIAGE status instead of BACKLOG")
	taskCreateCmd.Flags().Bool("force", false, "Force task creation bypassing duplicate checks")
	taskCmd.AddCommand(taskCreateCmd)
}
