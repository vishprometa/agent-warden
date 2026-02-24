package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// Classifier inspects intercepted HTTP requests and categorizes them into
// ActionTypes based on URL path patterns and request body content.
type Classifier struct {
	logger *slog.Logger
}

// NewClassifier creates a new request classifier.
func NewClassifier(logger *slog.Logger) *Classifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Classifier{
		logger: logger.With("component", "proxy.Classifier"),
	}
}

// classificationRule maps a URL path suffix to an action type and name.
type classificationRule struct {
	pathSuffix string
	actionType trace.ActionType
	actionName string
}

// Static classification rules ordered by specificity (most specific first).
var classificationRules = []classificationRule{
	{pathSuffix: "/chat/completions", actionType: trace.ActionLLMChat, actionName: "chat_completion"},
	{pathSuffix: "/completions", actionType: trace.ActionLLMChat, actionName: "completion"},
	{pathSuffix: "/embeddings", actionType: trace.ActionLLMEmbed, actionName: "embedding"},
	{pathSuffix: "/messages", actionType: trace.ActionLLMChat, actionName: "messages"},          // Anthropic Messages API
	{pathSuffix: ":generateContent", actionType: trace.ActionLLMChat, actionName: "generate"},   // Gemini
	{pathSuffix: ":streamGenerateContent", actionType: trace.ActionLLMChat, actionName: "generate_stream"}, // Gemini streaming
	{pathSuffix: "/images/generations", actionType: trace.ActionAPIRequest, actionName: "image_generation"},
	{pathSuffix: "/audio/transcriptions", actionType: trace.ActionAPIRequest, actionName: "audio_transcription"},
	{pathSuffix: "/audio/speech", actionType: trace.ActionAPIRequest, actionName: "audio_speech"},
	{pathSuffix: "/moderations", actionType: trace.ActionAPIRequest, actionName: "moderation"},
}

// modelExtractionBody is used to extract the model field from a JSON request body.
type modelExtractionBody struct {
	Model string `json:"model"`
}

// Classify inspects the request URL path and body to determine the action type,
// a human-readable action name, and the model being used. Returns:
//   - actionType: the categorized ActionType (e.g., llm.chat, llm.embedding)
//   - actionName: a descriptive name for the action (e.g., "chat_completion")
//   - model: the model string extracted from the request body, if any
func (c *Classifier) Classify(req *http.Request, body []byte) (trace.ActionType, string, string) {
	path := req.URL.Path

	// Match against known URL patterns.
	actionType := trace.ActionAPIRequest
	actionName := "api_request"

	for _, rule := range classificationRules {
		if strings.HasSuffix(path, rule.pathSuffix) {
			actionType = rule.actionType
			actionName = rule.actionName
			break
		}
	}

	// Extract model from request body for LLM requests.
	model := extractModel(body)

	// If the path didn't match any rule but the body contains a model field,
	// treat it as an API request with model context.
	if model != "" && actionName == "api_request" {
		actionName = "api_request:" + model
	}

	c.logger.Debug("classified request",
		"path", path,
		"method", req.Method,
		"action_type", actionType,
		"action_name", actionName,
		"model", model,
	)

	return actionType, actionName, model
}

// extractModel attempts to read the "model" field from a JSON request body.
// Returns an empty string if the body is not JSON or has no model field.
func extractModel(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var parsed modelExtractionBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return parsed.Model
}
