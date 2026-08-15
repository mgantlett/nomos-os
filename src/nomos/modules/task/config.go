package task

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// Config represents task tracker connection parameters.
type Config struct {
	TrackerType string
	RepoRoot    string
}

// LoadConfig parses local and global environment files to load task credentials dynamically.
// It loads configuration sequentially from the user's home directory config followed
// by active repository configs, allowing environment overrides.
func LoadConfig(repoRoot string) (*Config, error) {
	// 1. Load from home global config files
	home, err := os.UserHomeDir()
	if err == nil {
		parseEnvFile(filepath.Join(home, ".global.nomos.config.env"))
	}

	// 2. Load from global data directory configs
	parseEnvFile(filepath.Join(config.GlobalDataDir(repoRoot), "config.env"))
	parseEnvFile(filepath.Join(config.GlobalDataDir(repoRoot), "config.local.env"))

	// Bind overrides from standard environment variables
	trackerType := os.Getenv("NOMOS_DEFAULT_TASK_TRACKER")
	if trackerType == "" {
		trackerType = "local" // Default fallback is now local
	}

	return &Config{
		TrackerType: strings.ToLower(trackerType),
		RepoRoot:    repoRoot,
	}, nil
}

// parseEnvFile reads environment variables defined in the given file path
// and propagates them into the active process using os.Setenv.
func parseEnvFile(path string) {
	m, err := config.ParseEnvFile(path)
	if err != nil {
		return
	}
	for k, v := range m {
		os.Setenv(k, v)
	}
}
