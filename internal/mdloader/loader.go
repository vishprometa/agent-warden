// Package mdloader reads, caches, and watches Markdown files that serve as
// "config files for cognition" — consumed by LLMs in the evolution engine
// (AGENT.md, EVOLVE.md, PROMPT.md), AI-judge policy evaluator (POLICY.md),
// and detection playbook executor (playbooks/*.md).
package mdloader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// versionDirPattern matches version directories like "v1", "v2", "v42".
var versionDirPattern = regexp.MustCompile(`^v(\d+)$`)

// candidateSuffix marks a version directory as an in-progress candidate
// (e.g. "v3-candidate") that should not be returned by CurrentVersion.
const candidateSuffix = "-candidate"

// Loader reads and caches MD files from the filesystem. It is safe for
// concurrent use. The cache is keyed by absolute file path and entries are
// automatically invalidated when the Watcher detects a filesystem change.
type Loader struct {
	agentsDir    string // e.g. "./agents"
	policiesDir  string // e.g. "./policies"
	playbooksDir string // e.g. "./playbooks"
	cache        map[string]*CachedMD
	mu           sync.RWMutex
	watcher      *Watcher
}

// CachedMD holds a single cached Markdown file and its metadata.
type CachedMD struct {
	Path     string
	Content  string
	ModTime  time.Time
	LoadedAt time.Time
}

// NewLoader creates a new Loader for the given directory layout. The
// directories do not need to exist at construction time — they are checked
// on each load call.
func NewLoader(agentsDir, policiesDir, playbooksDir string) *Loader {
	return &Loader{
		agentsDir:    agentsDir,
		policiesDir:  policiesDir,
		playbooksDir: playbooksDir,
		cache:        make(map[string]*CachedMD),
	}
}

// AgentsDir returns the configured agents directory.
func (l *Loader) AgentsDir() string { return l.agentsDir }

// PoliciesDir returns the configured policies directory.
func (l *Loader) PoliciesDir() string { return l.policiesDir }

// PlaybooksDir returns the configured playbooks directory.
func (l *Loader) PlaybooksDir() string { return l.playbooksDir }

// SetWatcher associates a filesystem Watcher with this Loader. The watcher
// calls Invalidate on file changes. This is called by NewWatcher automatically.
func (l *Loader) SetWatcher(w *Watcher) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.watcher = w
}

// ---------------------------------------------------------------------------
// Agent files
// ---------------------------------------------------------------------------

// LoadAgentMD loads agents/<agentID>/AGENT.md.
func (l *Loader) LoadAgentMD(agentID string) (string, error) {
	p := filepath.Join(l.agentsDir, agentID, "AGENT.md")
	return l.loadFile(p)
}

// LoadEvolveMD loads agents/<agentID>/EVOLVE.md.
func (l *Loader) LoadEvolveMD(agentID string) (string, error) {
	p := filepath.Join(l.agentsDir, agentID, "EVOLVE.md")
	return l.loadFile(p)
}

// LoadPromptMD loads agents/<agentID>/versions/<version>/PROMPT.md.
func (l *Loader) LoadPromptMD(agentID string, version string) (string, error) {
	p := filepath.Join(l.agentsDir, agentID, "versions", version, "PROMPT.md")
	return l.loadFile(p)
}

// CurrentVersion scans agents/<agentID>/versions/ and returns the highest
// numbered version directory (e.g. "v3") that is NOT a candidate. Returns
// an error if no valid version directories exist.
func (l *Loader) CurrentVersion(agentID string) (string, error) {
	versions, err := l.listVersionEntries(agentID)
	if err != nil {
		return "", err
	}

	// Walk backwards to find the highest non-candidate version.
	for i := len(versions) - 1; i >= 0; i-- {
		name := versions[i]
		if !strings.HasSuffix(name, candidateSuffix) && versionDirPattern.MatchString(name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no released version found for agent %q", agentID)
}

// ListVersions returns all version directory names for an agent, sorted
// by version number (v1, v2, v2-candidate, v3, v3-candidate, ...).
func (l *Loader) ListVersions(agentID string) ([]string, error) {
	return l.listVersionEntries(agentID)
}

// listVersionEntries reads the versions directory and returns sorted dir names.
func (l *Loader) listVersionEntries(agentID string) ([]string, error) {
	dir := filepath.Join(l.agentsDir, agentID, "versions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read versions directory for agent %q: %w", agentID, err)
	}

	var versions []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Accept vN and vN-candidate directories.
		if versionDirPattern.MatchString(name) ||
			(strings.HasSuffix(name, candidateSuffix) &&
				versionDirPattern.MatchString(strings.TrimSuffix(name, candidateSuffix))) {
			versions = append(versions, name)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versionSortKey(versions[i]) < versionSortKey(versions[j])
	})

	return versions, nil
}

// versionSortKey produces an integer sort key from "v3" or "v3-candidate".
// Released versions sort before candidates of the same number.
func versionSortKey(name string) int {
	clean := strings.TrimSuffix(name, candidateSuffix)
	m := versionDirPattern.FindStringSubmatch(clean)
	if m == nil {
		return -1
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	// Candidates sort after the released version of the same number.
	if strings.HasSuffix(name, candidateSuffix) {
		return n*2 + 1
	}
	return n * 2
}

// ---------------------------------------------------------------------------
// Policy files
// ---------------------------------------------------------------------------

// LoadPolicyMD loads policies/<policyPath>/POLICY.md. The policyPath is
// typically the policy name from the config.
func (l *Loader) LoadPolicyMD(policyPath string) (string, error) {
	p := filepath.Join(l.policiesDir, policyPath, "POLICY.md")
	return l.loadFile(p)
}

// ---------------------------------------------------------------------------
// Playbook files
// ---------------------------------------------------------------------------

// LoadPlaybook loads playbooks/<NAME>.md. The name is uppercased before
// looking up the file (e.g. "loop" -> "playbooks/LOOP.md").
func (l *Loader) LoadPlaybook(name string) (string, error) {
	filename := strings.ToUpper(name) + ".md"
	p := filepath.Join(l.playbooksDir, filename)
	return l.loadFile(p)
}

// ---------------------------------------------------------------------------
// Cache management
// ---------------------------------------------------------------------------

// Invalidate removes a cached entry by its absolute or relative path. Called
// by the Watcher on filesystem change events.
func (l *Loader) Invalidate(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, abs)
}

// InvalidateAll clears the entire cache.
func (l *Loader) InvalidateAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = make(map[string]*CachedMD)
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

// loadFile returns the file content from cache if the file has not been
// modified since it was cached, otherwise reads from disk and updates the
// cache.
func (l *Loader) loadFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %q: %w", path, err)
	}

	// Stat the file to check existence and mod time.
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("file not found: %s", abs)
	}

	// Fast path: return cached content if the file has not changed.
	l.mu.RLock()
	cached, ok := l.cache[abs]
	l.mu.RUnlock()

	if ok && !info.ModTime().After(cached.ModTime) {
		return cached.Content, nil
	}

	// Slow path: read from disk.
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", abs, err)
	}

	entry := &CachedMD{
		Path:     abs,
		Content:  string(data),
		ModTime:  info.ModTime(),
		LoadedAt: time.Now(),
	}

	l.mu.Lock()
	l.cache[abs] = entry
	l.mu.Unlock()

	return entry.Content, nil
}
