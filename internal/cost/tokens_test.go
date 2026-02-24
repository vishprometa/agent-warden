package cost

import (
	"testing"
)

func TestCountRequestTokens_ChatMessages(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello, how are you?"}
		]
	}`)

	tokens := tc.CountRequestTokens(body)
	if tokens <= 0 {
		t.Errorf("CountRequestTokens returned %d, expected > 0", tokens)
	}
}

func TestCountRequestTokens_EmptyBody(t *testing.T) {
	tc := NewTokenCounter()

	tokens := tc.CountRequestTokens([]byte{})
	if tokens != 0 {
		t.Errorf("CountRequestTokens(empty) = %d, want 0", tokens)
	}
}

func TestCountRequestTokens_InvalidJSON(t *testing.T) {
	tc := NewTokenCounter()

	// Invalid JSON falls back to estimateTokens on the raw body
	body := []byte(`not valid json at all`)
	tokens := tc.CountRequestTokens(body)
	if tokens <= 0 {
		t.Errorf("CountRequestTokens(invalid JSON) = %d, expected > 0 (fallback estimation)", tokens)
	}
}

func TestCountRequestTokens_NoMessages(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`{"model": "gpt-4"}`)
	tokens := tc.CountRequestTokens(body)
	// No messages, so 0 tokens from message counting
	if tokens != 0 {
		t.Errorf("CountRequestTokens(no messages) = %d, want 0", tokens)
	}
}

func TestCountRequestTokens_MessageOverhead(t *testing.T) {
	tc := NewTokenCounter()

	// Each message adds 4 tokens overhead + estimated content tokens
	body := []byte(`{
		"messages": [
			{"role": "user", "content": "Hi"}
		]
	}`)

	tokens := tc.CountRequestTokens(body)
	// "Hi" = ~1 token estimate (2 chars + 3 / 4 = 1) + 4 overhead = 5
	if tokens < 4 {
		t.Errorf("CountRequestTokens should include message overhead, got %d", tokens)
	}
}

func TestCountResponseTokens_OpenAIFormat(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`{
		"id": "chatcmpl-abc123",
		"object": "chat.completion",
		"usage": {
			"prompt_tokens": 42,
			"completion_tokens": 100,
			"total_tokens": 142
		},
		"choices": [
			{"message": {"role": "assistant", "content": "Hello!"}}
		]
	}`)

	inputTokens, outputTokens := tc.CountResponseTokens(body)
	if inputTokens != 42 {
		t.Errorf("inputTokens = %d, want 42", inputTokens)
	}
	if outputTokens != 100 {
		t.Errorf("outputTokens = %d, want 100", outputTokens)
	}
}

func TestCountResponseTokens_AnthropicFormat(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`{
		"id": "msg_abc123",
		"type": "message",
		"usage": {
			"input_tokens": 50,
			"output_tokens": 200
		},
		"content": [
			{"type": "text", "text": "Hello from Claude!"}
		]
	}`)

	inputTokens, outputTokens := tc.CountResponseTokens(body)
	if inputTokens != 50 {
		t.Errorf("inputTokens = %d, want 50", inputTokens)
	}
	if outputTokens != 200 {
		t.Errorf("outputTokens = %d, want 200", outputTokens)
	}
}

func TestCountResponseTokens_NoUsage_FallbackEstimate(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`{
		"choices": [
			{"message": {"role": "assistant", "content": "This is a response without usage data provided."}}
		]
	}`)

	inputTokens, outputTokens := tc.CountResponseTokens(body)
	if inputTokens != 0 {
		t.Errorf("inputTokens = %d, want 0 (fallback)", inputTokens)
	}
	if outputTokens <= 0 {
		t.Errorf("outputTokens = %d, want > 0 (estimated from content)", outputTokens)
	}
}

func TestCountResponseTokens_EmptyBody(t *testing.T) {
	tc := NewTokenCounter()

	inputTokens, outputTokens := tc.CountResponseTokens([]byte{})
	// Empty body -> unmarshal fails -> fallback estimate on empty string -> 0
	if inputTokens != 0 {
		t.Errorf("inputTokens = %d, want 0", inputTokens)
	}
	if outputTokens != 0 {
		t.Errorf("outputTokens = %d, want 0", outputTokens)
	}
}

func TestCountResponseTokens_InvalidJSON(t *testing.T) {
	tc := NewTokenCounter()

	body := []byte(`not json`)
	inputTokens, outputTokens := tc.CountResponseTokens(body)
	if inputTokens != 0 {
		t.Errorf("inputTokens = %d, want 0", inputTokens)
	}
	// Falls back to estimating from the raw body text
	if outputTokens <= 0 {
		t.Errorf("outputTokens = %d, want > 0 (fallback estimate)", outputTokens)
	}
}

func TestCountStreamingTokens_UsageInLastChunk(t *testing.T) {
	tc := NewTokenCounter()

	chunks := []byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}
data: {"choices":[{"delta":{"content":" there"}}]}
data: {"usage":{"prompt_tokens":30,"completion_tokens":15}}
data: [DONE]
`)

	inputTokens, outputTokens := tc.CountStreamingTokens(chunks)
	if inputTokens != 30 {
		t.Errorf("inputTokens = %d, want 30", inputTokens)
	}
	if outputTokens != 15 {
		t.Errorf("outputTokens = %d, want 15", outputTokens)
	}
}

func TestCountStreamingTokens_AnthropicUsage(t *testing.T) {
	tc := NewTokenCounter()

	chunks := []byte(`data: {"type":"content_block_delta","delta":{"text":"Hello"}}
data: {"type":"message_delta","usage":{"input_tokens":25,"output_tokens":10}}
`)

	inputTokens, outputTokens := tc.CountStreamingTokens(chunks)
	if inputTokens != 25 {
		t.Errorf("inputTokens = %d, want 25", inputTokens)
	}
	if outputTokens != 10 {
		t.Errorf("outputTokens = %d, want 10", outputTokens)
	}
}

func TestCountStreamingTokens_NoUsage(t *testing.T) {
	tc := NewTokenCounter()

	chunks := []byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}
data: {"choices":[{"delta":{"content":" world"}}]}
data: [DONE]
`)

	inputTokens, outputTokens := tc.CountStreamingTokens(chunks)
	if inputTokens != 0 {
		t.Errorf("inputTokens = %d, want 0", inputTokens)
	}
	// Fallback: estimates from chunk size
	if outputTokens <= 0 {
		t.Errorf("outputTokens = %d, want > 0 (fallback estimate)", outputTokens)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty string", "", 0},
		{"short text", "hi", 1},       // (2+3)/4 = 1
		{"4 chars", "test", 1},        // (4+3)/4 = 1
		{"8 chars", "testtest", 2},    // (8+3)/4 = 2
		{"12 chars", "hello, world", 3}, // (12+3)/4 = 3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got != tt.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}
