package detection

import (
	"fmt"
	"testing"

	"github.com/agentwarden/agentwarden/internal/config"
)

func TestSpiralDetector_IdenticalOutputs(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	// Send 3 identical outputs (window=3)
	for i := 0; i < 3; i++ {
		event := ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Content:   "The answer to your question is that we need to analyze the data carefully before proceeding.",
		}
		result := d.Check(event)
		if i < 2 {
			// First 2 checks: not enough history yet (need window=3 outputs)
			if result != nil {
				t.Errorf("Check #%d: expected nil, got detection", i+1)
			}
		} else {
			// 3rd check: window full, all identical -> similarity=1.0 > 0.9
			if result == nil {
				t.Fatal("Check #3: expected spiral detection, got nil")
			}
			if result.Type != "spiral" {
				t.Errorf("event type = %q, want \"spiral\"", result.Type)
			}
			if result.Action != "alert" {
				t.Errorf("action = %q, want \"alert\"", result.Action)
			}
		}
	}
}

func TestSpiralDetector_DiverseOutputs(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	outputs := []string{
		"The weather today is sunny with clear skies across the region.",
		"Python is a popular programming language for machine learning tasks.",
		"The stock market experienced significant volatility during trading hours.",
	}

	for i, content := range outputs {
		event := ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Content:   content,
		}
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check #%d: expected nil for diverse outputs, got detection", i+1)
		}
	}
}

func TestSpiralDetector_EmptyContent(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Content:   "",
	}

	result := d.Check(event)
	if result != nil {
		t.Error("expected nil for empty content, got detection")
	}
}

func TestSpiralDetector_DifferentSessions(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	// Each session only gets 1 output, so no window is full
	for i := 0; i < 5; i++ {
		event := ActionEvent{
			SessionID: fmt.Sprintf("sess-%d", i),
			AgentID:   "agent-1",
			Content:   "The answer to your question is that we need to analyze the data carefully.",
		}
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check with session-%d: expected nil, got detection", i)
		}
	}
}

func TestSpiralDetector_HighlySimilarButNotIdentical(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.8,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	// These outputs are very similar (same words, slightly reordered/varied)
	outputs := []string{
		"I apologize but I cannot help with that request due to safety policy restrictions.",
		"I apologize but I cannot help with that request due to safety policy restrictions.",
		"I apologize but I cannot assist with that request due to safety policy restrictions.",
	}

	var lastResult *Event
	for i, content := range outputs {
		event := ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Content:   content,
		}
		lastResult = d.Check(event)
		if i < 2 && lastResult != nil {
			t.Errorf("Check #%d: not enough window yet, expected nil", i+1)
		}
	}

	if lastResult == nil {
		t.Fatal("expected spiral detection for highly similar outputs, got nil")
	}
}

func TestSpiralDetector_BelowSimilarityThreshold(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.99, // very high threshold
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	// Similar but not 99% similar
	outputs := []string{
		"The quick brown fox jumps over the lazy dog in the morning sunshine.",
		"The quick brown fox leaps over the sleepy dog in the afternoon rain.",
		"The swift brown fox hops over the drowsy dog in the evening mist.",
	}

	var lastResult *Event
	for _, content := range outputs {
		event := ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Content:   content,
		}
		lastResult = d.Check(event)
	}

	if lastResult != nil {
		t.Error("expected nil with high threshold and varied outputs, got detection")
	}
}

func TestSpiralDetector_ResetSession(t *testing.T) {
	cfg := config.SpiralDetectionConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		Window:              3,
		Action:              "alert",
	}
	d := NewSpiralDetector(cfg)

	// Build up 2 identical outputs
	for i := 0; i < 2; i++ {
		d.Check(ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Content:   "Repeated output text that is identical each time.",
		})
	}

	// Reset session
	d.ResetSession("sess-1")

	// After reset, history is empty; one more event shouldn't trigger
	result := d.Check(ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Content:   "Repeated output text that is identical each time.",
	})
	if result != nil {
		t.Error("expected nil after ResetSession, got detection")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{
			name:    "identical strings",
			a:       "hello world how are you",
			b:       "hello world how are you",
			wantMin: 0.99,
			wantMax: 1.01,
		},
		{
			name:    "completely different",
			a:       "apple banana cherry",
			b:       "elephant giraffe hippopotamus",
			wantMin: -0.01,
			wantMax: 0.01,
		},
		{
			name:    "partially similar",
			a:       "the cat sat on the mat in the house",
			b:       "the dog sat on the rug in the garden",
			wantMin: 0.3,
			wantMax: 0.8,
		},
		{
			name:    "empty first string",
			a:       "",
			b:       "hello world",
			wantMin: -0.01,
			wantMax: 0.01,
		},
		{
			name:    "both empty",
			a:       "",
			b:       "",
			wantMin: -0.01,
			wantMax: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("cosineSimilarity(%q, %q) = %f, want in [%f, %f]",
					tt.a, tt.b, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected number of tokens
	}{
		{"normal text", "Hello World, how are you?", 5}, // "hello", "world", "how", "are", "you" (punctuation stripped, all > 1 char)
		{"empty", "", 0},
		{"single char words", "a b c d", 0}, // all single-char, filtered out
		{"punctuation stripped", "hello! world? foo.", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != tt.want {
				t.Errorf("tokenize(%q) returned %d tokens %v, want %d", tt.input, len(got), got, tt.want)
			}
		})
	}
}
