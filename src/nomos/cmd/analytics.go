package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

type weeklyVelocityEntry struct {
	Week    int `json:"week"`
	Commits int `json:"commits"`
}

type commitTypes map[string]int

type telemetryData struct {
	TotalEvents int    `json:"total_events"`
	Transitions int    `json:"transitions"`
	Failures    int    `json:"failures"`
	Lockouts    int    `json:"lockouts"`
	Bypasses    int    `json:"bypasses"`
	BypassRatio string `json:"bypass_ratio"`
}

type moduleRating struct {
	Module           string  `json:"module"`
	Grade            string  `json:"grade"`
	Successes        float64 `json:"successes"`
	Fails            float64 `json:"fails"`
	ConsecutiveFails float64 `json:"consecutive_fails"`
}

type analyticsData struct {
	Period          string                `json:"period"`
	TotalCommits    int                   `json:"total_commits"`
	WeeklyVelocity  []weeklyVelocityEntry `json:"weekly_velocity"`
	CommitTypes     map[string]int        `json:"commit_types"`
	Telemetry       telemetryData         `json:"telemetry"`
	ModuleRatings   []moduleRating        `json:"module_ratings"`
	LayerAverages   map[string]float64    `json:"layer_averages"`
	EpicProjections map[string]float64    `json:"epic_projections"`
}

var (
	analyticsSummaryFlag bool
	analyticsJSONFlag    bool

	analyticsCmd = &cobra.Command{
		Use:   "analytics",
		Short: "Generate agent performance and codebase velocity analytics",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)

			since := "30 days ago"
			totalCommitsStr, err := runGitCommand(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "rev-list", "--count", "--since="+since, "HEAD")
			if err != nil {
				if analyticsJSONFlag {
					return fmt.Errorf("no git history found or not a git repository")
				}
				synapse.Info("%s", fmt.Sprint("  No git history found or not a git repository."))
				return nil
			}

			totalCommits, _ := strconv.Atoi(totalCommitsStr)
			if totalCommits == 0 && !analyticsJSONFlag {
				synapse.Info("%s", fmt.Sprint("  No commits in this period."))
				return nil
			}

			weeklyVelocity, maxWeeklyVal, err := collectWeeklyVelocity(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
			if err != nil {
				return err
			}

			CommitTypes, maxTypeVal, err := collectCommitTypes(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), since)
			if err != nil {
				return err
			}

			TelemetryData, err := collectTelemetry(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), totalCommits)
			if err != nil {
				return err
			}

			ModuleRatings, err := collectModuleRatings(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
			if err != nil {
				return err
			}

			tracker, _, _ := loadTrackerAndRoot()
			ctx := context.Background()
			tasks, _ := tracker.List(ctx)
			_ = task.SyncAgentVelocities(ctx, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), tasks)
			averages, _ := task.GetRollingAverages(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

			epicProjections := make(map[string]float64)
			for _, t := range tasks {
				if t.Type == task.TypeBatch && !t.IsClosed() {
					// We can group tasks by their ParentKey
					epicTasks := []task.Task{t}
					for _, child := range tasks {
						if child.ParentKey == t.Key {
							epicTasks = append(epicTasks, child)
						}
					}
					dur, _ := task.CalculateCriticalPath(epicTasks, averages)
					epicProjections[t.Key] = dur
				}
			}

			data := &analyticsData{
				Period:          "30d",
				TotalCommits:    totalCommits,
				WeeklyVelocity:  weeklyVelocity,
				CommitTypes:     CommitTypes,
				Telemetry:       TelemetryData,
				ModuleRatings:   ModuleRatings,
				LayerAverages:   averages,
				EpicProjections: epicProjections,
			}

			if analyticsJSONFlag {
				return renderJSON(data)
			}

			// For text output, determine if files exist to show sections
			telemetryPath := workspace.MustNewContext(repoRoot).NomosStatePath("logs", "telemetry.jsonl")
			_, errTelemetry := os.Stat(telemetryPath)
			hasTelemetry := errTelemetry == nil

			phaseStatePath := workspace.MustNewContext(repoRoot).NomosStatePath(".phase_state.json")
			_, errPhaseState := os.Stat(phaseStatePath)
			hasModuleRatings := errPhaseState == nil && len(ModuleRatings) > 0

			renderText(data, maxWeeklyVal, maxTypeVal, hasTelemetry, hasModuleRatings)
			return nil
		},
	}
)

func runGitCommand(ctx *workspace.WorkspaceContext, args ...string) (string, error) {
	repoRoot := ctx.RepoRoot
	gitCmd := exec.Command("git", args...)
	gitCmd.Dir = repoRoot
	out, err := gitCmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func collectWeeklyVelocity(ctx *workspace.WorkspaceContext) ([]weeklyVelocityEntry, int, error) {
	repoRoot := ctx.RepoRoot
	weeklyVelocity := make([]weeklyVelocityEntry, 0, 4)
	maxVal := 1
	for i := 4; i >= 1; i-- {
		wStart := fmt.Sprintf("%d days ago", i*7)
		wEnd := fmt.Sprintf("%d days ago", (i-1)*7)
		cStr, _ := runGitCommand(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "rev-list", "--count", "--since="+wStart, "--until="+wEnd, "HEAD")
		c, _ := strconv.Atoi(cStr)
		weeklyVelocity = append(weeklyVelocity, weeklyVelocityEntry{
			Week:    5 - i,
			Commits: c,
		})
		if c > maxVal {
			maxVal = c
		}
	}
	return weeklyVelocity, maxVal, nil
}

func collectCommitTypes(ctx *workspace.WorkspaceContext, since string) (commitTypes, int, error) {
	repoRoot := ctx.RepoRoot
	types := []string{"feat", "fix", "docs", "refactor", "chore"}
	commitTypesMap := make(commitTypes)
	maxTypeVal := 1
	for _, t := range types {
		cStr, _ := runGitCommand(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "log", "--since="+since, "--oneline", "--grep=^[^ ]* "+t)
		lines := strings.Split(cStr, "\n")
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		commitTypesMap[t] = count
		if count > maxTypeVal {
			maxTypeVal = count
		}
	}
	return commitTypesMap, maxTypeVal, nil
}

func collectTelemetry(ctx *workspace.WorkspaceContext, totalCommits int) (telemetryData, error) {
	repoRoot := ctx.RepoRoot
	telemetryPath := workspace.MustNewContext(repoRoot).NomosStatePath("logs", "telemetry.jsonl")
	if fi, err := os.Stat(telemetryPath); err != nil || fi.IsDir() {
		return telemetryData{
			TotalEvents: 0,
			Transitions: 0,
			Failures:    0,
			Lockouts:    0,
			Bypasses:    0,
			BypassRatio: "0.0%",
		}, nil
	}

	file, err := os.Open(telemetryPath)
	if err != nil {
		return telemetryData{
			TotalEvents: 0,
			Transitions: 0,
			Failures:    0,
			Lockouts:    0,
			Bypasses:    0,
			BypassRatio: "0.0%",
		}, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	totalEvents := 0
	transitions := 0
	failures := 0
	lockouts := 0
	bypasses := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		totalEvents++
		var event map[string]interface{}
		if json.Unmarshal([]byte(line), &event) == nil {
			processTelemetryEvent(event, &transitions, &failures, &lockouts, &bypasses)
		}
	}

	bypassRatio := "0%"
	if totalCommits > 0 {
		bypassRatio = fmt.Sprintf("%.1f%%", float64(bypasses*100)/float64(totalCommits))
	}

	return telemetryData{
		TotalEvents: totalEvents,
		Transitions: transitions,
		Failures:    failures,
		Lockouts:    lockouts,
		Bypasses:    bypasses,
		BypassRatio: bypassRatio,
	}, nil
}

func processTelemetryEvent(event map[string]interface{}, transitions, failures, lockouts, bypasses *int) {
	evType, _ := event["event_type"].(string)
	detail, _ := event["detail"].(string)

	if evType == "phase_transition" {
		*transitions++
	}
	if evType == "error" || (evType == "dod_result" && strings.Contains(strings.ToLower(detail), "fail")) {
		*failures++
	}
	if evType == "substrate_state" && strings.Contains(strings.ToLower(detail), "locked") {
		*lockouts++
	}
	if evType == "gate_bypass" {
		*bypasses++
	}
}

func collectModuleRatings(ctx *workspace.WorkspaceContext) ([]moduleRating, error) {
	repoRoot := ctx.RepoRoot
	ModuleRatings := []moduleRating{}
	phaseStatePath := workspace.MustNewContext(repoRoot).NomosStatePath(".phase_state.json")
	data, err := os.ReadFile(phaseStatePath)
	if err != nil {
		return ModuleRatings, nil
	}

	var phaseData map[string]interface{}
	if err := json.Unmarshal(data, &phaseData); err != nil {
		return ModuleRatings, nil
	}

	metrics, ok := phaseData["module_metrics"].(map[string]interface{})
	if !ok || len(metrics) == 0 {
		return ModuleRatings, nil
	}

	var sortedModules []string
	for mod := range metrics {
		sortedModules = append(sortedModules, mod)
	}
	sort.Strings(sortedModules)

	for _, mod := range sortedModules {
		val := metrics[mod]
		if mInfo, ok := val.(map[string]interface{}); ok {
			success, _ := mInfo["success_count"].(float64)
			failed, _ := mInfo["failed_runs"].(float64)
			consecutive, _ := mInfo["consecutive_fails"].(float64)

			// Grade calculation logic
			grade := "Expert"
			if failed > success {
				grade = "Novice"
			} else if failed > 0 && failed*2 > success {
				grade = "Competent"
			}

			ModuleRatings = append(ModuleRatings, moduleRating{
				Module:           mod,
				Grade:            grade,
				Successes:        success,
				Fails:            failed,
				ConsecutiveFails: consecutive,
			})
		}
	}

	return ModuleRatings, nil
}

func renderJSON(data *analyticsData) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func renderText(data *analyticsData, maxWeeklyVal int, maxTypeVal int, hasTelemetry bool, hasModuleRatings bool) {
	synapse.Info("%s", fmt.Sprint("\x1b[1m\x1b[36m  📊 Nomos Agent Performance Analytics (30d)\x1b[0m"))
	synapse.Info("")

	renderWeeklyVelocity(data, maxWeeklyVal)
	renderCommitTypes(data, maxTypeVal)
	renderTelemetry(data, hasTelemetry)
	renderModuleRatings(data, hasModuleRatings)
	renderCPM(data)
}

func renderWeeklyVelocity(data *analyticsData, maxWeeklyVal int) {
	synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Weekly Velocity Trend ──────────────────────────────\x1b[0m"))
	for _, entry := range data.WeeklyVelocity {
		barLen := (entry.Commits * 40) / maxWeeklyVal
		if barLen < 1 && entry.Commits > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		synapse.Info("    Week %-2d: %-40s %d\n", entry.Week, bar, entry.Commits)
	}
	synapse.Info("")
}

func renderCommitTypes(data *analyticsData, maxTypeVal int) {
	synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Commit Type Distribution ───────────────────────────\x1b[0m"))
	types := []string{"feat", "fix", "docs", "refactor", "chore"}
	for _, t := range types {
		count := data.CommitTypes[t]
		barLen := (count * 30) / maxTypeVal
		if barLen < 1 && count > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		synapse.Info("    %-10s %-30s %d\n", t+":", bar, count)
	}
	synapse.Info("")
}

func renderTelemetry(data *analyticsData, hasTelemetry bool) {
	if hasTelemetry {
		synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Telemetric Event Analytics ─────────────────────────\x1b[0m"))
		t := data.Telemetry
		synapse.Info("    Total Telemetric Events:   %d\n", t.TotalEvents)
		synapse.Info("    Operational Transitions:   %d\n", t.Transitions)
		synapse.Info("    Verification Failures:     %d\n", t.Failures)
		synapse.Info("    Substrate Lockouts:        %d\n", t.Lockouts)
		synapse.Info("    Gate Bypasses:             %d\n", t.Bypasses)
		synapse.Info("    Bypass Ratio:              %s (%d bypasses / %d commits)\n", t.BypassRatio, t.Bypasses, data.TotalCommits)
		synapse.Info("")
	}
}

func renderModuleRatings(data *analyticsData, hasModuleRatings bool) {
	if hasModuleRatings {
		synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Codebase Module Competence Ratings ─────────────────\x1b[0m"))
		for _, r := range data.ModuleRatings {
			gradeColor := "\x1b[32m" // green
			if r.Grade == "Novice" {
				gradeColor = "\x1b[33m" // yellow
			}
			synapse.Info("    %-15s → %s%-12s\x1b[0m (Successes: %.0f | Fails: %.0f | Streak Fails: %.0f)\n",
				r.Module, gradeColor, r.Grade, r.Successes, r.Fails, r.ConsecutiveFails)
		}
		synapse.Info("")
	}
}

func renderCPM(data *analyticsData) {
	if len(data.LayerAverages) > 0 {
		synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Critical Path (CPM) Layer Averages ─────────────────\x1b[0m"))
		for layer, avg := range data.LayerAverages {
			synapse.Info("    %-15s → %.1f agent cycles\n", layer, avg)
		}
		synapse.Info("")
	}

	if len(data.EpicProjections) > 0 {
		synapse.Info("%s", fmt.Sprint("  \x1b[1m─── Epic Projections (CPM) ─────────────────────────────\x1b[0m"))
		for epic, dur := range data.EpicProjections {
			synapse.Info("    %-15s → Projected %.1f agent cycles\n", epic, dur)
		}
		synapse.Info("")
	}
}

func init() {
	analyticsCmd.Flags().BoolVar(&analyticsSummaryFlag, "summary", false, "Output simplified summary of analytics metrics")
	analyticsCmd.Flags().BoolVar(&analyticsJSONFlag, "json", false, "Output analytics metrics in JSON format")
	RootCmd.AddCommand(analyticsCmd)
}
