package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentwarden/agentwarden/internal/trace"
)

func TestClassify_ChatCompletions(t *testing.T) {
	c := NewClassifier(nil)

	tests := []struct {
		name       string
		path       string
		body       string
		wantType   trace.ActionType
		wantAction string
		wantModel  string
	}{
		{
			name:       "OpenAI chat completions",
			path:       "/v1/chat/completions",
			body:       `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "chat_completion",
			wantModel:  "gpt-4o",
		},
		{
			name:       "chat completions without version prefix",
			path:       "/chat/completions",
			body:       `{"model":"gpt-3.5-turbo"}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "chat_completion",
			wantModel:  "gpt-3.5-turbo",
		},
		{
			name:       "embeddings endpoint",
			path:       "/v1/embeddings",
			body:       `{"model":"text-embedding-ada-002","input":"hello world"}`,
			wantType:   trace.ActionLLMEmbed,
			wantAction: "embedding",
			wantModel:  "text-embedding-ada-002",
		},
		{
			name:       "Anthropic messages API",
			path:       "/v1/messages",
			body:       `{"model":"claude-3-opus","messages":[{"role":"user","content":"hi"}]}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "messages",
			wantModel:  "claude-3-opus",
		},
		{
			name:       "Gemini generateContent",
			path:       "/v1beta/models/gemini-1.5-pro:generateContent",
			body:       `{"model":"gemini-1.5-pro"}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "generate",
			wantModel:  "gemini-1.5-pro",
		},
		{
			name:       "Gemini streamGenerateContent",
			path:       "/v1beta/models/gemini-1.5-pro:streamGenerateContent",
			body:       `{"model":"gemini-1.5-pro"}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "generate_stream",
			wantModel:  "gemini-1.5-pro",
		},
		{
			name:       "image generation",
			path:       "/v1/images/generations",
			body:       `{"model":"dall-e-3","prompt":"a cat"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "image_generation",
			wantModel:  "dall-e-3",
		},
		{
			name:       "unknown path without model",
			path:       "/v1/some/random/endpoint",
			body:       `{"key":"value"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "api_request",
			wantModel:  "",
		},
		{
			name:       "unknown path with model field",
			path:       "/v1/some/random/endpoint",
			body:       `{"model":"custom-model"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "api_request:custom-model",
			wantModel:  "custom-model",
		},
		{
			name:       "empty body",
			path:       "/v1/chat/completions",
			body:       ``,
			wantType:   trace.ActionLLMChat,
			wantAction: "chat_completion",
			wantModel:  "",
		},
		{
			name:       "invalid JSON body",
			path:       "/v1/chat/completions",
			body:       `not json at all`,
			wantType:   trace.ActionLLMChat,
			wantAction: "chat_completion",
			wantModel:  "",
		},
		{
			name:       "completions (non-chat)",
			path:       "/v1/completions",
			body:       `{"model":"gpt-3.5-turbo-instruct","prompt":"Say hello"}`,
			wantType:   trace.ActionLLMChat,
			wantAction: "completion",
			wantModel:  "gpt-3.5-turbo-instruct",
		},
		{
			name:       "audio transcriptions",
			path:       "/v1/audio/transcriptions",
			body:       `{"model":"whisper-1"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "audio_transcription",
			wantModel:  "whisper-1",
		},
		{
			name:       "audio speech",
			path:       "/v1/audio/speech",
			body:       `{"model":"tts-1","input":"hello"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "audio_speech",
			wantModel:  "tts-1",
		},
		{
			name:       "moderations",
			path:       "/v1/moderations",
			body:       `{"model":"text-moderation-latest","input":"test"}`,
			wantType:   trace.ActionAPIRequest,
			wantAction: "moderation",
			wantModel:  "text-moderation-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			gotType, gotAction, gotModel := c.Classify(req, []byte(tt.body))

			if gotType != tt.wantType {
				t.Errorf("actionType = %q, want %q", gotType, tt.wantType)
			}
			if gotAction != tt.wantAction {
				t.Errorf("actionName = %q, want %q", gotAction, tt.wantAction)
			}
			if gotModel != tt.wantModel {
				t.Errorf("model = %q, want %q", gotModel, tt.wantModel)
			}
		})
	}
}

func TestExtractModel(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "valid model field",
			body: []byte(`{"model":"gpt-4","messages":[]}`),
			want: "gpt-4",
		},
		{
			name: "no model field",
			body: []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
			want: "",
		},
		{
			name: "empty body",
			body: []byte{},
			want: "",
		},
		{
			name: "nil body",
			body: nil,
			want: "",
		},
		{
			name: "invalid JSON",
			body: []byte(`{invalid`),
			want: "",
		},
		{
			name: "model is empty string",
			body: []byte(`{"model":""}`),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModel(tt.body)
			if got != tt.want {
				t.Errorf("extractModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassify_PathSpecificityOrdering(t *testing.T) {
	// /chat/completions should match before /completions
	c := NewClassifier(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, actionName, _ := c.Classify(req, []byte(`{"model":"gpt-4"}`))

	if actionName != "chat_completion" {
		t.Errorf("expected chat_completion for /v1/chat/completions, got %q", actionName)
	}

	// /completions alone should still match
	req2 := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	_, actionName2, _ := c.Classify(req2, []byte(`{"model":"gpt-3.5-turbo-instruct"}`))
	if actionName2 != "completion" {
		t.Errorf("expected completion for /v1/completions, got %q", actionName2)
	}
}
