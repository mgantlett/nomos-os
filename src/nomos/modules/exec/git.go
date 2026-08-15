package exec

import (
	"strings"
)

// GitStatus runs 'git status' using the command runner and returns the output.
func GitStatus(dbPath string) (string, error) {
	return RunCommand(dbPath, "", "git", "status")
}

// GitCheckout runs 'git checkout <branch>' using the command runner and returns the output.
func GitCheckout(dbPath string, branch string) (string, error) {
	return RunCommand(dbPath, "", "git", "checkout", branch)
}

// GitDiff runs 'git diff' using the command runner and returns the output.
// If staged is true, it passes the '--cached' flag to obtain the staged diff.
func GitDiff(dbPath string, staged bool) (string, error) {
	if staged {
		return RunCommand(dbPath, "", "git", "diff", "--cached")
	}
	return RunCommand(dbPath, "", "git", "diff")
}

// GitStashSave stashes active uncommitted changes in directory wd with a custom message.
func GitStashSave(dbPath string, wd string, message string) (string, error) {
	return RunCommand(dbPath, wd, "git", "stash", "save", message)
}

// GitStashPopByName pops a stash that matches a specific substring in its message in directory wd.
func GitStashPopByName(dbPath string, wd string, name string) (string, error) {
	out, err := RunCommand(dbPath, wd, "git", "stash", "list")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, name) {
			// Extract the stash name, e.g. "stash@{0}"
			idx := strings.Index(line, ":")
			if idx != -1 {
				stashRef := strings.TrimSpace(line[:idx]) // "stash@{0}"
				return RunCommand(dbPath, wd, "git", "stash", "pop", stashRef)
			}
		}
	}
	return "", nil
}
