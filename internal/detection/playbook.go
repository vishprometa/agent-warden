package detection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// PlaybookExecutor loads playbook MDs and calls an LLM for nuanced verdicts.
// When a detection fires with action "playbook", the engine delegates to this
// executor to get a context-aware decision rather than a hardcoded action.
type PlaybookExecutor struct {
	loadPlaybook func(name string) (string, error)
	httpClient   *http.Client
	defaultModel string
}

// PlaybookInput contains the detection context sent to the LLM along with
// the playbook markdown content.
type PlaybookInput struct {
	DetectionType string // "loop", "spiral", "budget_breach", "drift"
	ActionType    string // what was being done (e.g. "tool.call")
	ActionName    string // specific action (e.g. "shell_exec")
	RepeatCount   int    // how many repeats for loops
	Window        string // time window for the detection
	SessionID     string
	AgentID       string
	RecentActions []RecentAction         // last N actions for context
	Metadata      map[string]interface{} // additional detection-specific data
}

// RecentAction is a single action from the agent's recent history, provided
// to the LLM for context when evaluating a playbook.
type RecentAction struct {
	Type      string
	Name      string
	Params    string // JSON-encoded parameters
	Timestamp time.Time
	Status    string
}

// PlaybookVerdict is the parsed LLM response containing the recommended action.
type PlaybookVerdict struct {
	Action     string  // "allow", "pause", "terminate", "alert", "backoff"
	Reason     string  // explanation from LLM
	Confidence float64 // 0.0 - 1.0
	Details    string  // optional additional context
}

// NewPlaybookExecutor creates a PlaybookExecutor.
//
// loadFn maps a playbook name (e.g. "LOOP") to its markdown content. It is
// typically backed by the mdloader package. model is the default LLM model
// to use (overridable per-detection via config).
func NewPlaybookExecutor(loadFn func(string) (string, error), model string) *PlaybookExecutor {
	return &PlaybookExecutor{
		loadPlaybook: loadFn,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		defaultModel: model,
	}
}

// detectionTypeToPlaybook maps detection type strings to playbook file names.
var detectionTypeToPlaybook = map[string]string{
	"loop":          "LOOP",
	"spiral":        "SPIRAL",
	"budget_breach": "BUDGET_BREACH",
	"cost_anomaly":  "BUDGET_BREACH",
	"drift":         "DRIFT",
}

// Execute loads the playbook MD, builds a prompt with detection context, calls
// the LLM, and parses the verdict. If the LLM call fails for any reason, it
// returns an error so the caller can fall back to the detection's fallback_action.
func (p *PlaybookExecutor) Execute(ctx context.Context, input PlaybookInput) (*PlaybookVerdict, error) {
	// Resolve playbook name from detection type.
	playbookName, ok := detectionTypeToPlaybook[input.DetectionType]
	if !ok {
		return nil, fmt.Errorf("no playbook mapping for detection type %q", input.DetectionType)
	}

	// Load the playbook markdown content.
	playbookMD, err := p.loadPlaybook(playbookName)
	if err != nil {
		return nil, fmt.Errorf("failed to load playbook %q: %w", playbookName, err)
	}

	// Build the LLM prompt.
	systemPrompt := buildPlaybookSystemPrompt(playbookMD)
	userPrompt := buildPlaybookUserPrompt(input)

	// Call the LLM.
	response, err := p.callLLM(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed for playbook %q: %w", playbookName, err)
	}

	// Parse the verdict from the LLM response.
	verdict, err := parsePlaybookVerdict(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse playbook verdict: %w (raw response: %s)", err, truncate(response, 200))
	}

	return verdict, nil
}

// buildPlaybookSystemPrompt constructs the system prompt with the playbook
// content and response format instructions.
func buildPlaybookSystemPrompt(playbookMD string) string {
	return fmt.Sprintf(`You are a security and governance judge for an AI agent orchestration system called AgentWarden.

Your job is to read the playbook below and evaluate the detected anomaly. Based on the playbook guidance and the specific context of the detection, decide what action to take.

## PLAYBOOK

%s

## RESPONSE FORMAT

You MUST respond with a single JSON object (no markdown fencing, no extra text):
{"action": "<allow|pause|terminate|alert|backoff>", "reason": "<concise explanation>", "confidence": <0.0-1.0>}

- "allow": The detected pattern is benign; let the agent continue.
- "pause": Temporarily halt the agent for human review.
- "terminate": Kill the agent session immediately.
- "alert": Send an alert but let the agent continue.
- "backoff": Introduce a delay before the next action (e.g. exponential backoff).

Choose the action that best matches the playbook's guidance given the specific context.`, playbookMD)
}

// buildPlaybookUserPrompt constructs the user message with detection context.
func buildPlaybookUserPrompt(input PlaybookInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Detection Event\n\n")
	fmt.Fprintf(&b, "- **Type**: %s\n", input.DetectionType)
	fmt.Fprintf(&b, "- **Action**: %s / %s\n", input.ActionType, input.ActionName)
	if input.RepeatCount > 0 {
		fmt.Fprintf(&b, "- **Repeat count**: %d\n", input.RepeatCount)
	}
	if input.Window != "" {
		fmt.Fprintf(&b, "- **Window**: %s\n", input.Window)
	}
	fmt.Fprintf(&b, "- **Session**: %s\n", input.SessionID)
	fmt.Fprintf(&b, "- **Agent**: %s\n", input.AgentID)

	if len(input.Metadata) > 0 {
		fmt.Fprintf(&b, "\n## Additional Context\n\n")
		for k, v := range input.Metadata {
			fmt.Fprintf(&b, "- **%s**: %v\n", k, v)
		}
	}

	if len(input.RecentActions) > 0 {
		fmt.Fprintf(&b, "\n## Recent Actions (most recent first)\n\n")
		for i, a := range input.RecentActions {
			fmt.Fprintf(&b, "%d. [%s] %s / %s", i+1, a.Timestamp.Format(time.RFC3339), a.Type, a.Name)
			if a.Status != "" {
				fmt.Fprintf(&b, " (status: %s)", a.Status)
			}
			if a.Params != "" && a.Params != "{}" {
				fmt.Fprintf(&b, "\n   params: %s", truncate(a.Params, 200))
			}
			fmt.Fprintf(&b, "\n")
		}
	}

	fmt.Fprintf(&b, "\nBased on the playbook, what action should be taken?")
	return b.String()
}

// chatCompletionRequest is the OpenAI-compatible chat completions request body.
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the OpenAI-compatible chat completions response.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// callLLM sends a chat completion request to the OpenAI-compatible API
// configured via environment variables.
func (p *PlaybookExecutor) callLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	baseURL := os.Getenv("AGENTWARDEN_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := os.Getenv("AGENTWARDEN_LLM_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("AGENTWARDEN_LLM_API_KEY environment variable is not set")
	}

	model := p.defaultModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1,
		MaxTokens:   256,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("status %d", resp.StatusCode)
		if result.Error != nil {
			errMsg += ": " + result.Error.Message
		}
		return "", fmt.Errorf("LLM API error: %s", errMsg)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// llmVerdictResponse is the JSON structure expected from the LLM.
type llmVerdictResponse struct {
	Action     string  `json:"action"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// parsePlaybookVerdict extracts a PlaybookVerdict from the raw LLM response text.
// It handles optional markdown code fencing around the JSON.
func parsePlaybookVerdict(raw string) (*PlaybookVerdict, error) {
	// Strip markdown code fences if present.
	cleaned := raw
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var parsed llmVerdictResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate action value.
	validActions := map[string]bool{
		"allow": true, "pause": true, "terminate": true,
		"alert": true, "backoff": true,
	}
	action := strings.ToLower(strings.TrimSpace(parsed.Action))
	if !validActions[action] {
		return nil, fmt.Errorf("invalid action %q; must be one of: allow, pause, terminate, alert, backoff", parsed.Action)
	}

	// Clamp confidence to [0, 1].
	confidence := parsed.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return &PlaybookVerdict{
		Action:     action,
		Reason:     parsed.Reason,
		Confidence: confidence,
	}, nil
}

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
