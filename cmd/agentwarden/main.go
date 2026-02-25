package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentwarden/agentwarden/internal/adapter/openclaw"
	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/api"
	"github.com/agentwarden/agentwarden/internal/approval"
	"github.com/agentwarden/agentwarden/internal/capability"
	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/dashboard"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/killswitch"
	"github.com/agentwarden/agentwarden/internal/mdloader"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/safety"
	"github.com/agentwarden/agentwarden/internal/sanitize"
	"github.com/agentwarden/agentwarden/internal/server"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/spawn"
	"github.com/agentwarden/agentwarden/internal/trace"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "agentwarden",
		Short: "Runtime governance sidecar for AI agents",
		Long:  "AgentWarden — Observe. Enforce. Evolve.\nA governance sidecar that enforces policies on AI agent actions via gRPC/HTTP events.",
	}

	var configFile string
	var port int
	var devMode bool

	// ─── start ───
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the AgentWarden governance server and dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(configFile, port, devMode)
		},
	}
	startCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file (default: agentwarden.yaml)")
	startCmd.Flags().IntVarP(&port, "port", "p", 0, "Override HTTP port (default: 6777)")
	startCmd.Flags().BoolVar(&devMode, "dev", false, "Dev mode: verbose logs, auto-reload, CORS *")

	// ─── init ───
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Generate starter config and directory structure",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}

	initAgentCmd := &cobra.Command{
		Use:   "agent [agent-id]",
		Short: "Scaffold agents/<id>/AGENT.md + EVOLVE.md templates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitAgent(args[0])
		},
	}

	initPolicyCmd := &cobra.Command{
		Use:   "policy [policy-name]",
		Short: "Scaffold policies/<name>/policy.yaml + POLICY.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitPolicy(args[0])
		},
	}

	initPlaybookCmd := &cobra.Command{
		Use:   "playbook [detection-type]",
		Short: "Scaffold playbooks/<DETECTION>.md from template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitPlaybook(args[0])
		},
	}

	initCmd.AddCommand(initAgentCmd, initPolicyCmd, initPlaybookCmd)

	// ─── status ───
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show running status, active sessions, recent violations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(port)
		},
	}

	// ─── version ───
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("AgentWarden %s\n", version)
			fmt.Printf("  Commit:  %s\n", commit)
			fmt.Printf("  Built:   %s\n", buildDate)
		},
	}

	// ─── policy ───
	policyCmd := &cobra.Command{
		Use:   "policy",
		Short: "Policy management commands",
	}

	policyValidateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate config + check referenced MDs exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyValidate(configFile)
		},
	}
	policyValidateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file")

	policyReloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Hot-reload policies without restart",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/policies/reload", p), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect to AgentWarden: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == 200 {
				fmt.Println("✓ Policies reloaded")
			} else {
				fmt.Printf("✗ Reload failed (HTTP %d)\n", resp.StatusCode)
			}
			return nil
		},
	}

	policyListCmd := &cobra.Command{
		Use:   "list",
		Short: "Show all loaded policies with status",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/policies", p))
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			_ = decodeJSON(resp, &result)
			policies, _ := result["policies"].([]interface{})
			if len(policies) == 0 {
				fmt.Println("No policies loaded.")
				return nil
			}
			fmt.Printf("%-25s %-12s %-15s %s\n", "NAME", "TYPE", "EFFECT", "CONDITION")
			fmt.Println(strings.Repeat("─", 80))
			for _, p := range policies {
				m := p.(map[string]interface{})
				fmt.Printf("%-25v %-12v %-15v %v\n", m["name"], m["type"], m["effect"], truncate(str(m["condition"]), 30))
			}
			return nil
		},
	}

	policyCmd.AddCommand(policyValidateCmd, policyReloadCmd, policyListCmd)

	// ─── trace ───
	traceCmd := &cobra.Command{
		Use:   "trace",
		Short: "Trace inspection commands",
	}

	traceListCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTraceList(port, cmd)
		},
	}
	var traceAgent string
	traceListCmd.Flags().StringVar(&traceAgent, "agent", "", "Filter by agent ID")

	traceShowCmd := &cobra.Command{
		Use:   "show [session-id]",
		Short: "Show full session trace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTraceShow(port, args[0])
		},
	}

	traceSearchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Full-text search across traces",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTraceSearch(port, args[0])
		},
	}

	traceVerifyCmd := &cobra.Command{
		Use:   "verify [session-id]",
		Short: "Verify hash chain integrity for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/traces/verify/%s", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			if err := decodeJSON(resp, &result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
			if valid, _ := result["valid"].(bool); valid {
				fmt.Printf("✓ Hash chain intact for session %s (%v traces verified)\n", args[0], result["verified_count"])
			} else {
				fmt.Printf("✗ Hash chain broken for session %s at trace %v\n", args[0], result["broken_at"])
			}
			return nil
		},
	}

	traceCmd.AddCommand(traceListCmd, traceShowCmd, traceSearchCmd, traceVerifyCmd)

	// ─── agent ───
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent management commands",
	}

	agentListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all known agents with stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentList(port)
		},
	}

	agentShowCmd := &cobra.Command{
		Use:   "show [agent-id]",
		Short: "Show AGENT.md + current version + metrics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentShow(port, args[0])
		},
	}

	agentStatsCmd := &cobra.Command{
		Use:   "stats [agent-id]",
		Short: "Performance metrics for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/agents/%s", p, args[0]))
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			if err := decodeJSON(resp, &result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}

	agentPauseCmd := &cobra.Command{
		Use:   "pause [agent-id]",
		Short: "Block all actions for an agent (graceful)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/agents/%s/pause", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			fmt.Printf("✓ Agent %s paused\n", args[0])
			return nil
		},
	}

	agentResumeCmd := &cobra.Command{
		Use:   "resume [agent-id]",
		Short: "Unblock a paused agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/agents/%s/resume", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			fmt.Printf("✓ Agent %s resumed\n", args[0])
			return nil
		},
	}

	agentKillCmd := &cobra.Command{
		Use:   "kill [session-id]",
		Short: "Force-terminate a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost:%d/api/sessions/%s", p, args[0]), nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			fmt.Printf("✓ Session %s terminated\n", args[0])
			return nil
		},
	}

	agentCmd.AddCommand(agentListCmd, agentShowCmd, agentStatsCmd, agentPauseCmd, agentResumeCmd, agentKillCmd)

	// ─── evolve ───
	evolveCmd := &cobra.Command{
		Use:   "evolve",
		Short: "Evolution engine commands",
	}

	evolveStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Active evolution loops",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/evolution/status", p))
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			_ = decodeJSON(resp, &result)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}

	evolveTriggerCmd := &cobra.Command{
		Use:   "trigger [agent-id]",
		Short: "Manually trigger evolution analysis",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			fmt.Printf("Triggering evolution for agent %s...\n", args[0])
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/evolution/%s/trigger", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			_ = decodeJSON(resp, &result)
			fmt.Printf("✓ Evolution triggered. Status: %v\n", result["status"])
			return nil
		},
	}

	evolveHistoryCmd := &cobra.Command{
		Use:   "history [agent-id]",
		Short: "Version timeline (PROMPT.md diffs)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/agents/%s/versions", p, args[0]))
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			_ = decodeJSON(resp, &result)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}

	evolveDiffCmd := &cobra.Command{
		Use:   "diff [agent-id]",
		Short: "Current vs proposed PROMPT.md diff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load MDs locally and diff
			loader := mdloader.NewLoader("./agents", "./policies", "./playbooks")
			currentVer, err := loader.CurrentVersion(args[0])
			if err != nil {
				return fmt.Errorf("no versions found for agent %s: %w", args[0], err)
			}
			current, err := loader.LoadPromptMD(args[0], currentVer)
			if err != nil {
				return fmt.Errorf("failed to load current PROMPT.md: %w", err)
			}
			candidate, err := loader.LoadPromptMD(args[0], currentVer+"-candidate")
			if err != nil {
				fmt.Println("No candidate version found. Trigger evolution first.")
				return nil
			}
			fmt.Printf("─── Current (%s) ───\n", currentVer)
			fmt.Println(current)
			fmt.Printf("\n─── Candidate (%s-candidate) ───\n", currentVer)
			fmt.Println(candidate)
			return nil
		},
	}

	evolvePromoteCmd := &cobra.Command{
		Use:   "promote [agent-id]",
		Short: "Manually promote candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/evolution/%s/promote", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			fmt.Printf("✓ Candidate promoted for agent %s\n", args[0])
			return nil
		},
	}

	evolveRollbackCmd := &cobra.Command{
		Use:   "rollback [agent-id]",
		Short: "Revert to previous version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/evolution/%s/rollback", p, args[0]), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			fmt.Printf("✓ Rolled back agent %s to previous version\n", args[0])
			return nil
		},
	}

	evolveCmd.AddCommand(evolveStatusCmd, evolveTriggerCmd, evolveHistoryCmd, evolveDiffCmd, evolvePromoteCmd, evolveRollbackCmd)

	// ─── doctor ───
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check config, connectivity, storage, and MD integrity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(port, configFile)
		},
	}

	// ─── mock ───
	mockCmd := &cobra.Command{
		Use:   "mock",
		Short: "Generate mock agent traffic for testing",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMock(port)
		},
	}

	// ─── kill (emergency kill switch) ───
	var killAll bool
	killCmd := &cobra.Command{
		Use:   "kill [agent-id|session-id]",
		Short: "Emergency kill switch — immediately block all agent actions",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := resolvePort(port)
			if killAll {
				resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/killswitch/trigger", p), "application/json",
					strings.NewReader(`{"scope":"global","reason":"CLI kill command","source":"cli"}`))
				if err != nil {
					return fmt.Errorf("failed to connect: %w", err)
				}
				_ = resp.Body.Close()
				fmt.Println("  GLOBAL KILL SWITCH ACTIVATED — all agent actions blocked")
				return nil
			}
			if len(args) == 0 {
				return fmt.Errorf("specify agent-id, session-id, or --all")
			}
			target := args[0]
			scope := "agent"
			if strings.HasPrefix(target, "ses_") || strings.HasPrefix(target, "oc_") {
				scope = "session"
			}
			body := fmt.Sprintf(`{"scope":"%s","target_id":"%s","reason":"CLI kill command","source":"cli"}`, scope, target)
			resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/killswitch/trigger", p), "application/json", strings.NewReader(body))
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			_ = resp.Body.Close()
			fmt.Printf("  KILL SWITCH ACTIVATED for %s %s\n", scope, target)
			return nil
		},
	}
	killCmd.Flags().BoolVar(&killAll, "all", false, "Activate global kill switch (blocks ALL agents)")

	rootCmd.AddCommand(startCmd, initCmd, statusCmd, versionCmd, policyCmd, traceCmd, agentCmd, evolveCmd, doctorCmd, mockCmd, killCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runStart(configFile string, portOverride int, devMode bool) error {
	// Load config
	cfgLoader := config.NewLoader()
	if configFile == "" {
		configFile = findConfigFile()
	}
	if configFile != "" {
		if err := cfgLoader.Load(configFile); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	cfg := cfgLoader.Get()

	if portOverride > 0 {
		cfg.Server.Port = portOverride
	}
	if devMode {
		cfg.Server.CORS = true
		cfg.Server.LogLevel = "debug"
	}

	// Setup logger
	logLevel := slog.LevelInfo
	switch strings.ToLower(cfg.Server.LogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	// Initialize trace store
	store, err := trace.NewSQLiteStore(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	if err := store.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Initialize MD loader
	mdLoader := mdloader.NewLoader(cfg.AgentsDir, cfg.PoliciesDir, cfg.PlaybooksDir)
	mdWatcher, err := mdloader.NewWatcher(mdLoader, logger)
	if err != nil {
		logger.Warn("failed to create MD file watcher", "error", err)
	} else {
		if err := mdWatcher.Start(); err != nil {
			logger.Warn("failed to start MD file watcher", "error", err)
		} else {
			defer func() { _ = mdWatcher.Stop() }()
		}
	}

	// Initialize session manager and cost tracker
	sessionMgr := session.NewManager(store, logger)
	costTracker := cost.NewTracker(logger)

	// Initialize alert manager
	alertMgr := alert.NewManager(cfg.Alerts, logger)

	// Initialize approval queue
	approvalQueue := approval.NewQueue(store, alertMgr, logger)

	// Initialize policy engine
	celEval, err := policy.NewCELEvaluator(logger)
	if err != nil {
		return fmt.Errorf("failed to create CEL evaluator: %w", err)
	}
	policyLoader := policy.NewLoader(celEval, logger)
	budgetChecker := policy.NewBudgetChecker(logger)
	policyEngine := policy.NewEngine(policyLoader, celEval, budgetChecker, logger)
	policyEngine.SetConfigLoader(cfgLoader)
	if err := policyEngine.LoadPolicies(cfg.Policies); err != nil {
		logger.Warn("some policies failed to load", "error", err)
	}

	// Initialize detection engine
	detectionEngine := detection.NewEngine(cfg.Detection, func(event detection.Event) {
		alertMgr.Send(alert.Alert{
			Type:      event.Type,
			Severity:  "warning",
			Title:     fmt.Sprintf("Detection: %s", event.Type),
			Message:   event.Message,
			AgentID:   event.AgentID,
			SessionID: event.SessionID,
			Details:   event.Details,
		})
	}, logger)

	// ─── Autonomous Agent Governance Components ───

	// Initialize kill switch (checked before ALL policy evaluation).
	ks := killswitch.New(logger)

	// Start file-based kill switch watcher.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ks.CheckFileKill()
		}
	}()

	// Initialize capability engine.
	capEngine := capability.NewEngine(logger)

	// Initialize safety invariants engine.
	safetyEngine := safety.NewEngine(logger)

	// Initialize spawn governor.
	spawnGov := spawn.NewGovernor(spawn.Config{
		Enabled:             cfg.Spawn.Enabled,
		MaxChildrenPerAgent: cfg.Spawn.MaxChildrenPerAgent,
		MaxDepth:            cfg.Spawn.MaxDepth,
		MaxGlobalAgents:     cfg.Spawn.MaxGlobalAgents,
		InheritCapabilities: cfg.Spawn.InheritCapabilities,
		RequireApproval:     cfg.Spawn.RequireApproval,
		CascadeKill:         cfg.Spawn.CascadeKill,
		ChildBudgetMax:      cfg.Spawn.ChildBudgetMax,
	}, logger)

	// Initialize prompt injection scanner.
	injectionScanner := sanitize.NewScanner(sanitize.Config{
		Enabled: cfg.Sanitize.Enabled,
		Mode:    cfg.Sanitize.Mode,
	}, logger)

	// Initialize OpenClaw adapter if enabled.
	var openclawAdapter *openclaw.GatewayAdapter
	if cfg.Adapters.OpenClaw.Enabled {
		openclawAdapter = openclaw.NewGatewayAdapter(openclaw.Config{
			Enabled:    true,
			Mode:       cfg.Adapters.OpenClaw.Mode,
			GatewayURL: cfg.Adapters.OpenClaw.GatewayURL,
			AuthToken:  cfg.Adapters.OpenClaw.AuthToken,
			ProxyPath:  cfg.Adapters.OpenClaw.ProxyPath,
			Intercept:  cfg.Adapters.OpenClaw.Intercept,
		}, logger)

		// Start the adapter with a governance-aware evaluator.
		go func() {
			evaluator := func(ctx policy.ActionContext) policy.PolicyResult {
				// 1. Kill switch check (highest priority).
				if blocked, reason := ks.IsBlocked(ctx.Agent.ID, ctx.Session.ID); blocked {
					return policy.PolicyResult{
						Effect:  "terminate",
						Message: reason,
					}
				}

				// 2. Capability check.
				capResult := capEngine.Check(ctx.Agent.ID, ctx.Action.Type, ctx.Action.Params)
				if !capResult.Allowed {
					return policy.PolicyResult{
						Effect:     "deny",
						PolicyName: "capability-boundary",
						Message:    capResult.Reason,
					}
				}

				// 3. Spawn governance.
				if ctx.Action.Type == "agent.spawn" {
					childID := ctx.Action.Target
					result := spawnGov.RequestSpawn(ctx.Agent.ID, childID, 0)
					if !result.Allowed {
						return policy.PolicyResult{
							Effect:     "deny",
							PolicyName: "spawn-governor",
							Message:    result.Reason,
						}
					}
				}

				// 4. Policy evaluation.
				return policyEngine.Evaluate(ctx)
			}

			if err := openclawAdapter.Start(context.Background(), evaluator); err != nil {
				logger.Error("OpenClaw adapter error", "error", err)
			}
		}()
	}

	// Log governance component status.
	logger.Info("governance components initialized",
		"kill_switch", "armed",
		"capability_engine", true,
		"safety_invariants", safetyEngine.Count(),
		"spawn_governor", cfg.Spawn.Enabled,
		"injection_scanner", cfg.Sanitize.Enabled,
		"openclaw_adapter", cfg.Adapters.OpenClaw.Enabled,
	)

	// Suppress unused variable warnings for components used via API.
	_ = injectionScanner
	_ = spawnGov
	_ = safetyEngine
	_ = capEngine

	// Initialize management API server
	apiServer := api.NewServer(cfg.Server, store, cfgLoader, approvalQueue, logger)

	// Initialize gRPC event server
	grpcEventServer := server.NewGRPCServer(
		policyEngine, store, sessionMgr, costTracker,
		detectionEngine, alertMgr, logger,
	)

	// Initialize HTTP event server
	httpEventsServer := server.NewHTTPEventsServer(
		policyEngine, store, sessionMgr, costTracker,
		detectionEngine, alertMgr, logger,
	)

	// Build combined HTTP server: /dashboard → SPA, /api/* → management, /v1/events/* → event endpoints
	masterMux := http.NewServeMux()
	if cfg.Server.Dashboard {
		masterMux.Handle("/dashboard/", dashboard.Handler())
	}
	masterMux.Handle("/api/", apiServer.Handler())

	// Event endpoints (SDK/webhook event receiver)
	httpEventsServer.RegisterRoutes(masterMux)

	// OpenClaw gateway proxy endpoint.
	if openclawAdapter != nil {
		proxyPath := cfg.Adapters.OpenClaw.ProxyPath
		if proxyPath == "" {
			proxyPath = "/gateway"
		}
		masterMux.HandleFunc(proxyPath, openclawAdapter.HandleWebSocket)
		logger.Info("OpenClaw gateway proxy registered", "path", proxyPath)
	}

	// Kill switch API endpoints.
	masterMux.HandleFunc("POST /api/killswitch/trigger", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Scope    string `json:"scope"`
			TargetID string `json:"target_id"`
			Reason   string `json:"reason"`
			Source   string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch req.Scope {
		case "global":
			ks.TriggerGlobal(req.Reason, req.Source)
			if openclawAdapter != nil {
				openclawAdapter.KillAll()
			}
		case "agent":
			ks.TriggerAgent(req.TargetID, req.Reason, req.Source)
			if openclawAdapter != nil {
				openclawAdapter.KillAgent(req.TargetID)
			}
		case "session":
			ks.TriggerSession(req.TargetID, req.Reason, req.Source)
			if openclawAdapter != nil {
				openclawAdapter.KillSession(req.TargetID)
			}
		default:
			http.Error(w, "invalid scope: use global, agent, or session", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
	})

	masterMux.HandleFunc("GET /api/killswitch/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ks.Status())
	})

	masterMux.HandleFunc("POST /api/killswitch/reset", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Scope    string `json:"scope"`
			TargetID string `json:"target_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch req.Scope {
		case "global":
			ks.ResetGlobal()
		case "agent":
			ks.ResetAgent(req.TargetID)
		case "session":
			ks.ResetSession(req.TargetID)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      masterMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// Print startup banner
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════╗")
	fmt.Println("  ║          AgentWarden v" + version + "              ║")
	fmt.Println("  ║   Runtime governance for AI agents       ║")
	fmt.Println("  ╚══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  → HTTP:      http://localhost:%d\n", cfg.Server.Port)
	fmt.Printf("  → gRPC:      localhost:%d\n", cfg.Server.GRPCPort)
	if cfg.Server.Dashboard {
		fmt.Printf("  → Dashboard: http://localhost:%d/dashboard\n", cfg.Server.Port)
	}
	fmt.Printf("  → API:       http://localhost:%d/api\n", cfg.Server.Port)
	fmt.Printf("  → Events:    http://localhost:%d/v1/events\n", cfg.Server.Port)
	fmt.Printf("  → Storage:   %s (%s)\n", cfg.Storage.Driver, cfg.Storage.Path)
	fmt.Printf("  → Policies:  %d loaded\n", policyEngine.PolicyCount())
	fmt.Printf("  → Fail mode: %s\n", cfg.Server.FailMode)
	fmt.Println()
	fmt.Println("  SDK:  pip install agentwarden")
	fmt.Println("  SDK:  npm install @agentwarden/sdk")
	fmt.Println()

	// Hot-reload config file
	if configFile != "" {
		if err := policyLoader.WatchConfig(configFile, func(path string) {
			if err := policyEngine.ReloadPolicies(); err != nil {
				logger.Error("hot-reload failed", "error", err)
			}
		}); err != nil {
			logger.Error("failed to watch config for hot-reload", "error", err)
		}
		defer policyLoader.StopWatch()
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down...")
		grpcEventServer.Stop()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	// Start gRPC server
	go func() {
		if err := grpcEventServer.Start(cfg.Server.GRPCPort); err != nil {
			logger.Error("gRPC server error", "port", cfg.Server.GRPCPort, "error", err)
		}
	}()

	// Start HTTP server
	logger.Info("starting HTTP server", "port", cfg.Server.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// ─── Init Commands ───

func runInit() error {
	// Generate agentwarden.yaml
	configPath := "agentwarden.yaml"
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("  ⚠ %s already exists (skipping)\n", configPath)
	} else {
		if err := config.GenerateDefault(configPath); err != nil {
			return err
		}
		fmt.Printf("  ✓ Generated %s\n", configPath)
	}

	// Create directory structure
	dirs := []string{"agents", "policies", "playbooks"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create %s/: %w", d, err)
		}
		fmt.Printf("  ✓ Created %s/\n", d)
	}

	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    agentwarden init agent <agent-id>       # Register an agent")
	fmt.Println("    agentwarden init policy <policy-name>   # Create an AI-judge policy")
	fmt.Println("    agentwarden init playbook loop          # Create a detection playbook")
	fmt.Println("    agentwarden start                       # Start the server")
	return nil
}

func runInitAgent(agentID string) error {
	dir := filepath.Join("agents", agentID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// AGENT.md
	agentPath := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte(mdloader.AgentMDTemplate(agentID)), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", agentPath)

	// EVOLVE.md
	evolvePath := filepath.Join(dir, "EVOLVE.md")
	if err := os.WriteFile(evolvePath, []byte(mdloader.EvolveMDTemplate(agentID)), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", evolvePath)

	// versions/v1/PROMPT.md
	versionsDir := filepath.Join(dir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		return err
	}
	promptPath := filepath.Join(versionsDir, "PROMPT.md")
	if err := os.WriteFile(promptPath, []byte(mdloader.PromptMDTemplate(agentID)), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", promptPath)

	fmt.Printf("\n  Agent %q scaffolded. Edit the MDs to define your agent.\n", agentID)
	return nil
}

func runInitPolicy(policyName string) error {
	dir := filepath.Join("policies", policyName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// policy.yaml
	yamlPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(yamlPath, []byte(mdloader.PolicyYAMLTemplate(policyName)), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", yamlPath)

	// POLICY.md
	mdPath := filepath.Join(dir, "POLICY.md")
	if err := os.WriteFile(mdPath, []byte(mdloader.PolicyMDTemplate(policyName)), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", mdPath)

	fmt.Printf("\n  Policy %q scaffolded. Edit POLICY.md to provide rich context for the AI judge.\n", policyName)
	return nil
}

func runInitPlaybook(detectionType string) error {
	if err := os.MkdirAll("playbooks", 0755); err != nil {
		return err
	}

	filename := strings.ToUpper(detectionType) + ".md"
	path := filepath.Join("playbooks", filename)
	content := mdloader.PlaybookTemplate(detectionType)
	if content == "" {
		return fmt.Errorf("unknown detection type %q. Supported: loop, spiral, budget_breach, drift", detectionType)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("  ✓ Created %s\n", path)
	fmt.Println("  Edit this playbook to customize how the AI reasons about detections.")
	return nil
}

// ─── Policy Validate ───

func runPolicyValidate(configFile string) error {
	path := configFile
	if path == "" {
		path = findConfigFile()
	}
	if path == "" {
		return fmt.Errorf("no config file found, run 'agentwarden init' to create one")
	}

	loader := config.NewLoader()
	if err := loader.Load(path); err != nil {
		fmt.Printf("✗ Invalid config: %s\n", err)
		return err
	}

	cfg := loader.Get()
	fmt.Printf("✓ Config file valid: %s\n", path)
	fmt.Printf("  Policies: %d\n", len(cfg.Policies))
	fmt.Printf("  Storage:  %s\n", cfg.Storage.Driver)
	fmt.Printf("  Port:     %d\n", cfg.Server.Port)

	// Validate CEL expressions
	evaluator, err := policy.NewCELEvaluator(nil)
	if err != nil {
		return fmt.Errorf("failed to create CEL evaluator: %w", err)
	}
	for _, p := range cfg.Policies {
		if p.Type == "ai-judge" || p.Condition == "" {
			continue
		}
		if _, err := evaluator.CompileExpression(p.Condition); err != nil {
			fmt.Printf("  ✗ Policy %q: invalid CEL expression: %s\n", p.Name, err)
		} else {
			fmt.Printf("  ✓ Policy %q: CEL expression valid\n", p.Name)
		}
	}

	// Validate referenced MDs
	var policyRefs []mdloader.PolicyRef
	for _, p := range cfg.Policies {
		policyRefs = append(policyRefs, mdloader.PolicyRef{
			Name:    p.Name,
			Type:    p.Type,
			Context: p.Context,
		})
	}
	var detectionRefs []mdloader.DetectionRef
	if cfg.Detection.Loop.Enabled {
		detectionRefs = append(detectionRefs, mdloader.DetectionRef{Name: "loop", Action: cfg.Detection.Loop.Action})
	}
	if cfg.Detection.Spiral.Enabled {
		detectionRefs = append(detectionRefs, mdloader.DetectionRef{Name: "spiral", Action: cfg.Detection.Spiral.Action})
	}

	result := mdloader.ValidateAll(cfg.AgentsDir, cfg.PoliciesDir, cfg.PlaybooksDir, policyRefs, detectionRefs)
	for _, e := range result.Errors {
		fmt.Printf("  ✗ %s\n", e)
	}
	for _, w := range result.Warnings {
		fmt.Printf("  ⚠ %s\n", w)
	}
	if len(result.Errors) == 0 {
		fmt.Println("  ✓ All referenced MD files exist")
	}

	return nil
}

// ─── Doctor ───

func runDoctor(port int, configFile string) error {
	fmt.Println("AgentWarden Doctor")
	fmt.Println("─────────────────")

	// Config check
	path := configFile
	if path == "" {
		path = findConfigFile()
	}
	if path != "" {
		fmt.Printf("✓ Config file found: %s\n", path)
	} else {
		fmt.Println("⚠ No config file found (will use defaults)")
	}

	// Directory checks
	for _, dir := range []string{"agents", "policies", "playbooks"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			fmt.Printf("✓ Directory exists: %s/\n", dir)
		} else {
			fmt.Printf("⚠ Missing directory: %s/ (run 'agentwarden init')\n", dir)
		}
	}

	// Server connectivity
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/health", p))
	if err != nil {
		fmt.Printf("✗ AgentWarden not running on port %d\n", p)
	} else {
		_ = resp.Body.Close()
		fmt.Printf("✓ HTTP server running on port %d\n", p)
	}

	// MD validation
	if path != "" {
		loader := config.NewLoader()
		if err := loader.Load(path); err == nil {
			cfg := loader.Get()
			var policyRefs []mdloader.PolicyRef
			for _, p := range cfg.Policies {
				policyRefs = append(policyRefs, mdloader.PolicyRef{Name: p.Name, Type: p.Type, Context: p.Context})
			}
			result := mdloader.ValidateAll(cfg.AgentsDir, cfg.PoliciesDir, cfg.PlaybooksDir, policyRefs, nil)
			for _, e := range result.Errors {
				fmt.Printf("✗ MD: %s\n", e)
			}
			for _, w := range result.Warnings {
				fmt.Printf("⚠ MD: %s\n", w)
			}
			if len(result.Errors) == 0 && len(result.Warnings) == 0 {
				fmt.Println("✓ All MD files valid")
			}
		}
	}

	return nil
}

// ─── Agent Show ───

func runAgentShow(port int, agentID string) error {
	// Try to load AGENT.md locally
	agentMDPath := filepath.Join("agents", agentID, "AGENT.md")
	if data, err := os.ReadFile(agentMDPath); err == nil {
		fmt.Println(string(data))
		fmt.Println()
	}

	// Get metrics from server
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/agents/%s", p, agentID))
	if err != nil {
		fmt.Printf("(Server not running — showing local files only)\n")
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var result map[string]interface{}
	_ = decodeJSON(resp, &result)

	if stats, ok := result["stats"].(map[string]interface{}); ok {
		fmt.Println("Metrics:")
		for k, v := range stats {
			fmt.Printf("  %-25s %v\n", k+":", v)
		}
	}
	return nil
}

// ─── Shared Helpers ───

func runStatus(port int) error {
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/stats", p))
	if err != nil {
		fmt.Printf("AgentWarden is not running on port %d\n", p)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	var stats map[string]interface{}
	if err := decodeJSON(resp, &stats); err != nil {
		return err
	}

	fmt.Println("AgentWarden Status")
	fmt.Println("─────────────────")
	for k, v := range stats {
		fmt.Printf("  %-20s %v\n", k+":", v)
	}
	return nil
}

func runTraceList(port int, cmd *cobra.Command) error {
	p := resolvePort(port)
	agent, _ := cmd.Flags().GetString("agent")
	url := fmt.Sprintf("http://localhost:%d/api/traces?limit=20", p)
	if agent != "" {
		url += "&agent_id=" + agent
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return err
	}

	traces, ok := result["traces"].([]interface{})
	if !ok || len(traces) == 0 {
		fmt.Println("No traces found.")
		return nil
	}

	fmt.Printf("%-26s %-12s %-15s %-10s %-10s %s\n", "TIMESTAMP", "SESSION", "TYPE", "STATUS", "COST", "AGENT")
	fmt.Println(strings.Repeat("─", 90))
	for _, t := range traces {
		m := t.(map[string]interface{})
		fmt.Printf("%-26v %-12v %-15v %-10v $%-9.4f %v\n",
			m["timestamp"], truncate(str(m["session_id"]), 12),
			m["action_type"], m["status"], num(m["cost_usd"]), m["agent_id"])
	}
	return nil
}

func runTraceShow(port int, sessionID string) error {
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/sessions/%s", p, sessionID))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return err
	}

	session := result["session"].(map[string]interface{})
	fmt.Printf("Session: %s\n", session["id"])
	fmt.Printf("Agent:   %s\n", session["agent_id"])
	fmt.Printf("Status:  %s\n", session["status"])
	fmt.Printf("Cost:    $%.4f\n", num(session["total_cost"]))
	fmt.Printf("Actions: %v\n\n", session["action_count"])

	traces, ok := result["traces"].([]interface{})
	if ok {
		for i, t := range traces {
			m := t.(map[string]interface{})
			fmt.Printf("  %d. [%s] %s %s → %s\n", i+1, m["timestamp"], m["action_type"], m["action_name"], m["status"])
		}
	}
	return nil
}

func runTraceSearch(port int, query string) error {
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/traces/search?q=%s&limit=20", p, query))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	_ = decodeJSON(resp, &result)

	traces, ok := result["traces"].([]interface{})
	if !ok || len(traces) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	fmt.Printf("Found %d matching traces:\n\n", len(traces))
	for _, t := range traces {
		m := t.(map[string]interface{})
		fmt.Printf("  [%s] %s %s (session: %s)\n", m["timestamp"], m["action_type"], m["action_name"], m["session_id"])
	}
	return nil
}

func runAgentList(port int) error {
	p := resolvePort(port)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/agents", p))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	_ = decodeJSON(resp, &result)

	agents, ok := result["agents"].([]interface{})
	if !ok || len(agents) == 0 {
		fmt.Println("No agents registered yet.")
		return nil
	}

	fmt.Printf("%-20s %-15s %-20s %s\n", "ID", "VERSION", "SESSIONS", "CREATED")
	fmt.Println(strings.Repeat("─", 70))
	for _, a := range agents {
		m := a.(map[string]interface{})
		fmt.Printf("%-20v %-15v %-20v %v\n", m["id"], m["current_version"], m["session_count"], m["created_at"])
	}
	return nil
}

func runMock(port int) error {
	p := resolvePort(port)
	fmt.Printf("Sending mock traffic to localhost:%d...\n\n", p)

	client := &http.Client{Timeout: 5 * time.Second}

	// Start a session
	sessionData := map[string]interface{}{
		"session_id":    "ses_mock001",
		"agent_id":      "mock-agent",
		"agent_version": "v1",
		"metadata":      map[string]string{"source": "mock"},
	}
	body, _ := json.Marshal(sessionData)
	resp, err := client.Post(fmt.Sprintf("http://localhost:%d/v1/sessions/start", p), "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to connect (is AgentWarden running?): %w", err)
	}
	_ = resp.Body.Close()
	fmt.Println("  ✓ Started session ses_mock001")

	// Send some actions
	actions := []map[string]interface{}{
		{"type": "tool.call", "name": "github.get_pr", "params_json": `{"repo":"myapp","pr":42}`, "target": "github.com"},
		{"type": "llm.chat", "name": "llm.chat.gpt-4o", "params_json": `{"model":"gpt-4o"}`, "target": ""},
		{"type": "api.request", "name": "stripe.refund", "params_json": `{"amount":2500}`, "target": "api.stripe.com"},
		{"type": "db.query", "name": "db.query", "params_json": `{"query":"SELECT * FROM users"}`, "target": "production.users"},
	}

	for _, action := range actions {
		event := map[string]interface{}{
			"session_id":    "ses_mock001",
			"agent_id":      "mock-agent",
			"agent_version": "v1",
			"action":        action,
			"context":       map[string]interface{}{"session_cost": 0.5, "session_action_count": 1, "session_duration_seconds": 10},
			"metadata":      map[string]string{},
		}
		body, _ := json.Marshal(event)
		resp, err := client.Post(fmt.Sprintf("http://localhost:%d/v1/events/evaluate", p), "application/json", strings.NewReader(string(body)))
		if err != nil {
			fmt.Printf("  ✗ %s: %s\n", action["name"], err)
			continue
		}
		var verdict map[string]interface{}
		_ = decodeJSON(resp, &verdict)
		_ = resp.Body.Close()
		fmt.Printf("  → %s → %v\n", action["name"], verdict["verdict"])
	}

	fmt.Println("\n  ✓ Mock traffic complete. Check the dashboard or trace list.")
	return nil
}

func findConfigFile() string {
	candidates := []string{
		"agentwarden.yaml",
		"agentwarden.yml",
		filepath.Join(os.Getenv("HOME"), ".config", "agentwarden", "config.yaml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func resolvePort(port int) int {
	if port == 0 {
		return 6777
	}
	return port
}

func decodeJSON(resp *http.Response, v interface{}) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func num(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
