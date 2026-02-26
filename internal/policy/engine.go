// Package policy implements the governance policy evaluation pipeline for
// AgentWarden. Policies are evaluated in a strict order: budget checks, rate
// limits, deterministic CEL rules, AI-evaluated judgments, and finally approval
// gates. The first deny or terminate result short-circuits the pipeline.
package policy

import (
	"log/slog"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// Effect constants match the values used in config.PolicyConfig.Effect.
const (
	EffectAllow     = "allow"
	EffectDeny      = "deny"
	EffectTerminate = "terminate"
	EffectThrottle  = "throttle"
	EffectApprove   = "approve"
)

// ActionContext holds all the information the policy engine needs to evaluate
// a single agent action. It is assembled by the proxy layer before calling
// Engine.Evaluate.
type ActionContext struct {
	Action   ActionInfo
	Session  SessionInfo
	Agent    AgentInfo
	Request  RequestInfo
	Metadata map[string]interface{}
}

// ActionInfo describes the action being performed.
type ActionInfo struct {
	Type   string                 // e.g. "llm.chat", "tool.call", "db.query"
	Name   string                 // human-readable action name
	Params map[string]interface{} // action-specific parameters
	Target string                 // resource being acted upon
}

// SessionInfo provides session-level context for policy evaluation.
type SessionInfo struct {
	ID          string
	AgentID     string
	Cost        float64
	ActionCount int

	// ActionCountByType returns the number of actions of the given type
	// within the specified sliding window (e.g. "60s", "5m"). The engine
	// calls this to support rate-limit CEL expressions like
	//   session.action_count("api.call", "60s") > 100
	// The function is provided by the caller (typically backed by
	// session.Manager.GetActionCount or RateLimiter.GetCount).
	ActionCountByType func(actionType, window string) int
}

// AgentInfo identifies the agent performing the action.
type AgentInfo struct {
	ID        string
	Name      string
	DailyCost float64 // cumulative cost for the agent today (across all sessions)
}

// RequestInfo holds raw HTTP request details for policies that inspect
// request content directly.
type RequestInfo struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// PolicyResult is the outcome of evaluating all policies against an action.
type PolicyResult struct {
	Effect     string        // allow, deny, terminate, throttle, approve
	PolicyName string        // name of the policy that triggered, empty if allowed
	Message    string        // human-readable explanation
	Delay      time.Duration // non-zero only for throttle effects
}

// Engine is the policy evaluation orchestrator. It holds a compiled, ordered
// set of policies and evaluates each action against them. The evaluation
// pipeline is: budget -> rate limit -> deterministic (CEL) -> AI judge ->
// approval. The first deny/terminate short-circuits.
//
// Engine is safe for concurrent use. Policies can be hot-reloaded via
// LoadPolicies or ReloadPolicies without stopping traffic.
type Engine struct {
	mu       sync.RWMutex
	policies []CompiledPolicy
	loader   *Loader
	celEval  *CELEvaluator
	budget   *BudgetChecker
	logger   *slog.Logger

	// configLoader is an optional reference to the top-level config loader,
	// used by ReloadPolicies to re-read config from disk.
	configLoader *config.Loader
}

// NewEngine creates a policy Engine with the given sub-components.
// Call LoadPolicies to populate the engine with compiled policies.
func NewEngine(
	loader *Loader,
	celEval *CELEvaluator,
	budget *BudgetChecker,
	logger *slog.Logger,
) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		loader:  loader,
		celEval: celEval,
		budget:  budget,
		logger:  logger.With("component", "policy.Engine"),
	}
}

// SetConfigLoader sets an optional config.Loader reference so that
// ReloadPolicies can re-read policy configs from disk.
func (e *Engine) SetConfigLoader(cl *config.Loader) {
	e.mu.Lock()
	e.configLoader = cl
	e.mu.Unlock()
}

// LoadPolicies compiles the given policy configs and atomically replaces
// the engine's active policy set. This is safe to call while the engine
// is concurrently evaluating requests.
func (e *Engine) LoadPolicies(configs []config.PolicyConfig) error {
	compiled, err := e.loader.LoadFromConfig(configs)
	if err != nil {
		return err
	}

	e.mu.Lock()
	e.policies = compiled
	e.mu.Unlock()

	e.logger.Info("policies loaded into engine", "count", len(compiled))
	return nil
}

// ReloadPolicies re-reads the config file via the config.Loader and
// recompiles all policies. This is the method called by the fsnotify
// hot-reload callback.
func (e *Engine) ReloadPolicies() error {
	e.mu.RLock()
	cl := e.configLoader
	e.mu.RUnlock()

	if cl == nil {
		e.logger.Warn("ReloadPolicies called but no config loader is set")
		return nil
	}

	if err := cl.Reload(); err != nil {
		e.logger.Error("failed to reload config from disk", "error", err)
		return err
	}

	cfg := cl.Get()
	if err := e.LoadPolicies(cfg.Policies); err != nil {
		e.logger.Error("failed to compile reloaded policies", "error", err)
		return err
	}

	e.logger.Info("policies hot-reloaded successfully")
	return nil
}

// Evaluate runs the action context through all loaded policies in pipeline
// order and returns the result. The default result is EffectAllow.
//
// Pipeline order:
//  1. Budget policies (any policy whose CEL condition references session.cost
//     with a terminate/deny effect)
//  2. Rate limit policies
//  3. Generic CEL policies
//  4. AI-judge policies (not yet implemented -- logged and skipped)
//  5. Approval policies (not yet implemented -- logged and skipped)
//
// The pipeline short-circuits on the first deny or terminate. Throttle
// results are collected and the longest delay is returned.
func (e *Engine) Evaluate(ctx ActionContext) PolicyResult {
	e.mu.RLock()
	policies := e.policies
	e.mu.RUnlock()

	if len(policies) == 0 {
		return PolicyResult{Effect: EffectAllow}
	}

	var longestThrottle PolicyResult

	for _, p := range policies {
		result := e.evaluateOne(p, ctx)

		switch result.Effect {
		case EffectDeny, EffectTerminate:
			e.logger.Warn("policy triggered",
				"policy", result.PolicyName,
				"effect", result.Effect,
				"message", result.Message,
				"session_id", ctx.Session.ID,
				"action_type", ctx.Action.Type,
			)
			return result

		case EffectThrottle:
			e.logger.Info("throttle policy triggered",
				"policy", result.PolicyName,
				"delay", result.Delay,
				"session_id", ctx.Session.ID,
			)
			// Keep the longest throttle delay.
			if result.Delay > longestThrottle.Delay {
				longestThrottle = result
			}

		case EffectApprove:
			e.logger.Info("approval policy triggered",
				"policy", result.PolicyName,
				"session_id", ctx.Session.ID,
			)
			return result

		case EffectAllow:
			// Policy did not fire; continue to next.
		}
	}

	// If any throttle was triggered, return it.
	if longestThrottle.Delay > 0 {
		return longestThrottle
	}

	return PolicyResult{Effect: EffectAllow}
}

// evaluateOne runs a single compiled policy against the action context.
func (e *Engine) evaluateOne(p CompiledPolicy, ctx ActionContext) PolicyResult {
	switch p.Category {
	case CategoryCEL:
		return e.evaluateCEL(p, ctx)

	case CategoryAIJudge:
		// AI judge evaluation is a future extension point. For now, log
		// a warning and allow so that AI-judge policies in config don't
		// silently block traffic.
		e.logger.Warn("ai-judge policy evaluation not yet implemented, allowing — this policy has no effect",
			"policy", p.Config.Name,
		)
		return PolicyResult{Effect: EffectAllow}

	case CategoryApproval:
		// Approval gates require async human interaction. The engine
		// signals an "approve" effect so the proxy layer can park the
		// request and wait for resolution.
		return PolicyResult{
			Effect:     EffectApprove,
			PolicyName: p.Config.Name,
			Message:    p.Config.Message,
		}

	default:
		e.logger.Warn("unknown policy category, allowing",
			"policy", p.Config.Name,
			"category", string(p.Category),
		)
		return PolicyResult{Effect: EffectAllow}
	}
}

// evaluateCEL evaluates a CEL-based policy (budget, rate limit, or generic).
func (e *Engine) evaluateCEL(p CompiledPolicy, ctx ActionContext) PolicyResult {
	if p.CELRule == nil {
		e.logger.Error("CEL policy has nil compiled rule, failing closed",
			"policy", p.Config.Name,
		)
		return PolicyResult{
			Effect:     EffectDeny,
			PolicyName: p.Config.Name,
			Message:    "policy has nil compiled rule",
		}
	}

	matched, err := e.celEval.Evaluate(*p.CELRule, ctx)
	if err != nil {
		e.logger.Error("CEL evaluation error, failing closed (deny)",
			"policy", p.Config.Name,
			"error", err,
		)
		return PolicyResult{
			Effect:     EffectDeny,
			PolicyName: p.Config.Name,
			Message:    "policy evaluation error: " + err.Error(),
		}
	}

	if !matched {
		return PolicyResult{Effect: EffectAllow}
	}

	// The condition matched -- apply the configured effect.
	result := PolicyResult{
		Effect:     p.Config.Effect,
		PolicyName: p.Config.Name,
		Message:    p.Config.Message,
	}

	if p.Config.Effect == EffectThrottle {
		result.Delay = p.Config.Delay
	}

	return result
}

// PolicyCount returns the number of currently loaded policies.
func (e *Engine) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}
