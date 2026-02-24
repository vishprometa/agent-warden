package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// Request represents a pending approval request.
type Request struct {
	ID            string
	SessionID     string
	TraceID       string
	PolicyName    string
	ActionSummary map[string]interface{}
	Timeout       time.Duration
	TimeoutEffect string // "deny" or "allow"
	CreatedAt     time.Time
	result        chan Result
}

// Result is the outcome of an approval request.
type Result struct {
	Approved   bool
	ResolvedBy string
}

// Queue manages pending approval requests.
type Queue struct {
	mu       sync.RWMutex
	pending  map[string]*Request // approvalID → Request
	store    trace.Store
	alertMgr *alert.Manager
	logger   *slog.Logger
}

// NewQueue creates a new approval queue.
func NewQueue(store trace.Store, alertMgr *alert.Manager, logger *slog.Logger) *Queue {
	q := &Queue{
		pending:  make(map[string]*Request),
		store:    store,
		alertMgr: alertMgr,
		logger:   logger,
	}

	// Start timeout checker
	go q.checkTimeouts()

	return q
}

// Submit queues an action for approval and blocks until resolved or timed out.
func (q *Queue) Submit(ctx context.Context, req *Request) (bool, error) {
	req.CreatedAt = time.Now()
	req.result = make(chan Result, 1)

	// Persist to store
	approval := &trace.Approval{
		ID:            req.ID,
		SessionID:     req.SessionID,
		TraceID:       req.TraceID,
		PolicyName:    req.PolicyName,
		Status:        "pending",
		CreatedAt:     req.CreatedAt,
		TimeoutAt:     req.CreatedAt.Add(req.Timeout),
	}

	if summary, err := json.Marshal(req.ActionSummary); err == nil {
		approval.ActionSummary = summary
	}

	if err := q.store.InsertApproval(approval); err != nil {
		return false, fmt.Errorf("failed to persist approval: %w", err)
	}

	// Add to in-memory queue
	q.mu.Lock()
	q.pending[req.ID] = req
	q.mu.Unlock()

	// Send alert for approval
	if q.alertMgr != nil {
		q.alertMgr.Send(alert.Alert{
			Type:      "approval_required",
			Severity:  "warning",
			Title:     fmt.Sprintf("Approval needed: %s", req.PolicyName),
			Message:   fmt.Sprintf("Action requires approval per policy %q. Session: %s", req.PolicyName, req.SessionID),
			SessionID: req.SessionID,
			Details:   req.ActionSummary,
		})
	}

	q.logger.Info("approval request submitted",
		"approval_id", req.ID,
		"policy", req.PolicyName,
		"session_id", req.SessionID,
		"timeout", req.Timeout,
	)

	// Wait for resolution
	select {
	case result := <-req.result:
		return result.Approved, nil
	case <-ctx.Done():
		q.cleanup(req.ID)
		return false, ctx.Err()
	}
}

// Resolve approves or denies a pending request.
func (q *Queue) Resolve(approvalID string, approved bool, resolvedBy string) error {
	q.mu.Lock()
	req, ok := q.pending[approvalID]
	if ok {
		delete(q.pending, approvalID)
	}
	q.mu.Unlock()

	if !ok {
		return fmt.Errorf("approval %s not found or already resolved", approvalID)
	}

	// Update store
	status := "denied"
	if approved {
		status = "approved"
	}
	if err := q.store.ResolveApproval(approvalID, status, resolvedBy); err != nil {
		q.logger.Error("failed to update approval in store", "error", err)
	}

	// Notify waiting goroutine
	req.result <- Result{Approved: approved, ResolvedBy: resolvedBy}

	q.logger.Info("approval resolved",
		"approval_id", approvalID,
		"approved", approved,
		"resolved_by", resolvedBy,
	)

	return nil
}

// ListPending returns all pending approval requests.
func (q *Queue) ListPending() []*Request {
	q.mu.RLock()
	defer q.mu.RUnlock()

	requests := make([]*Request, 0, len(q.pending))
	for _, req := range q.pending {
		requests = append(requests, req)
	}
	return requests
}

// checkTimeouts periodically checks for timed-out approval requests.
func (q *Queue) checkTimeouts() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		q.mu.Lock()
		now := time.Now()
		for id, req := range q.pending {
			deadline := req.CreatedAt.Add(req.Timeout)
			if now.After(deadline) {
				approved := req.TimeoutEffect == "allow"
				delete(q.pending, id)

				// Update store
				status := "timed_out"
				q.store.ResolveApproval(id, status, "timeout")

				// Notify waiting goroutine
				req.result <- Result{Approved: approved, ResolvedBy: "timeout"}

				q.logger.Warn("approval timed out",
					"approval_id", id,
					"default_effect", req.TimeoutEffect,
					"approved", approved,
				)
			}
		}
		q.mu.Unlock()
	}
}

func (q *Queue) cleanup(approvalID string) {
	q.mu.Lock()
	delete(q.pending, approvalID)
	q.mu.Unlock()
	q.store.ResolveApproval(approvalID, "timed_out", "context_cancelled")
}
