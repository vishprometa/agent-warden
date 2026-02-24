package policy

import (
	"testing"
)

func mustNewCELEvaluator(t *testing.T) *CELEvaluator {
	t.Helper()
	eval, err := NewCELEvaluator(nil)
	if err != nil {
		t.Fatalf("NewCELEvaluator() error: %v", err)
	}
	return eval
}

func TestCELEvaluator_CompileValidExpression(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	tests := []struct {
		name string
		expr string
	}{
		{"action type check", `action.type == "llm.chat"`},
		{"session cost check", `session.cost > 10.0`},
		{"action count check", `session.action_count > 100`},
		{"combined conditions", `action.type == "llm.chat" && session.cost > 5.0`},
		{"agent name check", `agent.name == "test-agent"`},
		{"string contains", `action.target.contains("production")`},
		{"or condition", `action.type == "db.query" || action.type == "file.write"`},
		{"negation", `!(action.type == "llm.chat")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := eval.CompileExpression(tt.expr)
			if err != nil {
				t.Fatalf("CompileExpression(%q) error: %v", tt.expr, err)
			}
			if rule.Expression != tt.expr {
				t.Errorf("rule.Expression = %q, want %q", rule.Expression, tt.expr)
			}
		})
	}
}

func TestCELEvaluator_CompileInvalidExpression(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	tests := []struct {
		name string
		expr string
	}{
		{"syntax error", `action.type ==`},
		{"undefined variable", `nonexistent.field == "test"`},
		{"type mismatch - string compared to int", `action.type > 5`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eval.CompileExpression(tt.expr)
			if err == nil {
				t.Errorf("CompileExpression(%q) expected error, got nil", tt.expr)
			}
		})
	}
}

func TestCELEvaluator_CompileNonBoolExpression(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	// Expression that returns a string, not a bool
	_, err := eval.CompileExpression(`action.type`)
	if err == nil {
		t.Error("CompileExpression for non-bool expression should return error")
	}
}

func TestCELEvaluator_EvaluateActionType(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	rule, err := eval.CompileExpression(`action.type == "llm.chat"`)
	if err != nil {
		t.Fatalf("CompileExpression error: %v", err)
	}

	tests := []struct {
		name       string
		actionType string
		want       bool
	}{
		{"matching type", "llm.chat", true},
		{"non-matching type", "tool.call", false},
		{"empty type", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ActionContext{
				Action: ActionInfo{
					Type:   tt.actionType,
					Name:   "test",
					Params: map[string]interface{}{},
					Target: "",
				},
				Session: SessionInfo{
					ID:          "sess-1",
					AgentID:     "agent-1",
					Cost:        0,
					ActionCount: 0,
				},
				Agent: AgentInfo{
					ID:   "agent-1",
					Name: "test-agent",
				},
			}

			result, err := eval.Evaluate(rule, ctx)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestCELEvaluator_EvaluateSessionCost(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	rule, err := eval.CompileExpression(`session.cost > 10.0`)
	if err != nil {
		t.Fatalf("CompileExpression error: %v", err)
	}

	tests := []struct {
		name string
		cost float64
		want bool
	}{
		{"over threshold", 15.0, true},
		{"exactly at threshold", 10.0, false},
		{"under threshold", 5.0, false},
		{"zero cost", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ActionContext{
				Action: ActionInfo{
					Type:   "llm.chat",
					Name:   "test",
					Params: map[string]interface{}{},
				},
				Session: SessionInfo{
					ID:      "sess-1",
					AgentID: "agent-1",
					Cost:    tt.cost,
				},
				Agent: AgentInfo{ID: "agent-1", Name: "agent"},
			}

			result, err := eval.Evaluate(rule, ctx)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate(cost=%f) = %v, want %v", tt.cost, result, tt.want)
			}
		})
	}
}

func TestCELEvaluator_EvaluateActionCount(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	rule, err := eval.CompileExpression(`session.action_count > 100`)
	if err != nil {
		t.Fatalf("CompileExpression error: %v", err)
	}

	ctx := ActionContext{
		Action: ActionInfo{
			Type:   "llm.chat",
			Name:   "test",
			Params: map[string]interface{}{},
		},
		Session: SessionInfo{
			ID:          "sess-1",
			AgentID:     "agent-1",
			ActionCount: 150,
		},
		Agent: AgentInfo{ID: "agent-1", Name: "agent"},
	}

	result, err := eval.Evaluate(rule, ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if !result {
		t.Error("expected true for action_count=150 > 100")
	}
}

func TestCELEvaluator_EvaluateCombinedCondition(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	rule, err := eval.CompileExpression(`action.type == "db.query" && action.target.contains("production")`)
	if err != nil {
		t.Fatalf("CompileExpression error: %v", err)
	}

	tests := []struct {
		name       string
		actionType string
		target     string
		want       bool
	}{
		{"both match", "db.query", "production-db-01", true},
		{"type matches, target doesn't", "db.query", "staging-db-01", false},
		{"type doesn't match", "llm.chat", "production-db-01", false},
		{"neither matches", "llm.chat", "staging-db-01", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ActionContext{
				Action: ActionInfo{
					Type:   tt.actionType,
					Name:   "test",
					Params: map[string]interface{}{},
					Target: tt.target,
				},
				Session: SessionInfo{
					ID:      "sess-1",
					AgentID: "agent-1",
				},
				Agent: AgentInfo{ID: "agent-1", Name: "agent"},
			}

			result, err := eval.Evaluate(rule, ctx)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate(type=%q, target=%q) = %v, want %v",
					tt.actionType, tt.target, result, tt.want)
			}
		})
	}
}

func TestCELEvaluator_NilParamsHandled(t *testing.T) {
	eval := mustNewCELEvaluator(t)

	rule, err := eval.CompileExpression(`action.type == "llm.chat"`)
	if err != nil {
		t.Fatalf("CompileExpression error: %v", err)
	}

	// Params is nil -- should not panic
	ctx := ActionContext{
		Action: ActionInfo{
			Type:   "llm.chat",
			Name:   "test",
			Params: nil,
			Target: "",
		},
		Session: SessionInfo{
			ID:      "sess-1",
			AgentID: "agent-1",
		},
		Agent: AgentInfo{ID: "agent-1", Name: "agent"},
	}

	result, err := eval.Evaluate(rule, ctx)
	if err != nil {
		t.Fatalf("Evaluate with nil params error: %v", err)
	}
	if !result {
		t.Error("expected true")
	}
}
