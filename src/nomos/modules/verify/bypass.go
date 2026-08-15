// Package verify implements quality assurance check gates for the Nomos repository.
// This file manages the technical quality debt manifest, providing functionality
// to register, query, verify expiration of, and automatically resolve/prune
// active bypass tokens on files that are out of scope.
package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// QualityDebtItem represents an active technical debt bypass record.
// It maps a target file to a specific gate, documenting the reason and linked task.
type QualityDebtItem struct {
	File       string `json:"file"`
	Gate       string `json:"gate"`
	Reason     string `json:"reason"`
	LinkedTask string `json:"linked_task"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
}

// QualityDebtManifest holds list of active technical debt entries.
type QualityDebtManifest struct {
	ActiveDebt []QualityDebtItem `json:"active_debt"`
}

// checkSingleDebtItem evaluates a single quality debt item against the requested file and gate.
// It verifies expiration status and ensures the active task is not illegally linked to the bypass.
// Returns (isValid, linkedTask, isMatch).
func checkSingleDebtItem(repoRoot, relFile, gate, activeTaskId string, item QualityDebtItem) (bool, string, bool) {
	if getRelativePath(repoRoot, item.File) == relFile && item.Gate == gate {
		if isBypassExpired(repoRoot, item) {
			return false, "", true
		}
		if item.LinkedTask == activeTaskId && activeTaskId != "" && activeTaskId != "AUTO" {
			synapse.Info("\x1b[31m❌ [Quality Debt Loophole Blocked] Bypass for '%s' (gate: %s) is illegally linked to the active task (%s)\x1b[0m\n", relFile, gate, activeTaskId)
			return false, "", true
		}
		return true, item.LinkedTask, true
	}
	return false, "", false
}

// CheckQualityDebtBypass checks if a file has an active quality debt bypass.
// Bypasses allow temporary exceptions when resolving technical debt in legacy code.
// The bypass is verified against an active expiration timestamp.
func CheckQualityDebtBypass(repoRoot string, file string, gate DebtGate) (bool, string) {
	if getActiveAgentTier(repoRoot) == state.Tier1 {
		return false, ""
	}

	manifest, err := readQualityDebtManifest(repoRoot)
	if err != nil {
		return false, ""
	}

	activeTaskId := GetActiveTaskId(repoRoot)
	relFile := getRelativePath(repoRoot, file)
	for _, item := range manifest.ActiveDebt {
		valid, linkedTask, matched := checkSingleDebtItem(repoRoot, relFile, string(gate), activeTaskId, item)
		if matched {
			return valid, linkedTask
		}
	}
	return false, ""
}
