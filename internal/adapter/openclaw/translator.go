package openclaw

import (
	"encoding/json"
	"strings"

	"github.com/agentwarden/agentwarden/internal/policy"
)

// OpenClaw event type → AgentWarden action type mapping.
var actionTypeMap = map[string]string{
	"tool.shell_exec":   "tool.call",
	"tool.file_write":   "file.write",
	"tool.file_read":    "file.read",
	"tool.file_delete":  "file.delete",
	"skill.install":     "skill.install",
	"skill.invoke":      "skill.invoke",
	"skill.uninstall":   "skill.uninstall",
	"agent.spawn":       "agent.spawn",
	"agent.fund":        "financial.transfer",
	"message.send":      "message.send",
	"message.broadcast": "message.broadcast",
	"llm.chat":          "llm.chat",
	"llm.completion":    "llm.chat",
	"heartbeat.trigger": "heartbeat.wake",
	"heartbeat.wake":    "heartbeat.wake",
	"config.modify":     "config.change",
	"config.set":        "config.change",
	"api.request":       "api.call",
	"db.query":          "db.query",
	"web.navigate":      "web.navigate",
	"web.screenshot":    "web.screenshot",
}

// TranslateEvent converts an OpenClaw gateway message into an AgentWarden
// ActionContext suitable for policy evaluation.
func TranslateEvent(msg map[string]interface{}, agentID, sessionID string) policy.ActionContext {
	ctx := policy.ActionContext{
		Agent: policy.AgentInfo{
			ID:   agentID,
			Name: agentID,
		},
		Session: policy.SessionInfo{
			ID:      sessionID,
			AgentID: agentID,
		},
	}

	// Extract the event type. OpenClaw uses "type" or "event" field.
	eventType := strVal(msg, "type")
	if eventType == "" {
		eventType = strVal(msg, "event")
	}

	// Map to AgentWarden action type.
	actionType, ok := actionTypeMap[eventType]
	if !ok {
		// Try to infer from the event type structure.
		actionType = inferActionType(eventType)
	}

	ctx.Action = policy.ActionInfo{
		Type:   actionType,
		Name:   strVal(msg, "name"),
		Target: strVal(msg, "target"),
		Params: extractParams(msg),
	}

	// If name is empty, use the event type as the name.
	if ctx.Action.Name == "" {
		ctx.Action.Name = eventType
	}

	// Extract metadata.
	if meta, ok := msg["metadata"].(map[string]interface{}); ok {
		ctx.Metadata = meta
	}

	// Extract context/session info if provided.
	if ctxData, ok := msg["context"].(map[string]interface{}); ok {
		if cost, ok := ctxData["session_cost"].(float64); ok {
			ctx.Session.Cost = cost
		}
		if count, ok := ctxData["action_count"].(float64); ok {
			ctx.Session.ActionCount = int(count)
		}
	}

	// Extract specific params for capability checking.
	enrichParams(&ctx, msg)

	return ctx
}

// inferActionType tries to map unknown event types to AgentWarden types.
func inferActionType(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "tool."):
		return "tool.call"
	case strings.HasPrefix(eventType, "llm."):
		return "llm.chat"
	case strings.HasPrefix(eventType, "message."):
		return "message.send"
	case strings.HasPrefix(eventType, "skill."):
		return "skill.invoke"
	case strings.HasPrefix(eventType, "agent."):
		return "agent.spawn"
	case strings.HasPrefix(eventType, "config."):
		return "config.change"
	case strings.HasPrefix(eventType, "web."):
		return "web.navigate"
	case strings.HasPrefix(eventType, "db."):
		return "db.query"
	case strings.HasPrefix(eventType, "api."):
		return "api.call"
	case strings.HasPrefix(eventType, "file."):
		return "file.write"
	default:
		return eventType
	}
}

// extractParams pulls the params/arguments from the message.
func extractParams(msg map[string]interface{}) map[string]interface{} {
	// OpenClaw uses various field names for parameters.
	for _, key := range []string{"params", "arguments", "args", "data", "payload"} {
		if params, ok := msg[key].(map[string]interface{}); ok {
			return params
		}
		// Handle JSON-encoded params.
		if paramsStr, ok := msg[key].(string); ok && paramsStr != "" {
			var params map[string]interface{}
			if json.Unmarshal([]byte(paramsStr), &params) == nil {
				return params
			}
		}
	}
	return nil
}

// enrichParams adds specific fields that capability and policy checks need.
func enrichParams(ctx *policy.ActionContext, msg map[string]interface{}) {
	if ctx.Action.Params == nil {
		ctx.Action.Params = make(map[string]interface{})
	}

	switch ctx.Action.Type {
	case "tool.call":
		// Extract shell command if present.
		if cmd, ok := ctx.Action.Params["command"].(string); ok {
			ctx.Action.Params["command"] = cmd
		}

	case "file.write", "file.read", "file.delete":
		// Extract file path.
		for _, key := range []string{"path", "file", "filepath", "filename"} {
			if path, ok := ctx.Action.Params[key].(string); ok {
				ctx.Action.Params["path"] = path
				break
			}
		}

	case "message.send", "message.broadcast":
		// Extract channel and recipient.
		if ch, ok := ctx.Action.Params["channel"].(string); ok {
			ctx.Action.Params["channel"] = ch
			ctx.Action.Target = ch
		}
		if to, ok := ctx.Action.Params["to"].(string); ok {
			ctx.Action.Target = to
		}

	case "financial.transfer":
		// Extract amount.
		if amount, ok := ctx.Action.Params["amount"].(float64); ok {
			ctx.Action.Params["amount"] = amount
		}

	case "agent.spawn":
		// Extract child agent configuration.
		if childID, ok := ctx.Action.Params["agent_id"].(string); ok {
			ctx.Action.Target = childID
		}

	case "skill.install":
		// Extract skill identifier.
		if skillID, ok := ctx.Action.Params["skill"].(string); ok {
			ctx.Action.Target = skillID
		}

	case "web.navigate":
		// Extract URL.
		if url, ok := ctx.Action.Params["url"].(string); ok {
			ctx.Action.Params["domain"] = url
			ctx.Action.Target = url
		}
	}
}

// strVal safely extracts a string value from a map.
func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
