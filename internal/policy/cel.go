package policy

import (
	"fmt"
	"log/slog"

	"github.com/google/cel-go/cel"
)

// CompiledRule wraps a pre-compiled CEL program for fast repeated evaluation.
type CompiledRule struct {
	Expression string
	program    cel.Program
}

// CELEvaluator compiles and evaluates CEL expressions against ActionContext
// values. Expressions are compiled once at load time; evaluation is lock-free
// and safe for concurrent use.
type CELEvaluator struct {
	env    *cel.Env
	logger *slog.Logger
}

// NewCELEvaluator creates a CELEvaluator with the standard variable
// declarations available in policy conditions.
func NewCELEvaluator(logger *slog.Logger) (*CELEvaluator, error) {
	if logger == nil {
		logger = slog.Default()
	}

	env, err := cel.NewEnv(
		// action.*
		cel.Variable("action.type", cel.StringType),
		cel.Variable("action.name", cel.StringType),
		cel.Variable("action.params", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("action.target", cel.StringType),

		// session.*
		cel.Variable("session.id", cel.StringType),
		cel.Variable("session.agent_id", cel.StringType),
		cel.Variable("session.cost", cel.DoubleType),
		cel.Variable("session.action_count", cel.IntType),

		// agent.*
		cel.Variable("agent.id", cel.StringType),
		cel.Variable("agent.name", cel.StringType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &CELEvaluator{
		env:    env,
		logger: logger.With("component", "policy.CELEvaluator"),
	}, nil
}

// CompileExpression parses and type-checks a CEL expression, returning a
// CompiledRule ready for evaluation. This should be called at load time, not
// in the hot path.
func (c *CELEvaluator) CompileExpression(expr string) (CompiledRule, error) {
	ast, issues := c.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return CompiledRule{}, fmt.Errorf("CEL compile error in %q: %w", expr, issues.Err())
	}

	// Ensure the expression evaluates to a boolean.
	if ast.OutputType() != cel.BoolType {
		return CompiledRule{}, fmt.Errorf("CEL expression %q must evaluate to bool, got %s", expr, ast.OutputType())
	}

	prg, err := c.env.Program(ast)
	if err != nil {
		return CompiledRule{}, fmt.Errorf("CEL program creation failed for %q: %w", expr, err)
	}

	c.logger.Debug("compiled CEL expression", "expression", expr)

	return CompiledRule{
		Expression: expr,
		program:    prg,
	}, nil
}

// Evaluate runs a pre-compiled CEL rule against the given ActionContext.
// Returns true if the condition matches (i.e. the policy should fire).
func (c *CELEvaluator) Evaluate(rule CompiledRule, ctx ActionContext) (bool, error) {
	vars := map[string]interface{}{
		"action.type":   ctx.Action.Type,
		"action.name":   ctx.Action.Name,
		"action.params": ctx.Action.Params,
		"action.target": ctx.Action.Target,

		"session.id":           ctx.Session.ID,
		"session.agent_id":     ctx.Session.AgentID,
		"session.cost":         ctx.Session.Cost,
		"session.action_count": int64(ctx.Session.ActionCount),

		"agent.id":   ctx.Agent.ID,
		"agent.name": ctx.Agent.Name,
	}

	// Ensure params map is never nil -- CEL map access on nil panics.
	if vars["action.params"] == nil {
		vars["action.params"] = map[string]interface{}{}
	}

	out, _, err := rule.program.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("CEL evaluation error for %q: %w", rule.Expression, err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression %q returned non-bool: %T", rule.Expression, out.Value())
	}

	return result, nil
}
