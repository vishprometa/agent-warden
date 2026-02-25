package cost

import (
	"math"
	"testing"
)

func TestGetPricing_KnownModels(t *testing.T) {
	tests := []struct {
		model      string
		wantInput  float64
		wantOutput float64
	}{
		{"gpt-4", 30.00, 60.00},
		{"gpt-4o", 2.50, 10.00},
		{"gpt-4o-mini", 0.15, 0.60},
		{"gpt-3.5-turbo", 0.50, 1.50},
		{"gpt-4-turbo", 10.00, 30.00},
		{"o1", 15.00, 60.00},
		{"o1-mini", 3.00, 12.00},
		{"o3-mini", 1.10, 4.40},
		{"claude-opus-4-6", 15.00, 75.00},
		{"claude-sonnet-4-6", 3.00, 15.00},
		{"claude-haiku-4-5", 0.80, 4.00},
		{"claude-3-5-sonnet", 3.00, 15.00},
		{"claude-3-opus", 15.00, 75.00},
		{"gemini-2.0-flash", 0.10, 0.40},
		{"gemini-1.5-pro", 1.25, 5.00},
		{"gemini-1.5-flash", 0.075, 0.30},
		{"llama-3.1-70b", 0.88, 0.88},
		{"mistral-large", 2.00, 6.00},
		{"deepseek-chat", 0.14, 0.28},
		{"deepseek-reasoner", 0.55, 2.19},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			p := GetPricing(tt.model)
			if p.InputPerMToken != tt.wantInput {
				t.Errorf("InputPerMToken = %f, want %f", p.InputPerMToken, tt.wantInput)
			}
			if p.OutputPerMToken != tt.wantOutput {
				t.Errorf("OutputPerMToken = %f, want %f", p.OutputPerMToken, tt.wantOutput)
			}
		})
	}
}

func TestGetPricing_UnknownModel(t *testing.T) {
	p := GetPricing("totally-unknown-model-xyz")
	if p.InputPerMToken != 1.00 {
		t.Errorf("fallback InputPerMToken = %f, want 1.00", p.InputPerMToken)
	}
	if p.OutputPerMToken != 3.00 {
		t.Errorf("fallback OutputPerMToken = %f, want 3.00", p.OutputPerMToken)
	}
}

func TestCalculateCost_GPT4(t *testing.T) {
	// gpt-4: $30/M input, $60/M output
	// 1000 input tokens = 1000/1M * 30 = 0.03
	// 500 output tokens = 500/1M * 60 = 0.03
	// Total = 0.06
	cost := CalculateCost("gpt-4", 1000, 500)
	expected := 0.06

	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("CalculateCost(\"gpt-4\", 1000, 500) = %f, want %f", cost, expected)
	}
}

func TestCalculateCost_GPT35Turbo(t *testing.T) {
	// gpt-3.5-turbo: $0.50/M input, $1.50/M output
	// 10000 input = 10000/1M * 0.50 = 0.005
	// 5000 output = 5000/1M * 1.50 = 0.0075
	// Total = 0.0125
	cost := CalculateCost("gpt-3.5-turbo", 10000, 5000)
	expected := 0.0125

	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("CalculateCost(\"gpt-3.5-turbo\", 10000, 5000) = %f, want %f", cost, expected)
	}
}

func TestCalculateCost_Claude3Opus(t *testing.T) {
	// claude-3-opus: $15/M input, $75/M output
	// 1_000_000 input = 15.00
	// 1_000_000 output = 75.00
	// Total = 90.00
	cost := CalculateCost("claude-3-opus", 1_000_000, 1_000_000)
	expected := 90.00

	if math.Abs(cost-expected) > 1e-6 {
		t.Errorf("CalculateCost(\"claude-3-opus\", 1M, 1M) = %f, want %f", cost, expected)
	}
}

func TestCalculateCost_ZeroTokens(t *testing.T) {
	cost := CalculateCost("gpt-4", 0, 0)
	if cost != 0 {
		t.Errorf("CalculateCost with 0 tokens = %f, want 0", cost)
	}
}

func TestCalculateCost_UnknownModelFallback(t *testing.T) {
	// Fallback: $1/M input, $3/M output
	// 1000 input = 1000/1M * 1 = 0.001
	// 1000 output = 1000/1M * 3 = 0.003
	// Total = 0.004
	cost := CalculateCost("unknown-model", 1000, 1000)
	expected := 0.004

	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("CalculateCost(\"unknown-model\", 1000, 1000) = %f, want %f", cost, expected)
	}
}
