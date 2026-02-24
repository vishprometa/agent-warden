package proxy

import (
	"log/slog"
	"strings"

	"github.com/agentwarden/agentwarden/internal/config"
)

// Router resolves which upstream provider URL to forward a request to
// based on the model string or an explicit provider name.
type Router struct {
	cfg    *config.UpstreamConfig
	logger *slog.Logger
}

// NewRouter creates a new upstream router with the given upstream configuration.
func NewRouter(cfg *config.UpstreamConfig, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		cfg:    cfg,
		logger: logger.With("component", "proxy.Router"),
	}
}

// modelPrefixMapping maps model name prefixes to provider keys.
// Evaluated in order; first match wins.
var modelPrefixMapping = []struct {
	prefix   string
	provider string
}{
	{prefix: "gpt-", provider: "openai"},
	{prefix: "o1-", provider: "openai"},
	{prefix: "o3-", provider: "openai"},
	{prefix: "o4-", provider: "openai"},
	{prefix: "chatgpt-", provider: "openai"},
	{prefix: "claude-", provider: "anthropic"},
	{prefix: "gemini-", provider: "gemini"},
	{prefix: "gemma-", provider: "gemini"},
	{prefix: "text-embedding-", provider: "openai"},
	{prefix: "text-moderation-", provider: "openai"},
	{prefix: "dall-e-", provider: "openai"},
	{prefix: "whisper-", provider: "openai"},
	{prefix: "tts-", provider: "openai"},
}

// ResolveUpstream determines the upstream base URL for a request based on the
// model name. It checks model prefix patterns first, then falls back to the
// configured default upstream.
//
// The model parameter can be:
//   - A model name like "gpt-4o", "claude-3-opus", "gemini-1.5-pro"
//   - An explicit provider key like "openai", "anthropic"
//   - Empty, in which case the default upstream is used
func (r *Router) ResolveUpstream(model string) string {
	if model == "" {
		r.logger.Debug("no model specified, using default upstream", "upstream", r.cfg.Default)
		return r.cfg.Default
	}

	// Check if the model string is itself a provider key.
	if url, ok := r.cfg.Providers[model]; ok {
		r.logger.Debug("model is a provider key", "provider", model, "upstream", url)
		return url
	}

	// Match model prefixes to determine the provider.
	modelLower := strings.ToLower(model)
	for _, mapping := range modelPrefixMapping {
		if strings.HasPrefix(modelLower, mapping.prefix) {
			if url, ok := r.cfg.Providers[mapping.provider]; ok {
				r.logger.Debug("resolved upstream from model prefix",
					"model", model,
					"provider", mapping.provider,
					"upstream", url,
				)
				return url
			}
		}
	}

	// Check if any provider name appears as a substring in the model
	// (handles cases like "openai/gpt-4" or "anthropic/claude-3").
	for provider, url := range r.cfg.Providers {
		if strings.Contains(modelLower, provider) {
			r.logger.Debug("resolved upstream from model substring",
				"model", model,
				"provider", provider,
				"upstream", url,
			)
			return url
		}
	}

	r.logger.Debug("no provider match for model, using default upstream",
		"model", model,
		"upstream", r.cfg.Default,
	)
	return r.cfg.Default
}

// ProviderForModel returns the provider key (e.g., "openai", "anthropic")
// for a given model name. Returns "unknown" if no match is found.
func (r *Router) ProviderForModel(model string) string {
	if model == "" {
		return "unknown"
	}

	if _, ok := r.cfg.Providers[model]; ok {
		return model
	}

	modelLower := strings.ToLower(model)
	for _, mapping := range modelPrefixMapping {
		if strings.HasPrefix(modelLower, mapping.prefix) {
			return mapping.provider
		}
	}

	for provider := range r.cfg.Providers {
		if strings.Contains(modelLower, provider) {
			return provider
		}
	}

	return "unknown"
}
