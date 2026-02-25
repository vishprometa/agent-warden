package spawn

import (
	"testing"
)

func TestGovernor_BasicSpawn(t *testing.T) {
	g := NewGovernor(DefaultConfig(), nil)
	g.RegisterRoot("parent", 100.0)

	result := g.RequestSpawn("parent", "child-1", 10.0)
	if !result.Allowed {
		t.Fatalf("expected spawn allowed: %s", result.Reason)
	}

	if g.AgentCount() != 2 {
		t.Errorf("agent count = %d, want 2", g.AgentCount())
	}
}

func TestGovernor_SpawnDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	g := NewGovernor(cfg, nil)

	result := g.RequestSpawn("parent", "child-1", 10.0)
	if result.Allowed {
		t.Fatal("expected spawn denied when disabled")
	}
}

func TestGovernor_MaxChildren(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxChildrenPerAgent = 2
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("parent", 100.0)

	// Spawn 2 children — should succeed.
	g.RequestSpawn("parent", "child-1", 10.0)
	g.RequestSpawn("parent", "child-2", 10.0)

	// 3rd child should fail.
	result := g.RequestSpawn("parent", "child-3", 10.0)
	if result.Allowed {
		t.Fatal("expected spawn denied: max children reached")
	}
}

func TestGovernor_MaxDepth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxDepth = 2
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("root", 100.0)

	// Root (depth 0) → child (depth 1).
	g.RequestSpawn("root", "child-1", 10.0)

	// Child (depth 1) → grandchild (depth 2).
	g.RequestSpawn("child-1", "grandchild-1", 5.0)

	// Grandchild (depth 2) → great-grandchild (depth 3) — should fail.
	result := g.RequestSpawn("grandchild-1", "great-grandchild-1", 2.0)
	if result.Allowed {
		t.Fatal("expected spawn denied: max depth exceeded")
	}
}

func TestGovernor_GlobalLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxGlobalAgents = 3
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("root", 100.0)

	// root=1, child1=2
	g.RequestSpawn("root", "child-1", 10.0)
	// root=1, child1=2, child2=3
	g.RequestSpawn("root", "child-2", 10.0)

	// Now at limit — should deny.
	result := g.RequestSpawn("root", "child-3", 10.0)
	if result.Allowed {
		t.Fatal("expected spawn denied: global limit reached")
	}
}

func TestGovernor_BudgetInheritance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChildBudgetMax = 0.5 // children get max 50% of parent budget
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("parent", 100.0)

	// Request 40 (within 50% of 100) — should succeed.
	result := g.RequestSpawn("parent", "child-1", 40.0)
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}

	// Request 60 (exceeds 50% of 100) — should fail.
	result = g.RequestSpawn("parent", "child-2", 60.0)
	if result.Allowed {
		t.Fatal("expected denied: exceeds budget limit")
	}
}

func TestGovernor_RequireApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequireApproval = true
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("parent", 100.0)

	result := g.RequestSpawn("parent", "child-1", 10.0)
	if result.Allowed {
		t.Fatal("expected denied: requires approval")
	}
	if result.Reason != "spawn requires human approval" {
		t.Errorf("reason = %q, want 'spawn requires human approval'", result.Reason)
	}
}

func TestGovernor_CascadeKill(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CascadeKill = true
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("root", 100.0)

	g.RequestSpawn("root", "child-1", 30.0)
	g.RequestSpawn("root", "child-2", 30.0)
	g.RequestSpawn("child-1", "grandchild-1", 10.0)

	if g.AgentCount() != 4 {
		t.Fatalf("agent count = %d, want 4", g.AgentCount())
	}

	// Kill root — should cascade to all descendants.
	killed := g.KillAgent("root")
	if len(killed) != 4 {
		t.Errorf("killed %d agents, want 4: %v", len(killed), killed)
	}

	if g.AgentCount() != 0 {
		t.Errorf("agent count after cascade kill = %d, want 0", g.AgentCount())
	}
}

func TestGovernor_NoCascadeKill(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CascadeKill = false
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("root", 100.0)

	g.RequestSpawn("root", "child-1", 30.0)

	// Kill root without cascade.
	killed := g.KillAgent("root")
	if len(killed) != 1 {
		t.Errorf("killed %d agents, want 1", len(killed))
	}

	// child-1 should still exist.
	if g.AgentCount() != 1 {
		t.Errorf("agent count = %d, want 1 (child still alive)", g.AgentCount())
	}
}

func TestGovernor_GetDescendants(t *testing.T) {
	cfg := DefaultConfig()
	g := NewGovernor(cfg, nil)
	g.RegisterRoot("root", 100.0)

	g.RequestSpawn("root", "child-1", 30.0)
	g.RequestSpawn("root", "child-2", 30.0)
	g.RequestSpawn("child-1", "grandchild-1", 10.0)

	descendants := g.GetDescendants("root")
	if len(descendants) != 3 {
		t.Errorf("descendants = %d, want 3: %v", len(descendants), descendants)
	}

	descendants = g.GetDescendants("child-1")
	if len(descendants) != 1 {
		t.Errorf("descendants of child-1 = %d, want 1", len(descendants))
	}

	descendants = g.GetDescendants("grandchild-1")
	if len(descendants) != 0 {
		t.Errorf("descendants of grandchild-1 = %d, want 0", len(descendants))
	}
}

func TestGovernor_GetTree(t *testing.T) {
	g := NewGovernor(DefaultConfig(), nil)
	g.RegisterRoot("root", 100.0)
	g.RequestSpawn("root", "child-1", 30.0)

	tree := g.GetTree()
	if len(tree) != 2 {
		t.Fatalf("tree size = %d, want 2", len(tree))
	}

	root := tree["root"]
	if root == nil {
		t.Fatal("root not in tree")
	}
	if len(root.Children) != 1 || root.Children[0] != "child-1" {
		t.Errorf("root.Children = %v, want [child-1]", root.Children)
	}

	child := tree["child-1"]
	if child == nil {
		t.Fatal("child-1 not in tree")
	}
	if child.ParentID != "root" {
		t.Errorf("child.ParentID = %q, want 'root'", child.ParentID)
	}
	if child.Depth != 1 {
		t.Errorf("child.Depth = %d, want 1", child.Depth)
	}
}

func TestGovernor_AutoRegisterParent(t *testing.T) {
	g := NewGovernor(DefaultConfig(), nil)

	// Spawn from unknown parent with 0 budget — auto-registered parent has 0
	// budget, so child budget must be 0 to pass the budget check.
	result := g.RequestSpawn("unknown-parent", "child-1", 0)
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}

	if g.AgentCount() != 2 {
		t.Errorf("agent count = %d, want 2", g.AgentCount())
	}
}

func TestGovernor_RegisterRootIdempotent(t *testing.T) {
	g := NewGovernor(DefaultConfig(), nil)
	g.RegisterRoot("root", 100.0)
	g.RegisterRoot("root", 200.0) // should not overwrite

	tree := g.GetTree()
	if tree["root"].Budget != 100.0 {
		t.Errorf("budget = %.2f, want 100.00 (should not overwrite)", tree["root"].Budget)
	}
}
