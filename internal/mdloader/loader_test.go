package mdloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewLoader verifies basic loader initialization.
func TestNewLoader(t *testing.T) {
	t.Run("creates loader with correct paths", func(t *testing.T) {
		l := NewLoader("agents", "policies", "playbooks")
		if l.AgentsDir() != "agents" {
			t.Errorf("expected agentsDir=agents, got %s", l.AgentsDir())
		}
		if l.PoliciesDir() != "policies" {
			t.Errorf("expected policiesDir=policies, got %s", l.PoliciesDir())
		}
		if l.PlaybooksDir() != "playbooks" {
			t.Errorf("expected playbooksDir=playbooks, got %s", l.PlaybooksDir())
		}
		if l.cache == nil {
			t.Errorf("cache should be initialized")
		}
	})
}

// TestLoadAgentMD tests loading AGENT.md files.
func TestLoadAgentMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	t.Run("loads existing file", func(t *testing.T) {
		agentID := "test-agent"
		agentDir := filepath.Join(agentsDir, agentID)
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Test Agent\nThis is a test agent."
		if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadAgentMD(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := l.LoadAgentMD("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestLoadEvolveMD tests loading EVOLVE.md files.
func TestLoadEvolveMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	t.Run("loads existing file", func(t *testing.T) {
		agentID := "test-agent"
		agentDir := filepath.Join(agentsDir, agentID)
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Evolution Rules\nMax iterations: 100"
		if err := os.WriteFile(filepath.Join(agentDir, "EVOLVE.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadEvolveMD(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := l.LoadEvolveMD("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestLoadPromptMD tests loading versioned PROMPT.md files.
func TestLoadPromptMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	t.Run("loads existing versioned prompt", func(t *testing.T) {
		agentID := "test-agent"
		version := "v1"
		promptDir := filepath.Join(agentsDir, agentID, "versions", version)
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "You are a helpful AI assistant."
		if err := os.WriteFile(filepath.Join(promptDir, "PROMPT.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadPromptMD(agentID, version)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("returns error for missing version", func(t *testing.T) {
		_, err := l.LoadPromptMD("test-agent", "v99")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestLoadPolicyMD tests loading POLICY.md files.
func TestLoadPolicyMD(t *testing.T) {
	tmpDir := t.TempDir()
	policiesDir := filepath.Join(tmpDir, "policies")

	l := NewLoader("", policiesDir, "")

	t.Run("loads existing policy", func(t *testing.T) {
		policyPath := "budget-check"
		policyDir := filepath.Join(policiesDir, policyPath)
		if err := os.MkdirAll(policyDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Budget Policy\nLimit: $100/day"
		if err := os.WriteFile(filepath.Join(policyDir, "POLICY.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadPolicyMD(policyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("returns error for missing policy", func(t *testing.T) {
		_, err := l.LoadPolicyMD("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestLoadPlaybook tests loading playbook files.
func TestLoadPlaybook(t *testing.T) {
	tmpDir := t.TempDir()
	playbooksDir := filepath.Join(tmpDir, "playbooks")

	l := NewLoader("", "", playbooksDir)

	t.Run("loads playbook with uppercase conversion", func(t *testing.T) {
		if err := os.MkdirAll(playbooksDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Loop Detection Playbook\nAlert after 5 repetitions."
		if err := os.WriteFile(filepath.Join(playbooksDir, "LOOP.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Test with lowercase name (should be converted to uppercase)
		got, err := l.LoadPlaybook("loop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("loads playbook with uppercase name", func(t *testing.T) {
		if err := os.MkdirAll(playbooksDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Spiral Detection Playbook"
		if err := os.WriteFile(filepath.Join(playbooksDir, "SPIRAL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadPlaybook("SPIRAL")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}
	})

	t.Run("returns error for missing playbook", func(t *testing.T) {
		_, err := l.LoadPlaybook("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestCaching verifies that the loader caches file content.
func TestCaching(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	agentID := "cached-agent"
	agentDir := filepath.Join(agentsDir, agentID)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(agentDir, "AGENT.md")
	content := "# Original Content"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("first load reads from disk", func(t *testing.T) {
		got, err := l.LoadAgentMD(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected content %q, got %q", content, got)
		}

		// Verify cache was populated
		abs, _ := filepath.Abs(filePath)
		l.mu.RLock()
		cached, ok := l.cache[abs]
		l.mu.RUnlock()

		if !ok {
			t.Fatal("expected file to be cached")
		}
		if cached.Content != content {
			t.Errorf("cached content mismatch: expected %q, got %q", content, cached.Content)
		}
	})

	t.Run("second load returns cached content", func(t *testing.T) {
		// Load again without modifying the file
		got, err := l.LoadAgentMD(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("expected cached content %q, got %q", content, got)
		}
	})

	t.Run("modified file invalidates cache", func(t *testing.T) {
		// Wait to ensure modtime changes
		time.Sleep(10 * time.Millisecond)

		newContent := "# Modified Content"
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.LoadAgentMD(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != newContent {
			t.Errorf("expected new content %q, got %q", newContent, got)
		}
	})
}

// TestInvalidate tests cache invalidation.
func TestInvalidate(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	agentID := "test-agent"
	agentDir := filepath.Join(agentsDir, agentID)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(agentDir, "AGENT.md")
	content := "# Test"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("invalidate removes cached entry", func(t *testing.T) {
		// Load to populate cache
		if _, err := l.LoadAgentMD(agentID); err != nil {
			t.Fatal(err)
		}

		// Verify cache is populated
		abs, _ := filepath.Abs(filePath)
		l.mu.RLock()
		_, ok := l.cache[abs]
		l.mu.RUnlock()
		if !ok {
			t.Fatal("expected file to be cached")
		}

		// Invalidate
		l.Invalidate(filePath)

		// Verify cache is cleared
		l.mu.RLock()
		_, ok = l.cache[abs]
		l.mu.RUnlock()
		if ok {
			t.Fatal("expected cache entry to be removed")
		}
	})
}

// TestInvalidateAll tests clearing the entire cache.
func TestInvalidateAll(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	// Create and load multiple files
	for i := 1; i <= 3; i++ {
		agentID := filepath.Join("agent", string(rune('0'+i)))
		agentDir := filepath.Join(agentsDir, agentID)
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			t.Fatal(err)
		}

		content := "# Agent"
		if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if _, err := l.LoadAgentMD(agentID); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("clears entire cache", func(t *testing.T) {
		// Verify cache has entries
		l.mu.RLock()
		cacheSize := len(l.cache)
		l.mu.RUnlock()
		if cacheSize == 0 {
			t.Fatal("expected cache to have entries")
		}

		// Invalidate all
		l.InvalidateAll()

		// Verify cache is empty
		l.mu.RLock()
		cacheSize = len(l.cache)
		l.mu.RUnlock()
		if cacheSize != 0 {
			t.Errorf("expected cache to be empty, got %d entries", cacheSize)
		}
	})
}

// TestListVersions tests version directory listing.
func TestListVersions(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	agentID := "versioned-agent"
	versionsDir := filepath.Join(agentsDir, agentID, "versions")

	t.Run("lists sorted versions", func(t *testing.T) {
		// Create version directories in random order
		versions := []string{"v3", "v1", "v2-candidate", "v2", "v10"}
		for _, v := range versions {
			dir := filepath.Join(versionsDir, v)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}

		got, err := l.ListVersions(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected order: v1, v2, v2-candidate, v3, v10
		expected := []string{"v1", "v2", "v2-candidate", "v3", "v10"}
		if len(got) != len(expected) {
			t.Fatalf("expected %d versions, got %d", len(expected), len(got))
		}

		for i, want := range expected {
			if got[i] != want {
				t.Errorf("position %d: expected %q, got %q", i, want, got[i])
			}
		}
	})

	t.Run("ignores non-version directories", func(t *testing.T) {
		// Create non-version directory
		if err := os.MkdirAll(filepath.Join(versionsDir, "not-a-version"), 0755); err != nil {
			t.Fatal(err)
		}

		got, err := l.ListVersions(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have valid versions
		for _, v := range got {
			if v == "not-a-version" {
				t.Errorf("non-version directory should be filtered out: %q", v)
			}
		}
	})

	t.Run("ignores files in versions directory", func(t *testing.T) {
		// Create a file in versions directory
		filePath := filepath.Join(versionsDir, "README.md")
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := l.ListVersions(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have directories
		for _, v := range got {
			if v == "README.md" {
				t.Errorf("files should be filtered out: %q", v)
			}
		}
	})

	t.Run("returns error for missing versions directory", func(t *testing.T) {
		_, err := l.ListVersions("nonexistent-agent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// TestCurrentVersion tests finding the current released version.
func TestCurrentVersion(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	agentID := "versioned-agent"
	versionsDir := filepath.Join(agentsDir, agentID, "versions")

	t.Run("returns highest non-candidate version", func(t *testing.T) {
		versions := []string{"v1", "v2", "v3-candidate", "v4", "v5-candidate"}
		for _, v := range versions {
			dir := filepath.Join(versionsDir, v)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}

		got, err := l.CurrentVersion(agentID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "v4" {
			t.Errorf("expected v4, got %q", got)
		}
	})

	t.Run("skips candidates", func(t *testing.T) {
		// Clean up and create only candidates
		if err := os.RemoveAll(versionsDir); err != nil {
			t.Fatal(err)
		}

		versions := []string{"v1-candidate", "v2-candidate"}
		for _, v := range versions {
			dir := filepath.Join(versionsDir, v)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}

		_, err := l.CurrentVersion(agentID)
		if err == nil {
			t.Fatal("expected error for agent with only candidate versions")
		}
	})

	t.Run("returns error when no versions exist", func(t *testing.T) {
		// Clean up all versions
		if err := os.RemoveAll(versionsDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(versionsDir, 0755); err != nil {
			t.Fatal(err)
		}

		_, err := l.CurrentVersion(agentID)
		if err == nil {
			t.Fatal("expected error for agent with no versions")
		}
	})
}

// TestVersionSortKey tests the version sorting logic.
func TestVersionSortKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"v1", "v1", 2},
		{"v2", "v2", 4},
		{"v10", "v10", 20},
		{"v1-candidate", "v1-candidate", 3},
		{"v2-candidate", "v2-candidate", 5},
		{"v10-candidate", "v10-candidate", 21},
		{"invalid", "invalid", -1},
		{"not-version", "v", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionSortKey(tt.input)
			if got != tt.expected {
				t.Errorf("versionSortKey(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}

	t.Run("candidates sort after released", func(t *testing.T) {
		v1 := versionSortKey("v1")
		v1Candidate := versionSortKey("v1-candidate")
		if v1 >= v1Candidate {
			t.Errorf("v1 (%d) should sort before v1-candidate (%d)", v1, v1Candidate)
		}
	})

	t.Run("versions sort numerically", func(t *testing.T) {
		v2 := versionSortKey("v2")
		v10 := versionSortKey("v10")
		if v2 >= v10 {
			t.Errorf("v2 (%d) should sort before v10 (%d)", v2, v10)
		}
	})
}

// TestSetWatcher tests watcher association.
func TestSetWatcher(t *testing.T) {
	l := NewLoader("agents", "policies", "playbooks")

	t.Run("sets watcher", func(t *testing.T) {
		// Create a minimal watcher (we don't need to start it)
		w := &Watcher{}
		l.SetWatcher(w)

		l.mu.RLock()
		gotWatcher := l.watcher
		l.mu.RUnlock()

		if gotWatcher != w {
			t.Errorf("expected watcher to be set")
		}
	})
}

// TestConcurrentAccess verifies thread-safety of the loader.
func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	l := NewLoader(agentsDir, "", "")

	// Create test agent
	agentID := "concurrent-agent"
	agentDir := filepath.Join(agentsDir, agentID)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := "# Concurrent Test"
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("concurrent reads", func(t *testing.T) {
		const numReaders = 10
		done := make(chan bool, numReaders)

		for i := 0; i < numReaders; i++ {
			go func() {
				_, err := l.LoadAgentMD(agentID)
				if err != nil {
					t.Errorf("concurrent read error: %v", err)
				}
				done <- true
			}()
		}

		for i := 0; i < numReaders; i++ {
			<-done
		}
	})

	t.Run("concurrent invalidations", func(t *testing.T) {
		const numInvalidators = 10
		done := make(chan bool, numInvalidators)

		filePath := filepath.Join(agentDir, "AGENT.md")

		for i := 0; i < numInvalidators; i++ {
			go func() {
				l.Invalidate(filePath)
				done <- true
			}()
		}

		for i := 0; i < numInvalidators; i++ {
			<-done
		}
	})

	t.Run("concurrent reads and invalidations", func(t *testing.T) {
		const numOps = 20
		done := make(chan bool, numOps)

		filePath := filepath.Join(agentDir, "AGENT.md")

		for i := 0; i < numOps; i++ {
			if i%2 == 0 {
				go func() {
					_, _ = l.LoadAgentMD(agentID)
					done <- true
				}()
			} else {
				go func() {
					l.Invalidate(filePath)
					done <- true
				}()
			}
		}

		for i := 0; i < numOps; i++ {
			<-done
		}
	})
}
