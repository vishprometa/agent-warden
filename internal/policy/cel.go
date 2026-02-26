package policy

import (
	"fmt"
	"log/slog"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
)

// CompiledRule wraps a pre-compiled CEL AST for repeated evaluation.
// If the expression uses the action_count_in_window function, a new
// cel.Program is created per evaluation to bind the function to the
// current ActionContext. Otherwise the program is pre-built and reused.
type CompiledRule struct {
	Expression string
	ast        *cel.Ast
	program    cel.Program // nil if expression uses dynamic functions
	usesDynFn  bool
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
		cel.Variable("agent.daily_cost", cel.DoubleType),

		// action_count_in_window(actionType, window) returns the number of
		// actions of the given type within the specified sliding window
		// (e.g. "60s", "5m"). The function binding is provided at program
		// creation time so it can capture the per-evaluation ActionContext.
		cel.Function("action_count_in_window",
			cel.Overload("action_count_in_window_string_string",
				[]*cel.Type{cel.StringType, cel.StringType},
				cel.IntType,
			),
		),
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

	rule := CompiledRule{
		Expression: expr,
		ast:        ast,
	}

	// Check if the expression references the dynamic function. If not, we
	// can create the program once and reuse it (faster).
	usesDynFn := containsFunc(expr, "action_count_in_window")
	if usesDynFn {
		rule.usesDynFn = true
	} else {
		prg, err := c.env.Program(ast)
		if err != nil {
			return CompiledRule{}, fmt.Errorf("CEL program creation failed for %q: %w", expr, err)
		}
		rule.program = prg
	}

	c.logger.Debug("compiled CEL expression", "expression", expr, "uses_dynamic_fn", usesDynFn)

	return rule, nil
}

// containsFunc is a simple check for whether an expression string references
// a function name. This is a heuristic used to decide whether to pre-build
// the program or defer to per-evaluation program creation.
func containsFunc(expr, funcName string) bool {
	// Simple substring check — the function name followed by "(" is sufficient.
	for i := 0; i <= len(expr)-len(funcName); i++ {
		if expr[i:i+len(funcName)] == funcName {
			return true
		}
	}
	return false
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

		"agent.id":         ctx.Agent.ID,
		"agent.name":       ctx.Agent.Name,
		"agent.daily_cost": ctx.Agent.DailyCost,
	}

	// Ensure params map is never nil -- CEL map access on nil panics.
	if vars["action.params"] == nil {
		vars["action.params"] = map[string]interface{}{}
	}

	var prg cel.Program
	if rule.usesDynFn {
		// Build a function binding that captures this evaluation's context.
		countFn := func(args ...ref.Val) ref.Val {
			if len(args) != 2 {
				return types.NewErr("action_count_in_window requires 2 arguments")
			}
			actionType, ok1 := args[0].Value().(string)
			window, ok2 := args[1].Value().(string)
			if !ok1 || !ok2 {
				return types.NewErr("action_count_in_window arguments must be strings")
			}
			if ctx.Session.ActionCountByType == nil {
				return types.Int(0)
			}
			return types.Int(int64(ctx.Session.ActionCountByType(actionType, window)))
		}

		var err error
		prg, err = c.env.Program(rule.ast,
			cel.Functions(
				&functions.Overload{
					Operator: "action_count_in_window_string_string",
					Function: countFn,
				},
			),
		)
		if err != nil {
			return false, fmt.Errorf("CEL program creation failed for %q: %w", rule.Expression, err)
		}
	} else {
		prg = rule.program
	}

	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("CEL evaluation error for %q: %w", rule.Expression, err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression %q returned non-bool: %T", rule.Expression, out.Value())
	}

	return result, nil
}
