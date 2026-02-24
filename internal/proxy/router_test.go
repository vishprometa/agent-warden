package proxy

import (
	"testing"

	"github.com/agentwarden/agentwarden/internal/config"
)

func newTestRouter() *Router {
	cfg := &config.UpstreamConfig{
		Default: "https://api.openai.com/v1",
		Providers: map[string]string{
			"openai":    "https://api.openai.com/v1",
			"anthropic": "https://api.anthropic.com/v1",
			"gemini":    "https://generativelanguage.googleapis.com/v1beta",
		},
	}
	return NewRouter(cfg, nil)
}

func TestResolveUpstream_ModelPrefixes(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name     string
		model    string
		wantURL  string
	}{
		{
			name:    "gpt-4 maps to openai",
			model:   "gpt-4",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "gpt-4o maps to openai",
			model:   "gpt-4o",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "gpt-3.5-turbo maps to openai",
			model:   "gpt-3.5-turbo",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "o1-mini maps to openai",
			model:   "o1-mini",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "o3-mini maps to openai",
			model:   "o3-mini",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "o4-mini maps to openai",
			model:   "o4-mini",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "chatgpt-4o maps to openai",
			model:   "chatgpt-4o-latest",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "claude-3-opus maps to anthropic",
			model:   "claude-3-opus",
			wantURL: "https://api.anthropic.com/v1",
		},
		{
			name:    "claude-sonnet-4-6 maps to anthropic",
			model:   "claude-sonnet-4-6",
			wantURL: "https://api.anthropic.com/v1",
		},
		{
			name:    "gemini-1.5-pro maps to gemini",
			model:   "gemini-1.5-pro",
			wantURL: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name:    "gemma-7b maps to gemini",
			model:   "gemma-7b",
			wantURL: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name:    "text-embedding-ada-002 maps to openai",
			model:   "text-embedding-ada-002",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "dall-e-3 maps to openai",
			model:   "dall-e-3",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "whisper-1 maps to openai",
			model:   "whisper-1",
			wantURL: "https://api.openai.com/v1",
		},
		{
			name:    "tts-1 maps to openai",
			model:   "tts-1",
			wantURL: "https://api.openai.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ResolveUpstream(tt.model)
			if got != tt.wantURL {
				t.Errorf("ResolveUpstream(%q) = %q, want %q", tt.model, got, tt.wantURL)
			}
		})
	}
}

func TestResolveUpstream_EmptyModel(t *testing.T) {
	r := newTestRouter()
	got := r.ResolveUpstream("")
	want := "https://api.openai.com/v1"
	if got != want {
		t.Errorf("ResolveUpstream(\"\") = %q, want %q", got, want)
	}
}

func TestResolveUpstream_ProviderKeyDirect(t *testing.T) {
	r := newTestRouter()

	// When the model string is itself a provider key, use it directly
	got := r.ResolveUpstream("anthropic")
	want := "https://api.anthropic.com/v1"
	if got != want {
		t.Errorf("ResolveUpstream(\"anthropic\") = %q, want %q", got, want)
	}
}

func TestResolveUpstream_SubstringMatch(t *testing.T) {
	r := newTestRouter()

	// "openai/gpt-4" should match "openai" as a substring
	got := r.ResolveUpstream("openai/gpt-4")
	want := "https://api.openai.com/v1"
	if got != want {
		t.Errorf("ResolveUpstream(\"openai/gpt-4\") = %q, want %q", got, want)
	}
}

func TestResolveUpstream_UnknownModel(t *testing.T) {
	r := newTestRouter()

	// Unknown model should fall back to default
	got := r.ResolveUpstream("totally-unknown-model-xyz")
	want := "https://api.openai.com/v1"
	if got != want {
		t.Errorf("ResolveUpstream(\"totally-unknown-model-xyz\") = %q, want %q", got, want)
	}
}

func TestResolveUpstream_CaseInsensitive(t *testing.T) {
	r := newTestRouter()

	// Model matching should be case-insensitive
	got := r.ResolveUpstream("GPT-4o")
	want := "https://api.openai.com/v1"
	if got != want {
		t.Errorf("ResolveUpstream(\"GPT-4o\") = %q, want %q", got, want)
	}
}

func TestResolveUpstream_CustomProviderConfig(t *testing.T) {
	cfg := &config.UpstreamConfig{
		Default: "https://my-proxy.example.com/v1",
		Providers: map[string]string{
			"openai":    "https://my-openai-proxy.example.com/v1",
			"anthropic": "https://my-anthropic-proxy.example.com/v1",
			"custom":    "https://custom-llm.example.com/api",
		},
	}
	r := NewRouter(cfg, nil)

	got := r.ResolveUpstream("gpt-4")
	want := "https://my-openai-proxy.example.com/v1"
	if got != want {
		t.Errorf("custom config: ResolveUpstream(\"gpt-4\") = %q, want %q", got, want)
	}

	// Direct provider key
	got = r.ResolveUpstream("custom")
	want = "https://custom-llm.example.com/api"
	if got != want {
		t.Errorf("custom config: ResolveUpstream(\"custom\") = %q, want %q", got, want)
	}

	// Unknown falls to custom default
	got = r.ResolveUpstream("some-unknown-model")
	want = "https://my-proxy.example.com/v1"
	if got != want {
		t.Errorf("custom config: ResolveUpstream(unknown) = %q, want %q", got, want)
	}
}

func TestProviderForModel(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name     string
		model    string
		wantProv string
	}{
		{"gpt-4 is openai", "gpt-4", "openai"},
		{"claude-3-opus is anthropic", "claude-3-opus", "anthropic"},
		{"gemini-1.5-pro is gemini", "gemini-1.5-pro", "gemini"},
		{"empty model is unknown", "", "unknown"},
		{"unknown model is unknown", "totally-random-xyz", "unknown"},
		{"provider key is itself", "openai", "openai"},
		{"o1-mini is openai", "o1-mini", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ProviderForModel(tt.model)
			if got != tt.wantProv {
				t.Errorf("ProviderForModel(%q) = %q, want %q", tt.model, got, tt.wantProv)
			}
		})
	}
}
