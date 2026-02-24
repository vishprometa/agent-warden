package cost

import (
	"encoding/json"
	"strings"
)

// TokenCounter estimates token counts from request/response bodies.
type TokenCounter struct{}

// NewTokenCounter creates a new token counter.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{}
}

// CountRequestTokens estimates input tokens from a chat completion request body.
func (tc *TokenCounter) CountRequestTokens(body []byte) int {
	var req struct {
		Messages []struct {
			Content interface{} `json:"content"`
		} `json:"messages"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return estimateTokens(string(body))
	}

	total := 0
	for _, msg := range req.Messages {
		switch v := msg.Content.(type) {
		case string:
			total += estimateTokens(v)
		default:
			b, _ := json.Marshal(v)
			total += estimateTokens(string(b))
		}
		total += 4 // message overhead
	}
	return total
}

// CountResponseTokens extracts token counts from a chat completion response.
// Prefers provider-reported usage; falls back to estimation.
func (tc *TokenCounter) CountResponseTokens(body []byte) (inputTokens, outputTokens int) {
	var resp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			// Anthropic format
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		// Anthropic format
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, estimateTokens(string(body))
	}

	// Provider-reported (OpenAI format)
	if resp.Usage.PromptTokens > 0 {
		return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
	}

	// Provider-reported (Anthropic format)
	if resp.Usage.InputTokens > 0 {
		return resp.Usage.InputTokens, resp.Usage.OutputTokens
	}

	// Fallback: estimate from response content
	outputText := ""
	for _, choice := range resp.Choices {
		outputText += choice.Message.Content
	}
	for _, content := range resp.Content {
		outputText += content.Text
	}

	return 0, estimateTokens(outputText)
}

// CountStreamingTokens extracts token counts from accumulated SSE data.
// Looks for the final usage chunk in streaming responses.
func (tc *TokenCounter) CountStreamingTokens(chunks []byte) (inputTokens, outputTokens int) {
	// Look for usage data in the final chunk
	lines := strings.Split(string(chunks), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimPrefix(lines[i], "data: ")
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var chunk struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				InputTokens      int `json:"input_tokens"`
				OutputTokens     int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err == nil {
			if chunk.Usage.PromptTokens > 0 {
				return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
			}
			if chunk.Usage.InputTokens > 0 {
				return chunk.Usage.InputTokens, chunk.Usage.OutputTokens
			}
		}
	}

	// Fallback: rough estimate from chunk size
	return 0, estimateTokens(string(chunks))
}

// estimateTokens gives a rough token count (~4 chars per token for English).
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	// ~4 chars per token is a reasonable approximation
	return (len(text) + 3) / 4
}
