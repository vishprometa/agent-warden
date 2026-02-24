package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ComputeHash computes the SHA-256 hash for a trace entry, chaining to the previous hash.
func ComputeHash(t *Trace) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		t.ID,
		t.SessionID,
		string(t.ActionType),
		string(t.RequestBody),
		string(t.ResponseBody),
		t.PrevHash,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ComputeSessionSeed computes the initial prev_hash for the first trace in a session.
func ComputeSessionSeed(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
}

// VerifyChain walks a list of traces and checks hash integrity.
// Returns (valid, brokenAtIndex). If valid is true, all hashes check out.
func VerifyChain(traces []*Trace) (bool, int) {
	for i, t := range traces {
		expected := ComputeHash(t)
		if t.Hash != expected {
			return false, i
		}
		// Check chain linkage
		if i > 0 && t.PrevHash != traces[i-1].Hash {
			return false, i
		}
	}
	return true, -1
}
