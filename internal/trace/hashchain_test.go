package trace

import (
	"encoding/json"
	"testing"
)

func TestComputeHash_Deterministic(t *testing.T) {
	tr := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{"choices":[{"message":{"content":"hello"}}]}`),
		PrevHash:     "0000000000000000000000000000000000000000000000000000000000000000",
	}

	hash1 := ComputeHash(tr)
	hash2 := ComputeHash(tr)

	if hash1 != hash2 {
		t.Errorf("ComputeHash is not deterministic: %q != %q", hash1, hash2)
	}

	// Hash should be a 64-char hex string (SHA-256)
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestComputeHash_DifferentInputs(t *testing.T) {
	tr1 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{"choices":[]}`),
		PrevHash:     "abc",
	}
	tr2 := &Trace{
		ID:           "trace-002",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{"choices":[]}`),
		PrevHash:     "abc",
	}

	hash1 := ComputeHash(tr1)
	hash2 := ComputeHash(tr2)

	if hash1 == hash2 {
		t.Error("different trace IDs should produce different hashes")
	}
}

func TestComputeHash_PrevHashAffectsOutput(t *testing.T) {
	tr1 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     "aaaa",
	}
	tr2 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     "bbbb",
	}

	hash1 := ComputeHash(tr1)
	hash2 := ComputeHash(tr2)

	if hash1 == hash2 {
		t.Error("different PrevHash should produce different hashes")
	}
}

func TestComputeSessionSeed(t *testing.T) {
	seed1 := ComputeSessionSeed("session-abc")
	seed2 := ComputeSessionSeed("session-abc")

	if seed1 != seed2 {
		t.Errorf("ComputeSessionSeed is not deterministic: %q != %q", seed1, seed2)
	}

	if len(seed1) != 64 {
		t.Errorf("seed length = %d, want 64", len(seed1))
	}

	// Different sessions produce different seeds
	seed3 := ComputeSessionSeed("session-xyz")
	if seed1 == seed3 {
		t.Error("different session IDs should produce different seeds")
	}
}

func TestVerifyChain_ValidChain(t *testing.T) {
	sessionSeed := ComputeSessionSeed("sess-001")

	trace1 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{"content":"hello"}`),
		PrevHash:     sessionSeed,
	}
	trace1.Hash = ComputeHash(trace1)

	trace2 := &Trace{
		ID:           "trace-002",
		SessionID:    "sess-001",
		ActionType:   ActionLLMEmbed,
		RequestBody:  json.RawMessage(`{"model":"text-embedding-ada-002"}`),
		ResponseBody: json.RawMessage(`{"data":[]}`),
		PrevHash:     trace1.Hash,
	}
	trace2.Hash = ComputeHash(trace2)

	trace3 := &Trace{
		ID:           "trace-003",
		SessionID:    "sess-001",
		ActionType:   ActionToolCall,
		RequestBody:  json.RawMessage(`{"tool":"search"}`),
		ResponseBody: json.RawMessage(`{"result":"found"}`),
		PrevHash:     trace2.Hash,
	}
	trace3.Hash = ComputeHash(trace3)

	valid, brokenAt := VerifyChain([]*Trace{trace1, trace2, trace3})
	if !valid {
		t.Errorf("VerifyChain returned invalid at index %d, expected valid", brokenAt)
	}
	if brokenAt != -1 {
		t.Errorf("brokenAt = %d, want -1 (valid chain)", brokenAt)
	}
}

func TestVerifyChain_TamperedHash(t *testing.T) {
	sessionSeed := ComputeSessionSeed("sess-001")

	trace1 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{"model":"gpt-4"}`),
		ResponseBody: json.RawMessage(`{"content":"hello"}`),
		PrevHash:     sessionSeed,
	}
	trace1.Hash = ComputeHash(trace1)

	trace2 := &Trace{
		ID:           "trace-002",
		SessionID:    "sess-001",
		ActionType:   ActionLLMEmbed,
		RequestBody:  json.RawMessage(`{}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     trace1.Hash,
	}
	trace2.Hash = "tampered_hash_value_that_is_clearly_wrong_and_invalid"

	valid, brokenAt := VerifyChain([]*Trace{trace1, trace2})
	if valid {
		t.Error("VerifyChain should detect tampered hash")
	}
	if brokenAt != 1 {
		t.Errorf("brokenAt = %d, want 1", brokenAt)
	}
}

func TestVerifyChain_BrokenLinkage(t *testing.T) {
	sessionSeed := ComputeSessionSeed("sess-001")

	trace1 := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     sessionSeed,
	}
	trace1.Hash = ComputeHash(trace1)

	// trace2 has wrong PrevHash (not linking to trace1.Hash)
	trace2 := &Trace{
		ID:           "trace-002",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     "wrong_prev_hash",
	}
	trace2.Hash = ComputeHash(trace2)

	valid, brokenAt := VerifyChain([]*Trace{trace1, trace2})
	if valid {
		t.Error("VerifyChain should detect broken chain linkage")
	}
	if brokenAt != 1 {
		t.Errorf("brokenAt = %d, want 1", brokenAt)
	}
}

func TestVerifyChain_EmptyChain(t *testing.T) {
	valid, brokenAt := VerifyChain([]*Trace{})
	if !valid {
		t.Error("empty chain should be valid")
	}
	if brokenAt != -1 {
		t.Errorf("brokenAt = %d, want -1", brokenAt)
	}
}

func TestVerifyChain_SingleTrace(t *testing.T) {
	tr := &Trace{
		ID:           "trace-001",
		SessionID:    "sess-001",
		ActionType:   ActionLLMChat,
		RequestBody:  json.RawMessage(`{}`),
		ResponseBody: json.RawMessage(`{}`),
		PrevHash:     ComputeSessionSeed("sess-001"),
	}
	tr.Hash = ComputeHash(tr)

	valid, brokenAt := VerifyChain([]*Trace{tr})
	if !valid {
		t.Errorf("single valid trace should pass, broken at %d", brokenAt)
	}
}
