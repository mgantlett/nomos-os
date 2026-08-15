package exec

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
)

type VerificationStage struct {
	Name     string `json:"name"`
	Guidance string `json:"guidance"`
}

// GenerateWiki exports the code-truth based on configuration to the outDir.
// It iterates through the Wiki configuration sync targets and delegates to specific generators.
func GenerateWiki(root string, outDir string) error {
	configPath := filepath.Join(config.GlobalDataDir(root), "config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config for wiki generation: %w", err)
	}

	if len(cfg.Wiki.SyncTargets) == 0 {
		synapse.Info("No wiki sync_targets defined in config. Skipping generation.\n")
		return nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
	}

	dbPath := config.ResolveCacheDbPath(root)
	binPath := filepath.Join(root, "bin", "nomos")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		binPath = "nomos"
	}

	for _, target := range cfg.Wiki.SyncTargets {
		if err := processWikiTarget(target.Type, target.File, target.Source, root, dbPath, binPath, outDir); err != nil {
			return err
		}
	}

	return nil
}

// processWikiTarget routes the generation logic based on target type.
func processWikiTarget(targetType, targetFile, targetSource, root, dbPath, binPath, outDir string) error {
	outFilePath := filepath.Join(outDir, targetFile)
	switch targetType {
	case "cli_schema":
		if err := generateCliSchema(dbPath, binPath, outFilePath); err != nil {
			return err
		}
		synapse.Info("Generated CLI Schema -> %s\n", targetFile)
	case "dod_gates":
		if err := generateDodGates(dbPath, binPath, outFilePath); err != nil {
			return err
		}
		synapse.Info("Generated DoD Gates -> %s\n", targetFile)
	case "workflows":
		if err := generateWorkflows(root, outFilePath); err != nil {
			return err
		}
		synapse.Info("Generated Workflows -> %s\n", targetFile)
	case "raw_file":
		if err := generateRawFile(root, targetSource, outFilePath); err != nil {
			return err
		}
		synapse.Info("Copied Raw File -> %s\n", targetFile)
	default:
		synapse.Info("Unknown sync target type %s, skipping.\n", targetType)
	}
	return nil
}

// generateCliSchema extracts the CLI schema and formats it as Markdown.
func generateCliSchema(dbPath, binPath, outFilePath string) error {
	outputStr, err := RunCommand(dbPath, "", binPath, "schema", "cli")
	if err != nil {
		return fmt.Errorf("failed to get cli schema: %w", err)
	}
	var cliSchema schema.CliSchema
	if err := json.Unmarshal([]byte(outputStr), &cliSchema); err != nil {
		return fmt.Errorf("failed to parse cli schema: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Nomos CLI Reference\n\n")
	generateCliDocs(&cliSchema, "", &sb)

	if err := os.WriteFile(outFilePath, []byte(sb.String()), 0644); err != nil {
		return err
	}
	return nil
}

// generateDodGates extracts the DoD verification gates and formats them as Markdown.
func generateDodGates(dbPath, binPath, outFilePath string) error {
	outputStr, err := RunCommand(dbPath, "", binPath, "verify", "--list-json")
	if err != nil {
		return fmt.Errorf("failed to get dod gates: %w\nOutput: %s", err, outputStr)
	}
	var stages []VerificationStage
	if err := json.Unmarshal([]byte(outputStr), &stages); err != nil {
		return fmt.Errorf("failed to parse dod gates: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Definition of Done Gates\n\n")
	for _, stage := range stages {
		escapedGuidance := html.EscapeString(stage.Guidance)
		sb.WriteString(fmt.Sprintf("<details>\n<summary><b>%s</b></summary>\n\n%s\n\n</details>\n\n", stage.Name, escapedGuidance))
	}

	if err := os.WriteFile(outFilePath, []byte(sb.String()), 0644); err != nil {
		return err
	}
	return nil
}

// generateWorkflows iterates over agent workflows and concatenates them into a single Markdown file.
func generateWorkflows(root, outFilePath string) error {
	workflowsDir := config.WorkflowsDir(root)
	var sb strings.Builder
	sb.WriteString("# Agent Workflows\n\n")

	if _, err := os.Stat(workflowsDir); err == nil {
		entries, _ := os.ReadDir(workflowsDir)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				content, err := os.ReadFile(filepath.Join(workflowsDir, entry.Name()))
				if err == nil {
					sb.WriteString(string(content))
					sb.WriteString("\n\n---\n\n")
				}
			}
		}
	}
	if err := os.WriteFile(outFilePath, []byte(sb.String()), 0644); err != nil {
		return err
	}
	return nil
}

// generateRawFile resolves raw file paths and copies them.
func generateRawFile(root, source, outFilePath string) error {
	if strings.HasPrefix(source, "~/") {
		home, _ := os.UserHomeDir()
		source = filepath.Join(home, source[2:])
	} else if !filepath.IsAbs(source) {
		source = filepath.Join(root, source)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("failed to read raw file %s: %w", source, err)
	}
	if err := os.WriteFile(outFilePath, content, 0644); err != nil {
		return err
	}
	return nil
}

func generateCliDocs(cmd *schema.CliSchema, prefix string, sb *strings.Builder) {
	cmdName := cmd.Name
	if prefix != "" {
		cmdName = prefix + " " + cmd.Name
	}
	sb.WriteString(fmt.Sprintf("## `%s`\n\n", cmdName))

	if len(cmd.Flags) > 0 {
		sb.WriteString("**Flags:**\n")
		for _, flag := range cmd.Flags {
			sb.WriteString(fmt.Sprintf("- `--%s`\n", flag))
		}
		sb.WriteString("\n")
	}

	for _, sub := range cmd.Subcommands {
		generateCliDocs(&sub, cmdName, sb)
	}
}
