package evolution

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// VersionManager handles PROMPT.md versioning on disk. Agent versions are
// stored at agents/<id>/versions/v1/, agents/<id>/versions/v2/, etc.
// Candidate versions use the suffix "-candidate" (e.g. v3-candidate).
type VersionManager struct {
	agentsDir string
}

// VersionInfo describes a single version of an agent's PROMPT.md.
type VersionInfo struct {
	Version  string // e.g. "v3", "v4-candidate"
	Path     string // absolute path to the version directory
	Status   string // "active", "candidate", "retired"
	IsActive bool
}

// NewVersionManager creates a VersionManager rooted at the given agents directory.
func NewVersionManager(agentsDir string) *VersionManager {
	return &VersionManager{
		agentsDir: agentsDir,
	}
}

// PromoteCandidate promotes the latest vN-candidate to vN by renaming its
// directory. Returns the new version string.
func (vm *VersionManager) PromoteCandidate(agentID string) (string, error) {
	versionsDir := filepath.Join(vm.agentsDir, agentID, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return "", fmt.Errorf("read versions directory: %w", err)
	}

	// Find the candidate directory.
	var candidateDir string
	var candidateVersion string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), "-candidate") {
			candidateDir = filepath.Join(versionsDir, entry.Name())
			candidateVersion = entry.Name()
			break
		}
	}

	if candidateDir == "" {
		return "", fmt.Errorf("no candidate version found for agent %s", agentID)
	}

	// Derive the promoted version name by stripping "-candidate".
	promoted := strings.TrimSuffix(candidateVersion, "-candidate")
	promotedDir := filepath.Join(versionsDir, promoted)

	// Check if the promoted version already exists (shouldn't happen, but guard).
	if _, err := os.Stat(promotedDir); err == nil {
		return "", fmt.Errorf("version %s already exists for agent %s", promoted, agentID)
	}

	if err := os.Rename(candidateDir, promotedDir); err != nil {
		return "", fmt.Errorf("rename %s to %s: %w", candidateVersion, promoted, err)
	}

	return promoted, nil
}

// Rollback sets the active version back to the previous one by renaming
// the current highest version directory with a "-rolledback" suffix.
// Returns the version rolled back to.
func (vm *VersionManager) Rollback(agentID string) (string, error) {
	versions, err := vm.listSortedVersions(agentID)
	if err != nil {
		return "", err
	}

	// Filter to only non-candidate, non-rolledback versions.
	var released []parsedVersion
	for _, v := range versions {
		if !strings.HasSuffix(v.raw, "-candidate") && !strings.HasSuffix(v.raw, "-rolledback") {
			released = append(released, v)
		}
	}

	if len(released) < 2 {
		return "", fmt.Errorf("cannot rollback: agent %s has fewer than 2 released versions", agentID)
	}

	// The last entry is the current active version — rename it to mark as rolled back.
	current := released[len(released)-1]
	currentDir := filepath.Join(vm.agentsDir, agentID, "versions", current.raw)
	rolledBackDir := currentDir + "-rolledback"

	if err := os.Rename(currentDir, rolledBackDir); err != nil {
		return "", fmt.Errorf("rename %s for rollback: %w", current.raw, err)
	}

	// The previous version is now active.
	previous := released[len(released)-2]
	return previous.raw, nil
}

// GetActiveVersion returns the highest non-candidate version string.
func (vm *VersionManager) GetActiveVersion(agentID string) (string, error) {
	versions, err := vm.listSortedVersions(agentID)
	if err != nil {
		return "", err
	}

	// Walk backwards to find the highest non-candidate, non-rolledback version.
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if !strings.HasSuffix(v.raw, "-candidate") && !strings.HasSuffix(v.raw, "-rolledback") {
			return v.raw, nil
		}
	}

	return "", fmt.Errorf("no active version found for agent %s", agentID)
}

// GetVersionHistory returns all versions with their status.
func (vm *VersionManager) GetVersionHistory(agentID string) ([]VersionInfo, error) {
	versionsDir := filepath.Join(vm.agentsDir, agentID, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read versions directory: %w", err)
	}

	activeVersion, _ := vm.GetActiveVersion(agentID)

	var history []VersionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		info := VersionInfo{
			Version: name,
			Path:    filepath.Join(versionsDir, name),
		}

		switch {
		case strings.HasSuffix(name, "-candidate"):
			info.Status = "candidate"
		case strings.HasSuffix(name, "-rolledback"):
			info.Status = "retired"
		case name == activeVersion:
			info.Status = "active"
			info.IsActive = true
		default:
			info.Status = "retired"
		}

		history = append(history, info)
	}

	return history, nil
}

// parsedVersion holds a raw version string and its numeric component for sorting.
type parsedVersion struct {
	raw string
	num int
}

// listSortedVersions reads the versions directory and returns entries sorted
// by version number ascending.
func (vm *VersionManager) listSortedVersions(agentID string) ([]parsedVersion, error) {
	versionsDir := filepath.Join(vm.agentsDir, agentID, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil, fmt.Errorf("read versions directory for %s: %w", agentID, err)
	}

	var versions []parsedVersion
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		num := extractVersionNumber(name)
		versions = append(versions, parsedVersion{raw: name, num: num})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].num < versions[j].num
	})

	return versions, nil
}

// extractVersionNumber parses the numeric part from a version string.
// "v3" -> 3, "v4-candidate" -> 4, "v2-rolledback" -> 2
func extractVersionNumber(version string) int {
	cleaned := strings.TrimPrefix(version, "v")

	// Take only the numeric prefix.
	numStr := ""
	for _, ch := range cleaned {
		if ch >= '0' && ch <= '9' {
			numStr += string(ch)
		} else {
			break
		}
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return num
}
