package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// LLMClient provides chat completion access for the analyzer and proposer.
// It communicates with any OpenAI-compatible API endpoint.
type LLMClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// chatRequest is the request body for /chat/completions.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the response body from /chat/completions.
type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// NewLLMClient creates a new LLM client.
// It reads AGENTWARDEN_LLM_BASE_URL (default: "https://api.openai.com/v1")
// and AGENTWARDEN_LLM_API_KEY from environment variables.
func NewLLMClient(model string) *LLMClient {
	baseURL := os.Getenv("AGENTWARDEN_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	apiKey := os.Getenv("AGENTWARDEN_LLM_API_KEY")

	return &LLMClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Chat sends a chat completion request and returns the assistant's response text.
// It constructs a two-message conversation (system + user) and calls the
// configured LLM endpoint.
func (c *LLMClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("AGENTWARDEN_LLM_API_KEY is not set")
	}

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("llm error [%s]: %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
