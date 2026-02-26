package safety

import (
	"strings"
	"testing"
)

func TestEngine_SetAndGetInvariants(t *testing.T) {
	e := NewEngine(nil)

	invariants := []Invariant{
		{
			Description: "NEVER delete more than 5 emails",
			Condition:   "action_count_in_window('email.delete', '3600s') > 5",
			Effect:      "deny",
			Enforcement: "proxy",
		},
		{
			Description: "STOP on user command",
			Enforcement: "inject",
		},
	}

	e.SetInvariants("agent-1", invariants)

	got := e.GetInvariants("agent-1")
	if len(got) != 2 {
		t.Fatalf("got %d invariants, want 2", len(got))
	}
	if got[0].Description != "NEVER delete more than 5 emails" {
		t.Errorf("invariant[0].Description = %q", got[0].Description)
	}
}

func TestEngine_DefaultEffectAndEnforcement(t *testing.T) {
	e := NewEngine(nil)

	invariants := []Invariant{
		{Description: "test invariant"},
	}

	e.SetInvariants("agent-1", invariants)

	got := e.GetInvariants("agent-1")
	if got[0].Effect != "deny" {
		t.Errorf("default effect = %q, want 'deny'", got[0].Effect)
	}
	if got[0].Enforcement != "proxy" {
		t.Errorf("default enforcement = %q, want 'proxy'", got[0].Enforcement)
	}
}

func TestEngine_GetProxyConditions(t *testing.T) {
	e := NewEngine(nil)

	invariants := []Invariant{
		{
			Description: "proxy invariant with condition",
			Condition:   "action.type == 'file.delete'",
			Enforcement: "proxy",
		},
		{
			Description: "inject-only invariant",
			Enforcement: "inject",
		},
		{
			Description: "proxy without condition (should be excluded)",
			Enforcement: "proxy",
		},
	}

	e.SetInvariants("agent-1", invariants)

	conditions := e.GetProxyConditions("agent-1")
	if len(conditions) != 1 {
		t.Fatalf("got %d proxy conditions, want 1", len(conditions))
	}
	if conditions[0].Condition != "action.type == 'file.delete'" {
		t.Errorf("condition = %q", conditions[0].Condition)
	}
}

func TestEngine_GetInjectionText(t *testing.T) {
	e := NewEngine(nil)

	invariants := []Invariant{
		{
			Description: "proxy-only invariant",
			Condition:   "some.condition",
			Enforcement: "proxy",
		},
		{
			Description: "inject invariant 1",
			Enforcement: "inject",
		},
		{
			Description: "both enforcement",
			Enforcement: "both",
		},
	}

	e.SetInvariants("agent-1", invariants)

	text := e.GetInjectionText("agent-1")
	if !strings.Contains(text, "SAFETY INVARIANTS") {
		t.Error("expected header in injection text")
	}
	if !strings.Contains(text, "inject invariant 1") {
		t.Error("expected inject invariant in text")
	}
	if !strings.Contains(text, "both enforcement") {
		t.Error("expected 'both' enforcement invariant in text")
	}
	if strings.Contains(text, "proxy-only invariant") {
		t.Error("proxy-only invariant should NOT be in injection text")
	}
}

func TestEngine_GetInjectionText_Empty(t *testing.T) {
	e := NewEngine(nil)

	// No invariants set.
	text := e.GetInjectionText("agent-1")
	if text != "" {
		t.Errorf("expected empty injection text, got %q", text)
	}

	// Invariants set but all proxy-only.
	e.SetInvariants("agent-2", []Invariant{
		{Description: "proxy only", Condition: "x", Enforcement: "proxy"},
	})
	text = e.GetInjectionText("agent-2")
	if text != "" {
		t.Errorf("expected empty injection text for proxy-only, got %q", text)
	}
}

func TestEngine_AllInvariants(t *testing.T) {
	e := NewEngine(nil)

	e.SetInvariants("agent-1", []Invariant{{Description: "inv1"}})
	e.SetInvariants("agent-2", []Invariant{{Description: "inv2"}, {Description: "inv3"}})

	all := e.AllInvariants()
	if len(all) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(all))
	}
	if len(all["agent-1"]) != 1 {
		t.Errorf("agent-1 invariants = %d, want 1", len(all["agent-1"]))
	}
	if len(all["agent-2"]) != 2 {
		t.Errorf("agent-2 invariants = %d, want 2", len(all["agent-2"]))
	}
}

func TestEngine_Count(t *testing.T) {
	e := NewEngine(nil)

	if e.Count() != 0 {
		t.Errorf("initial count = %d, want 0", e.Count())
	}

	e.SetInvariants("agent-1", []Invariant{{Description: "inv1"}})
	e.SetInvariants("agent-2", []Invariant{{Description: "inv2"}, {Description: "inv3"}})

	if e.Count() != 3 {
		t.Errorf("count = %d, want 3", e.Count())
	}
}

func TestEngine_SetInvariantsOverwrites(t *testing.T) {
	e := NewEngine(nil)

	e.SetInvariants("agent-1", []Invariant{
		{Description: "old"},
		{Description: "old2"},
	})

	e.SetInvariants("agent-1", []Invariant{
		{Description: "new"},
	})

	got := e.GetInvariants("agent-1")
	if len(got) != 1 {
		t.Fatalf("expected 1 invariant after overwrite, got %d", len(got))
	}
	if got[0].Description != "new" {
		t.Errorf("description = %q, want 'new'", got[0].Description)
	}
}
