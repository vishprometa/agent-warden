package cost

// ModelPricing holds per-token pricing for a model.
type ModelPricing struct {
	InputPerMToken  float64 // USD per million input tokens
	OutputPerMToken float64 // USD per million output tokens
}

// DefaultPricingTable returns current model pricing (Feb 2026).
// Updated via config or fetched from a pricing API.
var DefaultPricingTable = map[string]ModelPricing{
	// OpenAI
	"gpt-4o":            {InputPerMToken: 2.50, OutputPerMToken: 10.00},
	"gpt-4o-mini":       {InputPerMToken: 0.15, OutputPerMToken: 0.60},
	"gpt-4-turbo":       {InputPerMToken: 10.00, OutputPerMToken: 30.00},
	"gpt-4":             {InputPerMToken: 30.00, OutputPerMToken: 60.00},
	"gpt-3.5-turbo":     {InputPerMToken: 0.50, OutputPerMToken: 1.50},
	"o1":                {InputPerMToken: 15.00, OutputPerMToken: 60.00},
	"o1-mini":           {InputPerMToken: 3.00, OutputPerMToken: 12.00},
	"o3-mini":           {InputPerMToken: 1.10, OutputPerMToken: 4.40},

	// Anthropic
	"claude-opus-4-6":       {InputPerMToken: 15.00, OutputPerMToken: 75.00},
	"claude-sonnet-4-6":     {InputPerMToken: 3.00, OutputPerMToken: 15.00},
	"claude-haiku-4-5":      {InputPerMToken: 0.80, OutputPerMToken: 4.00},
	"claude-3-5-sonnet":     {InputPerMToken: 3.00, OutputPerMToken: 15.00},
	"claude-3-5-haiku":      {InputPerMToken: 0.80, OutputPerMToken: 4.00},
	"claude-3-opus":         {InputPerMToken: 15.00, OutputPerMToken: 75.00},

	// Google
	"gemini-2.0-flash":      {InputPerMToken: 0.10, OutputPerMToken: 0.40},
	"gemini-1.5-pro":        {InputPerMToken: 1.25, OutputPerMToken: 5.00},
	"gemini-1.5-flash":      {InputPerMToken: 0.075, OutputPerMToken: 0.30},

	// Meta (via providers)
	"llama-3.1-70b":         {InputPerMToken: 0.88, OutputPerMToken: 0.88},
	"llama-3.1-8b":          {InputPerMToken: 0.18, OutputPerMToken: 0.18},

	// Mistral
	"mistral-large":         {InputPerMToken: 2.00, OutputPerMToken: 6.00},
	"mistral-small":         {InputPerMToken: 0.20, OutputPerMToken: 0.60},

	// DeepSeek
	"deepseek-chat":         {InputPerMToken: 0.14, OutputPerMToken: 0.28},
	"deepseek-reasoner":     {InputPerMToken: 0.55, OutputPerMToken: 2.19},
}

// GetPricing returns pricing for a model, falling back to a default.
func GetPricing(model string) ModelPricing {
	if p, ok := DefaultPricingTable[model]; ok {
		return p
	}
	// Fallback: assume moderate pricing
	return ModelPricing{InputPerMToken: 1.00, OutputPerMToken: 3.00}
}

// CalculateCost computes the USD cost for a request.
func CalculateCost(model string, inputTokens, outputTokens int) float64 {
	pricing := GetPricing(model)
	inputCost := float64(inputTokens) / 1_000_000.0 * pricing.InputPerMToken
	outputCost := float64(outputTokens) / 1_000_000.0 * pricing.OutputPerMToken
	return inputCost + outputCost
}
