package proxy

import (
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// PolicyEngineAdapter bridges the policy.Engine to the proxy's PolicyEngine interface.
type PolicyEngineAdapter struct {
	Engine *policy.Engine
}

func (a *PolicyEngineAdapter) Evaluate(ctx ActionContext) (trace.TraceStatus, string, string) {
	policyCtx := policy.ActionContext{
		Action: policy.ActionInfo{
			Type:   string(ctx.ActionType),
			Name:   ctx.ActionName,
			Target: "",
		},
		Session: policy.SessionInfo{
			ID:          ctx.SessionID,
			AgentID:     ctx.AgentID,
			Cost:        ctx.SessionCost,
			ActionCount: ctx.ActionCount,
			ActionCountByType: func(actionType, window string) int {
				if ctx.ActionWindow != nil {
					d, err := time.ParseDuration(window)
					if err != nil {
						return 0
					}
					return ctx.ActionWindow(actionType, d)
				}
				return 0
			},
		},
		Agent: policy.AgentInfo{
			ID:   ctx.AgentID,
			Name: ctx.AgentID,
		},
		Request: policy.RequestInfo{
			Body: ctx.RequestBody,
		},
	}

	result := a.Engine.Evaluate(policyCtx)

	switch result.Effect {
	case policy.EffectDeny:
		return trace.StatusDenied, result.PolicyName, result.Message
	case policy.EffectTerminate:
		return trace.StatusTerminated, result.PolicyName, result.Message
	case policy.EffectThrottle:
		// For throttle, we treat it as allowed but the proxy could add a delay
		return trace.StatusAllowed, result.PolicyName, result.Message
	case policy.EffectApprove:
		return trace.StatusPending, result.PolicyName, result.Message
	default:
		return trace.StatusAllowed, "", ""
	}
}

// DetectionEngineAdapter bridges detection.Engine to the proxy's DetectionEngine interface.
type DetectionEngineAdapter struct {
	Engine *detection.Engine
}

func (a *DetectionEngineAdapter) Feed(t *trace.Trace) string {
	event := detection.ActionEvent{
		SessionID:  t.SessionID,
		AgentID:    t.AgentID,
		ActionType: string(t.ActionType),
		ActionName: t.ActionName,
		Signature:  string(t.ActionType) + "|" + t.ActionName + "|" + t.Model,
		CostUSD:    t.CostUSD,
	}
	// Extract content for spiral detection
	if t.ResponseBody != nil {
		event.Content = string(t.ResponseBody)
	}
	a.Engine.Analyze(event)
	return ""
}

// CostTrackerAdapter bridges cost.Tracker to the proxy's CostTracker interface.
type CostTrackerAdapter struct {
	Counter *cost.TokenCounter
}

func (a *CostTrackerAdapter) CountTokens(model string, requestBody, responseBody []byte) (int, int) {
	inputTokens := a.Counter.CountRequestTokens(requestBody)
	inputFromResp, outputTokens := a.Counter.CountResponseTokens(responseBody)
	if inputFromResp > 0 {
		inputTokens = inputFromResp
	}
	return inputTokens, outputTokens
}

func (a *CostTrackerAdapter) CalculateCost(model string, tokensIn, tokensOut int) float64 {
	return cost.CalculateCost(model, tokensIn, tokensOut)
}

// AlertManagerAdapter bridges alert.Manager to the proxy's AlertManager interface.
type AlertManagerAdapter struct {
	Manager *alert.Manager
}

func (a *AlertManagerAdapter) SendAlert(severity, message string, details map[string]any) {
	a.Manager.Send(alert.Alert{
		Type:     "proxy",
		Severity: severity,
		Title:    "AgentWarden Alert",
		Message:  message,
		Details:  details,
	})
}
