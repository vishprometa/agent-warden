package policy

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

// AIJudge evaluates actions using an LLM with rich POLICY.md context.
// It is used by ai-judge type policies in the governance pipeline to make
// nuanced allow/deny decisions that deterministic CEL rules cannot express.
type AIJudge struct {
	loadPolicyMD func(path string) (string, error)
	httpClient   *http.Client
	defaultModel string
}

// AIJudgeInput contains everything the judge LLM needs to evaluate an action.
type AIJudgeInput struct {
	PolicyName string
	PolicyMD   string // loaded POLICY.md content
	Model      string // LLM model override; falls back to AIJudge.defaultModel
	Effect     string // effect to apply if judge says deny (e.g. "deny", "terminate")
	ActionType string
	ActionName string
	Params     map[string]interface{}
	Target     string
	SessionID  string
	AgentID    string
	Metadata   map[string]interface{}
}

// AIJudgeResult is the outcome of an AI judge evaluation.
type AIJudgeResult struct {
	ShouldDeny bool
	Reason     string
	Confidence float64
}

// NewAIJudge creates an AIJudge.
//
// loadFn reads a POLICY.md file given its path and returns the content.
// defaultModel is used when the policy config does not specify a model.
func NewAIJudge(loadFn func(path string) (string, error), defaultModel string) *AIJudge {
	return &AIJudge{
		loadPolicyMD: loadFn,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		defaultModel: defaultModel,
	}
}

// Evaluate sends the action context and POLICY.md content to the judge LLM
// and returns whether the action should be denied.
func (j *AIJudge) Evaluate(ctx context.Context, input AIJudgeInput) (*AIJudgeResult, error) {
	systemPrompt := buildJudgeSystemPrompt(input.PolicyMD)
	userPrompt := buildJudgeUserPrompt(input)

	model := input.Model
	if model == "" {
		model = j.defaultModel
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	response, err := j.callLLM(ctx, model, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI judge LLM call failed for policy %q: %w", input.PolicyName, err)
	}

	result, err := parseJudgeResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI judge response for policy %q: %w (raw: %s)",
			input.PolicyName, err, truncateStr(response, 200))
	}

	return result, nil
}

// buildJudgeSystemPrompt constructs the system message with the policy context
// and response format instructions.
func buildJudgeSystemPrompt(policyMD string) string {
	return fmt.Sprintf(`You are a security policy judge for an AI agent governance system called AgentWarden.

Read the policy context below and evaluate whether the given action violates the policy. Consider the intent, scope, and potential risk of the action.

## POLICY CONTEXT

%s

## RESPONSE FORMAT

You MUST respond with a single JSON object (no markdown fencing, no extra text):
{"deny": true/false, "reason": "<concise explanation>", "confidence": <0.0-1.0>}

- Set "deny" to true ONLY if the action clearly violates the policy.
- Set "deny" to false if the action is acceptable or ambiguous (err on the side of allowing).
- "confidence" reflects how certain you are in your judgment (1.0 = completely certain).
- "reason" should be a brief, actionable explanation.`, policyMD)
}

// buildJudgeUserPrompt constructs the user message with the action details
// for the judge to evaluate.
func buildJudgeUserPrompt(input AIJudgeInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Action Under Review\n\n")
	fmt.Fprintf(&b, "- **Policy**: %s\n", input.PolicyName)
	fmt.Fprintf(&b, "- **Action type**: %s\n", input.ActionType)
	fmt.Fprintf(&b, "- **Action name**: %s\n", input.ActionName)
	if input.Target != "" {
		fmt.Fprintf(&b, "- **Target**: %s\n", input.Target)
	}
	fmt.Fprintf(&b, "- **Agent**: %s\n", input.AgentID)
	fmt.Fprintf(&b, "- **Session**: %s\n", input.SessionID)

	if len(input.Params) > 0 {
		paramsJSON, err := json.MarshalIndent(input.Params, "  ", "  ")
		if err == nil {
			fmt.Fprintf(&b, "\n### Parameters\n\n```json\n  %s\n```\n", string(paramsJSON))
		}
	}

	if len(input.Metadata) > 0 {
		fmt.Fprintf(&b, "\n### Metadata\n\n")
		for k, v := range input.Metadata {
			fmt.Fprintf(&b, "- **%s**: %v\n", k, v)
		}
	}

	fmt.Fprintf(&b, "\nDoes this action violate the policy? Respond with JSON.")
	return b.String()
}

// judgeChatRequest is the OpenAI-compatible chat completions request body.
type judgeChatRequest struct {
	Model       string             `json:"model"`
	Messages    []judgeChatMessage  `json:"messages"`
	Temperature float64            `json:"temperature"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
}

type judgeChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// judgeChatResponse is the OpenAI-compatible chat completions response body.
type judgeChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// callLLM sends a chat completion request to the OpenAI-compatible API.
func (j *AIJudge) callLLM(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	baseURL := os.Getenv("AGENTWARDEN_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := os.Getenv("AGENTWARDEN_LLM_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("AGENTWARDEN_LLM_API_KEY environment variable is not set")
	}

	reqBody := judgeChatRequest{
		Model: model,
		Messages: []judgeChatMessage{
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

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var result judgeChatResponse
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

// judgeResponseJSON is the JSON structure expected from the LLM.
type judgeResponseJSON struct {
	Deny       bool    `json:"deny"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// parseJudgeResponse extracts an AIJudgeResult from the raw LLM response text.
// It handles optional markdown code fencing around the JSON.
func parseJudgeResponse(raw string) (*AIJudgeResult, error) {
	// Strip any surrounding text to extract just the JSON object.
	cleaned := raw
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var parsed judgeResponseJSON
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Clamp confidence to [0, 1].
	confidence := parsed.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return &AIJudgeResult{
		ShouldDeny: parsed.Deny,
		Reason:     parsed.Reason,
		Confidence: confidence,
	}, nil
}

// truncateStr returns the first n characters of s, appending "..." if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
