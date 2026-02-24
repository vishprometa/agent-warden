package detection

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/agentwarden/agentwarden/internal/config"
)

// SpiralDetector detects when an LLM keeps generating similar non-converging outputs.
type SpiralDetector struct {
	mu     sync.Mutex
	config config.SpiralDetectionConfig
	// sessionID → recent output texts
	history map[string][]string
}

// NewSpiralDetector creates a new spiral detector.
func NewSpiralDetector(cfg config.SpiralDetectionConfig) *SpiralDetector {
	return &SpiralDetector{
		config:  cfg,
		history: make(map[string][]string),
	}
}

// Check records an LLM output and detects if outputs are spiraling.
func (d *SpiralDetector) Check(event ActionEvent) *Event {
	if event.Content == "" {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.history[event.SessionID] = append(d.history[event.SessionID], event.Content)

	outputs := d.history[event.SessionID]
	window := d.config.Window
	if len(outputs) < window {
		return nil
	}

	// Check last `window` outputs for high similarity
	recent := outputs[len(outputs)-window:]
	allSimilar := true
	avgSimilarity := 0.0
	comparisons := 0

	for i := 0; i < len(recent)-1; i++ {
		sim := cosineSimilarity(recent[i], recent[i+1])
		avgSimilarity += sim
		comparisons++
		if sim < d.config.SimilarityThreshold {
			allSimilar = false
		}
	}

	if comparisons > 0 {
		avgSimilarity /= float64(comparisons)
	}

	if allSimilar {
		return &Event{
			Type:      "spiral",
			SessionID: event.SessionID,
			AgentID:   event.AgentID,
			Action:    d.config.Action,
			Message: fmt.Sprintf("Conversation spiral: %d consecutive outputs with %.0f%% average similarity (threshold: %.0f%%)",
				window, avgSimilarity*100, d.config.SimilarityThreshold*100),
			Details: map[string]interface{}{
				"window":          window,
				"avg_similarity":  avgSimilarity,
				"threshold":       d.config.SimilarityThreshold,
				"consecutive":     window,
			},
		}
	}

	// Trim history to prevent unbounded growth
	if len(outputs) > window*3 {
		d.history[event.SessionID] = outputs[len(outputs)-window*2:]
	}

	return nil
}

// ResetSession clears state for a session.
func (d *SpiralDetector) ResetSession(sessionID string) {
	d.mu.Lock()
	delete(d.history, sessionID)
	d.mu.Unlock()
}

// cosineSimilarity computes a simple word-frequency-based cosine similarity.
// This is a lightweight approximation; production use should use embeddings.
func cosineSimilarity(a, b string) float64 {
	wordsA := tokenize(a)
	wordsB := tokenize(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	// Build frequency vectors
	vocab := make(map[string]int)
	freqA := make(map[string]float64)
	freqB := make(map[string]float64)

	for _, w := range wordsA {
		vocab[w] = 1
		freqA[w]++
	}
	for _, w := range wordsB {
		vocab[w] = 1
		freqB[w]++
	}

	// Compute dot product and magnitudes
	var dot, magA, magB float64
	for word := range vocab {
		a := freqA[word]
		b := freqB[word]
		dot += a * b
		magA += a * a
		magB += b * b
	}

	if magA == 0 || magB == 0 {
		return 0
	}

	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

// tokenize splits text into lowercase word tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	result := make([]string, 0, len(words))
	for _, w := range words {
		// Strip basic punctuation
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) > 1 {
			result = append(result, w)
		}
	}
	return result
}
