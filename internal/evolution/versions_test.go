package evolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewVersionManager(t *testing.T) {
	agentsDir := "/tmp/agents"
	vm := NewVersionManager(agentsDir)

	if vm.agentsDir != agentsDir {
		t.Errorf("agentsDir = %q, want %q", vm.agentsDir, agentsDir)
	}
}

func TestPromoteCandidate(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		setupVersions []string // versions to create (e.g. "v1", "v2-candidate")
		wantVersion   string   // expected promoted version
		wantErr       bool
	}{
		{
			name:          "promote v2-candidate to v2",
			agentID:       "agent1",
			setupVersions: []string{"v1", "v2-candidate"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "promote v3-candidate to v3",
			agentID:       "agent2",
			setupVersions: []string{"v1", "v2", "v3-candidate"},
			wantVersion:   "v3",
			wantErr:       false,
		},
		{
			name:          "no candidate version",
			agentID:       "agent3",
			setupVersions: []string{"v1", "v2"},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "promoted version already exists",
			agentID:       "agent4",
			setupVersions: []string{"v1", "v2", "v2-candidate"},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "no versions directory",
			agentID:       "agent5",
			setupVersions: nil,
			wantVersion:   "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			vm := NewVersionManager(agentsDir)

			// Setup versions directory structure
			if tt.setupVersions != nil {
				versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
				if err := os.MkdirAll(versionsDir, 0755); err != nil {
					t.Fatalf("failed to create versions directory: %v", err)
				}

				for _, version := range tt.setupVersions {
					versionDir := filepath.Join(versionsDir, version)
					if err := os.MkdirAll(versionDir, 0755); err != nil {
						t.Fatalf("failed to create version directory %s: %v", version, err)
					}
				}
			}

			// Execute
			got, err := vm.PromoteCandidate(tt.agentID)

			// Verify error
			if (err != nil) != tt.wantErr {
				t.Errorf("PromoteCandidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify result
			if got != tt.wantVersion {
				t.Errorf("PromoteCandidate() = %q, want %q", got, tt.wantVersion)
			}

			// If successful, verify directory was renamed
			if !tt.wantErr && tt.setupVersions != nil {
				versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
				promotedDir := filepath.Join(versionsDir, tt.wantVersion)

				// Check that promoted directory exists
				if _, err := os.Stat(promotedDir); os.IsNotExist(err) {
					t.Errorf("promoted directory %s does not exist", promotedDir)
				}

				// Check that candidate directory no longer exists
				candidateDir := filepath.Join(versionsDir, tt.wantVersion+"-candidate")
				if _, err := os.Stat(candidateDir); !os.IsNotExist(err) {
					t.Errorf("candidate directory %s should not exist after promotion", candidateDir)
				}
			}
		})
	}
}

func TestRollback(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		setupVersions []string
		wantVersion   string // version rolled back TO
		wantErr       bool
	}{
		{
			name:          "rollback from v3 to v2",
			agentID:       "agent1",
			setupVersions: []string{"v1", "v2", "v3"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "rollback from v5 to v4",
			agentID:       "agent2",
			setupVersions: []string{"v1", "v2", "v3", "v4", "v5"},
			wantVersion:   "v4",
			wantErr:       false,
		},
		{
			name:          "rollback with candidate present",
			agentID:       "agent3",
			setupVersions: []string{"v1", "v2", "v3", "v4-candidate"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "rollback with previous rolledback version",
			agentID:       "agent4",
			setupVersions: []string{"v1", "v2", "v3", "v4-rolledback"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "cannot rollback with only 1 version",
			agentID:       "agent5",
			setupVersions: []string{"v1"},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "cannot rollback with no versions",
			agentID:       "agent6",
			setupVersions: []string{},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "cannot rollback with only candidate",
			agentID:       "agent7",
			setupVersions: []string{"v1-candidate"},
			wantVersion:   "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			vm := NewVersionManager(agentsDir)

			// Setup versions directory structure
			versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
			if err := os.MkdirAll(versionsDir, 0755); err != nil {
				t.Fatalf("failed to create versions directory: %v", err)
			}

			for _, version := range tt.setupVersions {
				versionDir := filepath.Join(versionsDir, version)
				if err := os.MkdirAll(versionDir, 0755); err != nil {
					t.Fatalf("failed to create version directory %s: %v", version, err)
				}
			}

			// Execute
			got, err := vm.Rollback(tt.agentID)

			// Verify error
			if (err != nil) != tt.wantErr {
				t.Errorf("Rollback() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify result
			if got != tt.wantVersion {
				t.Errorf("Rollback() = %q, want %q", got, tt.wantVersion)
			}

			// If successful, verify directory was renamed
			if !tt.wantErr {
				// Find the highest non-candidate, non-rolledback version from setup
				// This is the version that should have been rolled back
				var currentVersion string
				maxNum := 0
				for _, v := range tt.setupVersions {
					// Skip candidates and already-rolledback versions
					if strings.Contains(v, "-candidate") || strings.Contains(v, "-rolledback") {
						continue
					}
					num := extractVersionNumber(v)
					if num > maxNum {
						maxNum = num
						currentVersion = v
					}
				}

				if currentVersion != "" {
					// Check that current version is now marked as rolledback
					rolledbackDir := filepath.Join(versionsDir, currentVersion+"-rolledback")
					if _, err := os.Stat(rolledbackDir); os.IsNotExist(err) {
						t.Errorf("rolledback directory %s does not exist", rolledbackDir)
					}

					// Check that original current version directory no longer exists
					currentDir := filepath.Join(versionsDir, currentVersion)
					if _, err := os.Stat(currentDir); !os.IsNotExist(err) {
						t.Errorf("current directory %s should not exist after rollback", currentDir)
					}
				}
			}
		})
	}
}

func TestGetActiveVersion(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		setupVersions []string
		wantVersion   string
		wantErr       bool
	}{
		{
			name:          "highest version is active",
			agentID:       "agent1",
			setupVersions: []string{"v1", "v2", "v3"},
			wantVersion:   "v3",
			wantErr:       false,
		},
		{
			name:          "skip candidate version",
			agentID:       "agent2",
			setupVersions: []string{"v1", "v2", "v3-candidate"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "skip rolledback version",
			agentID:       "agent3",
			setupVersions: []string{"v1", "v2", "v3-rolledback"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "skip both candidate and rolledback",
			agentID:       "agent4",
			setupVersions: []string{"v1", "v2", "v3-rolledback", "v4-candidate"},
			wantVersion:   "v2",
			wantErr:       false,
		},
		{
			name:          "only candidate versions",
			agentID:       "agent5",
			setupVersions: []string{"v1-candidate", "v2-candidate"},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "no versions directory",
			agentID:       "agent6",
			setupVersions: nil,
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "empty versions directory",
			agentID:       "agent7",
			setupVersions: []string{},
			wantVersion:   "",
			wantErr:       true,
		},
		{
			name:          "version sorting v10 > v2",
			agentID:       "agent8",
			setupVersions: []string{"v1", "v2", "v10"},
			wantVersion:   "v10",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			vm := NewVersionManager(agentsDir)

			// Setup versions directory structure
			if tt.setupVersions != nil {
				versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
				if err := os.MkdirAll(versionsDir, 0755); err != nil {
					t.Fatalf("failed to create versions directory: %v", err)
				}

				for _, version := range tt.setupVersions {
					versionDir := filepath.Join(versionsDir, version)
					if err := os.MkdirAll(versionDir, 0755); err != nil {
						t.Fatalf("failed to create version directory %s: %v", version, err)
					}
				}
			}

			// Execute
			got, err := vm.GetActiveVersion(tt.agentID)

			// Verify error
			if (err != nil) != tt.wantErr {
				t.Errorf("GetActiveVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify result
			if got != tt.wantVersion {
				t.Errorf("GetActiveVersion() = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}

func TestGetVersionHistory(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		setupVersions []string
		wantCount     int
		wantStatuses  map[string]string // version -> status
	}{
		{
			name:          "all version types",
			agentID:       "agent1",
			setupVersions: []string{"v1", "v2", "v3", "v4-candidate"},
			wantCount:     4,
			wantStatuses: map[string]string{
				"v1":           "retired",
				"v2":           "retired",
				"v3":           "active",
				"v4-candidate": "candidate",
			},
		},
		{
			name:          "with rolledback version",
			agentID:       "agent2",
			setupVersions: []string{"v1", "v2", "v3-rolledback"},
			wantCount:     3,
			wantStatuses: map[string]string{
				"v1":             "retired",
				"v2":             "active",
				"v3-rolledback":  "retired",
			},
		},
		{
			name:          "single active version",
			agentID:       "agent3",
			setupVersions: []string{"v1"},
			wantCount:     1,
			wantStatuses: map[string]string{
				"v1": "active",
			},
		},
		{
			name:          "no versions directory",
			agentID:       "agent4",
			setupVersions: nil,
			wantCount:     0,
			wantStatuses:  map[string]string{},
		},
		{
			name:          "empty versions directory",
			agentID:       "agent5",
			setupVersions: []string{},
			wantCount:     0,
			wantStatuses:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			vm := NewVersionManager(agentsDir)

			// Setup versions directory structure
			if tt.setupVersions != nil {
				versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
				if err := os.MkdirAll(versionsDir, 0755); err != nil {
					t.Fatalf("failed to create versions directory: %v", err)
				}

				for _, version := range tt.setupVersions {
					versionDir := filepath.Join(versionsDir, version)
					if err := os.MkdirAll(versionDir, 0755); err != nil {
						t.Fatalf("failed to create version directory %s: %v", version, err)
					}
				}
			}

			// Execute
			history, err := vm.GetVersionHistory(tt.agentID)
			if err != nil {
				t.Fatalf("GetVersionHistory() error = %v", err)
			}

			// Verify count
			if len(history) != tt.wantCount {
				t.Errorf("GetVersionHistory() returned %d versions, want %d", len(history), tt.wantCount)
			}

			// Verify statuses
			for _, info := range history {
				wantStatus, ok := tt.wantStatuses[info.Version]
				if !ok {
					t.Errorf("unexpected version %q in history", info.Version)
					continue
				}

				if info.Status != wantStatus {
					t.Errorf("version %q status = %q, want %q", info.Version, info.Status, wantStatus)
				}

				// Verify IsActive flag matches status
				wantActive := (wantStatus == "active")
				if info.IsActive != wantActive {
					t.Errorf("version %q IsActive = %v, want %v", info.Version, info.IsActive, wantActive)
				}

				// Verify path is set
				expectedPath := filepath.Join(agentsDir, tt.agentID, "versions", info.Version)
				if info.Path != expectedPath {
					t.Errorf("version %q path = %q, want %q", info.Version, info.Path, expectedPath)
				}
			}
		})
	}
}

func TestGetVersionHistory_WithFiles(t *testing.T) {
	// Test that files in versions directory are ignored (only directories are versions)
	agentsDir := t.TempDir()
	vm := NewVersionManager(agentsDir)
	agentID := "agent1"

	versionsDir := filepath.Join(agentsDir, agentID, "versions")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatalf("failed to create versions directory: %v", err)
	}

	// Create version directories
	for _, version := range []string{"v1", "v2"} {
		versionDir := filepath.Join(versionsDir, version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatalf("failed to create version directory %s: %v", version, err)
		}
	}

	// Create a file in versions directory (should be ignored)
	filePath := filepath.Join(versionsDir, "README.md")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	history, err := vm.GetVersionHistory(agentID)
	if err != nil {
		t.Fatalf("GetVersionHistory() error = %v", err)
	}

	// Should only have 2 versions (directories), not the file
	if len(history) != 2 {
		t.Errorf("GetVersionHistory() returned %d versions, want 2 (file should be ignored)", len(history))
	}
}

func TestListSortedVersions(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		setupVersions []string
		wantOrder     []string // expected order after sorting
	}{
		{
			name:          "numeric sorting v1 < v2 < v10",
			agentID:       "agent1",
			setupVersions: []string{"v10", "v2", "v1"},
			wantOrder:     []string{"v1", "v2", "v10"},
		},
		{
			name:          "candidate versions sorted by number",
			agentID:       "agent2",
			setupVersions: []string{"v3-candidate", "v1", "v2"},
			wantOrder:     []string{"v1", "v2", "v3-candidate"},
		},
		{
			name:          "rolledback versions sorted by number",
			agentID:       "agent3",
			setupVersions: []string{"v2", "v1", "v3-rolledback"},
			wantOrder:     []string{"v1", "v2", "v3-rolledback"},
		},
		{
			name:          "mixed versions",
			agentID:       "agent4",
			setupVersions: []string{"v5", "v3-candidate", "v1", "v4-rolledback", "v2"},
			wantOrder:     []string{"v1", "v2", "v3-candidate", "v4-rolledback", "v5"},
		},
		{
			name:          "single version",
			agentID:       "agent5",
			setupVersions: []string{"v1"},
			wantOrder:     []string{"v1"},
		},
		{
			name:          "empty directory",
			agentID:       "agent6",
			setupVersions: []string{},
			wantOrder:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			vm := NewVersionManager(agentsDir)

			// Setup versions directory structure
			versionsDir := filepath.Join(agentsDir, tt.agentID, "versions")
			if err := os.MkdirAll(versionsDir, 0755); err != nil {
				t.Fatalf("failed to create versions directory: %v", err)
			}

			for _, version := range tt.setupVersions {
				versionDir := filepath.Join(versionsDir, version)
				if err := os.MkdirAll(versionDir, 0755); err != nil {
					t.Fatalf("failed to create version directory %s: %v", version, err)
				}
			}

			// Execute
			got, err := vm.listSortedVersions(tt.agentID)
			if err != nil {
				t.Fatalf("listSortedVersions() error = %v", err)
			}

			// Verify count
			if len(got) != len(tt.wantOrder) {
				t.Errorf("listSortedVersions() returned %d versions, want %d", len(got), len(tt.wantOrder))
			}

			// Verify order
			for i, wantVersion := range tt.wantOrder {
				if i >= len(got) {
					break
				}
				if got[i].raw != wantVersion {
					t.Errorf("listSortedVersions()[%d] = %q, want %q", i, got[i].raw, wantVersion)
				}
			}
		})
	}
}

func TestExtractVersionNumber(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{"v1", 1},
		{"v2", 2},
		{"v10", 10},
		{"v100", 100},
		{"v3-candidate", 3},
		{"v4-rolledback", 4},
		{"v5-anything", 5},
		{"invalid", 0},
		{"v", 0},
		{"", 0},
		{"v0", 0},
		{"vABC", 0},
		{"v12abc", 12},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := extractVersionNumber(tt.version)
			if got != tt.want {
				t.Errorf("extractVersionNumber(%q) = %d, want %d", tt.version, got, tt.want)
			}
		})
	}
}

func TestRollback_DirectoryRenameSuccess(t *testing.T) {
	// Specific test to verify the v3 directory is renamed to v3-rolledback
	agentsDir := t.TempDir()
	vm := NewVersionManager(agentsDir)
	agentID := "agent1"

	versionsDir := filepath.Join(agentsDir, agentID, "versions")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatalf("failed to create versions directory: %v", err)
	}

	// Create v1, v2, v3
	for _, version := range []string{"v1", "v2", "v3"} {
		versionDir := filepath.Join(versionsDir, version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatalf("failed to create version directory %s: %v", version, err)
		}
	}

	// Rollback from v3 to v2
	rolledBackTo, err := vm.Rollback(agentID)
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	if rolledBackTo != "v2" {
		t.Errorf("Rollback() = %q, want v2", rolledBackTo)
	}

	// Verify v3 is now v3-rolledback
	v3Dir := filepath.Join(versionsDir, "v3")
	if _, err := os.Stat(v3Dir); !os.IsNotExist(err) {
		t.Errorf("v3 directory still exists after rollback")
	}

	v3RolledbackDir := filepath.Join(versionsDir, "v3-rolledback")
	if _, err := os.Stat(v3RolledbackDir); os.IsNotExist(err) {
		t.Errorf("v3-rolledback directory does not exist after rollback")
	}

	// Verify v2 still exists
	v2Dir := filepath.Join(versionsDir, "v2")
	if _, err := os.Stat(v2Dir); os.IsNotExist(err) {
		t.Errorf("v2 directory does not exist after rollback")
	}
}

func TestPromoteCandidate_DirectoryRenameSuccess(t *testing.T) {
	// Specific test to verify the v2-candidate directory is renamed to v2
	agentsDir := t.TempDir()
	vm := NewVersionManager(agentsDir)
	agentID := "agent1"

	versionsDir := filepath.Join(agentsDir, agentID, "versions")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatalf("failed to create versions directory: %v", err)
	}

	// Create v1, v2-candidate
	for _, version := range []string{"v1", "v2-candidate"} {
		versionDir := filepath.Join(versionsDir, version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatalf("failed to create version directory %s: %v", version, err)
		}
	}

	// Promote v2-candidate to v2
	promoted, err := vm.PromoteCandidate(agentID)
	if err != nil {
		t.Fatalf("PromoteCandidate() error = %v", err)
	}

	if promoted != "v2" {
		t.Errorf("PromoteCandidate() = %q, want v2", promoted)
	}

	// Verify v2-candidate is now v2
	v2CandidateDir := filepath.Join(versionsDir, "v2-candidate")
	if _, err := os.Stat(v2CandidateDir); !os.IsNotExist(err) {
		t.Errorf("v2-candidate directory still exists after promotion")
	}

	v2Dir := filepath.Join(versionsDir, "v2")
	if _, err := os.Stat(v2Dir); os.IsNotExist(err) {
		t.Errorf("v2 directory does not exist after promotion")
	}

	// Verify v1 still exists
	v1Dir := filepath.Join(versionsDir, "v1")
	if _, err := os.Stat(v1Dir); os.IsNotExist(err) {
		t.Errorf("v1 directory does not exist after promotion")
	}
}
