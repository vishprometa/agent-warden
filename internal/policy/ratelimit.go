package policy

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// bucketGranularity is the time resolution for counter buckets.
	// Finer granularity gives more accurate sliding windows at the cost of
	// slightly more memory. One second is a good default for rate limits
	// expressed in per-minute or per-second terms.
	bucketGranularity = time.Second

	// gcInterval controls how often expired buckets are pruned. This is
	// checked lazily on each RecordAction call rather than via a background
	// goroutine to keep the type self-contained and easy to test.
	gcInterval = 30 * time.Second

	// maxWindowDuration caps the lookback that GetCount will accept to
	// prevent unbounded memory growth from callers requesting huge windows.
	maxWindowDuration = 24 * time.Hour
)

// bucket holds the count for a single time slice.
type bucket struct {
	key   int64 // unix-second timestamp of the bucket start
	count int
}

// sessionCounters holds per-action-type time-bucketed counters for one session.
type sessionCounters struct {
	// actions maps actionType -> ordered slice of buckets.
	actions map[string][]bucket
}

// RateLimiter provides thread-safe sliding-window rate limiting using
// time-bucketed counters. Each (session, actionType) pair maintains an
// independent set of counters. Expired buckets are lazily garbage-collected.
type RateLimiter struct {
	mu       sync.Mutex
	sessions map[string]*sessionCounters
	lastGC   time.Time
	logger   *slog.Logger
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(logger *slog.Logger) *RateLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	return &RateLimiter{
		sessions: make(map[string]*sessionCounters),
		lastGC:   time.Now(),
		logger:   logger.With("component", "policy.RateLimiter"),
	}
}

// RecordAction increments the counter for the given session and action type
// at the current time bucket.
func (r *RateLimiter) RecordAction(sessionID, actionType string) {
	now := time.Now()
	key := now.Truncate(bucketGranularity).Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	sc, ok := r.sessions[sessionID]
	if !ok {
		sc = &sessionCounters{actions: make(map[string][]bucket)}
		r.sessions[sessionID] = sc
	}

	buckets := sc.actions[actionType]

	// Fast path: last bucket matches current time key.
	if len(buckets) > 0 && buckets[len(buckets)-1].key == key {
		buckets[len(buckets)-1].count++
	} else {
		buckets = append(buckets, bucket{key: key, count: 1})
	}
	sc.actions[actionType] = buckets

	// Lazy GC check.
	if now.Sub(r.lastGC) > gcInterval {
		r.gcLocked(now)
		r.lastGC = now
	}
}

// GetCount returns the total number of actions of the given type recorded
// for the session within the specified sliding window. The window string
// is parsed as a Go duration (e.g. "60s", "5m", "1h").
func (r *RateLimiter) GetCount(sessionID, actionType, window string) int {
	dur, err := time.ParseDuration(window)
	if err != nil {
		r.logger.Warn("invalid window duration, returning 0",
			"window", window,
			"error", err,
		)
		return 0
	}
	if dur <= 0 {
		return 0
	}
	if dur > maxWindowDuration {
		dur = maxWindowDuration
	}

	cutoff := time.Now().Add(-dur).Truncate(bucketGranularity).Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	sc, ok := r.sessions[sessionID]
	if !ok {
		return 0
	}

	buckets := sc.actions[actionType]
	total := 0
	for _, b := range buckets {
		if b.key >= cutoff {
			total += b.count
		}
	}
	return total
}

// Reset removes all tracked counters for a session. Call this when a session
// ends to free memory.
func (r *RateLimiter) Reset(sessionID string) {
	r.mu.Lock()
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	r.logger.Debug("reset rate limit counters", "session_id", sessionID)
}

// gcLocked prunes buckets older than maxWindowDuration. Must be called
// while r.mu is held.
func (r *RateLimiter) gcLocked(now time.Time) {
	cutoff := now.Add(-maxWindowDuration).Truncate(bucketGranularity).Unix()
	pruned := 0

	for sid, sc := range r.sessions {
		empty := true
		for at, buckets := range sc.actions {
			// Find the first bucket that is within the retention window.
			firstValid := len(buckets)
			for i, b := range buckets {
				if b.key >= cutoff {
					firstValid = i
					break
				}
			}

			if firstValid > 0 {
				pruned += firstValid
				sc.actions[at] = buckets[firstValid:]
			}

			if len(sc.actions[at]) > 0 {
				empty = false
			} else {
				delete(sc.actions, at)
			}
		}
		if empty {
			delete(r.sessions, sid)
		}
	}

	if pruned > 0 {
		r.logger.Debug("rate limiter GC complete",
			"pruned_buckets", pruned,
			"active_sessions", len(r.sessions),
		)
	}
}
