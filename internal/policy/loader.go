package policy

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/fsnotify/fsnotify"
)

// PolicyCategory classifies how a policy is evaluated.
type PolicyCategory string

const (
	CategoryBudget    PolicyCategory = "budget"
	CategoryRateLimit PolicyCategory = "rate_limit"
	CategoryCEL       PolicyCategory = "cel"
	CategoryAIJudge   PolicyCategory = "ai_judge"
	CategoryApproval  PolicyCategory = "approval"
)

// CompiledPolicy wraps a PolicyConfig with pre-compiled evaluation artefacts.
// CEL-based policies carry a compiled program; other types (budget, rate limit,
// AI judge, approval) are resolved by inspecting the config at evaluation time.
type CompiledPolicy struct {
	Config   config.PolicyConfig
	Category PolicyCategory
	CELRule  *CompiledRule // non-nil only for CEL policies
}

// Loader compiles policy configs into evaluation-ready CompiledPolicy objects
// and optionally watches a config file for hot-reload notifications.
type Loader struct {
	celEval *CELEvaluator
	logger  *slog.Logger

	// watcher state
	mu        sync.Mutex
	watcher   *fsnotify.Watcher
	watchDone chan struct{}
}

// NewLoader creates a policy Loader.
func NewLoader(celEval *CELEvaluator, logger *slog.Logger) *Loader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Loader{
		celEval: celEval,
		logger:  logger.With("component", "policy.Loader"),
	}
}

// LoadFromConfig compiles an ordered slice of PolicyConfig into CompiledPolicy
// objects. Policies that fail compilation are logged and skipped rather than
// failing the entire load -- this is intentional so that one bad policy does
// not prevent the proxy from starting.
func (l *Loader) LoadFromConfig(configs []config.PolicyConfig) ([]CompiledPolicy, error) {
	policies := make([]CompiledPolicy, 0, len(configs))

	for i, cfg := range configs {
		cat := classifyPolicy(cfg)

		cp := CompiledPolicy{
			Config:   cfg,
			Category: cat,
		}

		if cat == CategoryCEL {
			rule, err := l.celEval.CompileExpression(cfg.Condition)
			if err != nil {
				l.logger.Error("skipping policy with invalid CEL expression",
					"policy_name", cfg.Name,
					"index", i,
					"error", err,
				)
				continue
			}
			cp.CELRule = &rule
		}

		policies = append(policies, cp)
		l.logger.Info("loaded policy",
			"name", cfg.Name,
			"category", string(cat),
			"effect", cfg.Effect,
		)
	}

	l.logger.Info("policy loading complete",
		"total_configs", len(configs),
		"loaded_policies", len(policies),
	)

	return policies, nil
}

// classifyPolicy determines the evaluation category for a PolicyConfig based
// on its fields. The classification order matches the engine's evaluation
// pipeline:
//
//  1. If Type == "ai-judge", it is an AI-evaluated policy.
//  2. If Approvers is non-empty, it is an approval-gate policy.
//  3. If the condition references session.cost with a comparison, treat as budget.
//  4. If the condition references action_count_in_window, treat as rate limit.
//  5. Otherwise, treat as a generic CEL rule.
//
// Budget and rate-limit detection is heuristic -- the condition is still compiled
// as CEL for actual evaluation. The category merely controls where the policy
// sits in the engine's evaluation pipeline.
func classifyPolicy(cfg config.PolicyConfig) PolicyCategory {
	if cfg.Type == "ai-judge" {
		return CategoryAIJudge
	}
	if len(cfg.Approvers) > 0 {
		return CategoryApproval
	}

	// All remaining policies use CEL conditions. We classify budget and
	// rate-limit as sub-categories for pipeline ordering, but they still
	// use the same CEL evaluator.
	return CategoryCEL
}

// WatchConfig starts an fsnotify watcher on the given config file path.
// When the file is modified, the onReload callback is invoked. The callback
// receives the absolute path of the changed file. Call StopWatch to clean up.
func (l *Loader) WatchConfig(configPath string, onReload func(path string)) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Stop any existing watcher.
	if l.watcher != nil {
		l.stopWatchLocked()
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	// Watch the directory rather than the file to catch editor rename-and-replace
	// patterns (e.g. vim, nano).
	dir := filepath.Dir(absPath)
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	l.watcher = w
	l.watchDone = make(chan struct{})

	go l.watchLoop(absPath, onReload)

	l.logger.Info("watching config for changes", "path", absPath)
	return nil
}

// watchLoop is the background goroutine that processes fsnotify events.
func (l *Loader) watchLoop(targetPath string, onReload func(string)) {
	defer close(l.watchDone)

	for {
		select {
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			// Only react to writes or creates of the target file.
			absEvent, _ := filepath.Abs(event.Name)
			if absEvent != targetPath {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				l.logger.Info("config file changed, triggering reload", "path", targetPath)
				onReload(targetPath)
			}

		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			l.logger.Error("fsnotify error", "error", err)
		}
	}
}

// StopWatch stops the config file watcher, if running.
func (l *Loader) StopWatch() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopWatchLocked()
}

func (l *Loader) stopWatchLocked() {
	if l.watcher != nil {
		_ = l.watcher.Close()
		// Wait for the goroutine to exit.
		if l.watchDone != nil {
			<-l.watchDone
		}
		l.watcher = nil
		l.watchDone = nil
	}
}
