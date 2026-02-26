// Package server implements the gRPC and HTTP event ingestion servers for
// AgentWarden. These are the real-time evaluation endpoints that SDKs
// call before/after every agent action. The management API lives separately
// in internal/api.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"
	pb "github.com/agentwarden/agentwarden/proto/agentwarden/v1"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
)

// PolicyEngine is the interface the gRPC server depends on for policy
// evaluation. Matches the concrete policy.Engine.Evaluate signature.
type PolicyEngine interface {
	Evaluate(ctx policy.ActionContext) policy.PolicyResult
}

// DetectionEngine is the interface for async anomaly detection.
type DetectionEngine interface {
	Analyze(event detection.ActionEvent)
}

// AlertManager is the interface for dispatching alerts.
type AlertManager interface {
	Send(a alert.Alert)
}

// GRPCServer implements the AgentWardenService gRPC interface. It is the
// primary real-time evaluation endpoint for agent SDKs.
type GRPCServer struct {
	pb.UnimplementedAgentWardenServiceServer

	policy    PolicyEngine
	store     trace.Store
	sessions  *session.Manager
	cost      *cost.Tracker
	detection DetectionEngine
	alerts    AlertManager
	logger    *slog.Logger

	grpcServer *grpc.Server
}

// NewGRPCServer creates a new GRPCServer wired to the given dependencies.
func NewGRPCServer(
	policyEngine PolicyEngine,
	store trace.Store,
	sessions *session.Manager,
	costTracker *cost.Tracker,
	detectionEngine DetectionEngine,
	alertManager AlertManager,
	logger *slog.Logger,
) *GRPCServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &GRPCServer{
		policy:    policyEngine,
		store:     store,
		sessions:  sessions,
		cost:      costTracker,
		detection: detectionEngine,
		alerts:    alertManager,
		logger:    logger.With("component", "server.gRPC"),
	}
}

// Start binds the gRPC server on the given port and begins serving.
// This call blocks until the server is stopped.
func (s *GRPCServer) Start(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	s.grpcServer = grpc.NewServer()
	pb.RegisterAgentWardenServiceServer(s.grpcServer, s)

	s.logger.Info("gRPC server listening", "port", port)
	return s.grpcServer.Serve(lis)
}

// Stop gracefully shuts down the gRPC server.
func (s *GRPCServer) Stop() {
	if s.grpcServer != nil {
		s.logger.Info("gRPC server shutting down")
		s.grpcServer.GracefulStop()
	}
}

// ---------------------------------------------------------------------------
// RPC implementations
// ---------------------------------------------------------------------------

// EvaluateAction evaluates a single action event against loaded policies,
// records a trace, fires async detection, and returns the verdict.
func (s *GRPCServer) EvaluateAction(ctx context.Context, req *pb.ActionEvent) (*pb.Verdict, error) {
	start := time.Now()

	if req.Action == nil {
		return nil, fmt.Errorf("action is required")
	}

	// Build policy context.
	policyCtx := s.buildPolicyContext(req)

	// Evaluate policies synchronously.
	result := s.policy.Evaluate(policyCtx)

	latencyMs := int32(time.Since(start).Milliseconds())
	traceID := ulid.Make().String()

	// Record trace (fire-and-forget; don't block the verdict).
	go s.recordTrace(req, result, traceID, latencyMs)

	// Fire async detection.
	go s.runDetection(req)

	// If the verdict is deny/terminate, send an alert.
	if result.Effect == policy.EffectDeny || result.Effect == policy.EffectTerminate {
		go s.sendViolationAlert(req, result)
	}

	verdict := &pb.Verdict{
		Verdict:    result.Effect,
		TraceId:    traceID,
		PolicyName: result.PolicyName,
		Message:    result.Message,
		LatencyMs:  latencyMs,
	}

	if result.Effect == policy.EffectApprove {
		verdict.ApprovalId = traceID // use trace ID as approval reference
		verdict.TimeoutSeconds = 300 // default 5 min approval timeout
	}

	return verdict, nil
}

// StartSession registers a new agent session.
func (s *GRPCServer) StartSession(ctx context.Context, req *pb.SessionStart) (*pb.SessionAck, error) {
	metadata, err := json.Marshal(req.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	sess, err := s.sessions.GetOrCreate(req.AgentId, req.SessionId, metadata)
	if err != nil {
		s.logger.Error("failed to start session",
			"session_id", req.SessionId,
			"agent_id", req.AgentId,
			"error", err,
		)
		return &pb.SessionAck{
			SessionId: req.SessionId,
			Ok:        false,
			Message:   err.Error(),
		}, nil
	}

	s.logger.Info("session started",
		"session_id", sess.ID,
		"agent_id", req.AgentId,
	)

	return &pb.SessionAck{
		SessionId: sess.ID,
		Ok:        true,
		Message:   "session registered",
	}, nil
}

// EndSession finalizes a session and returns its summary.
func (s *GRPCServer) EndSession(ctx context.Context, req *pb.SessionEnd) (*pb.SessionSummary, error) {
	sess := s.sessions.Get(req.SessionId)
	if sess == nil {
		return nil, fmt.Errorf("session %s not found", req.SessionId)
	}

	// Capture summary before ending.
	summary := &pb.SessionSummary{
		SessionId:   sess.ID,
		TotalCost:   sess.TotalCost,
		ActionCount: int32(sess.ActionCount),
		Status:      "completed",
	}

	if !sess.StartedAt.IsZero() {
		summary.DurationSeconds = int32(time.Since(sess.StartedAt).Seconds())
	}

	if err := s.sessions.End(req.SessionId); err != nil {
		s.logger.Error("failed to end session",
			"session_id", req.SessionId,
			"error", err,
		)
		return nil, fmt.Errorf("failed to end session: %w", err)
	}

	s.cost.ResetSession(req.SessionId)

	s.logger.Info("session ended",
		"session_id", req.SessionId,
		"total_cost", summary.TotalCost,
		"action_count", summary.ActionCount,
		"duration_s", summary.DurationSeconds,
	)

	return summary, nil
}

// ScoreSession stores a quality score on a completed session.
func (s *GRPCServer) ScoreSession(ctx context.Context, req *pb.SessionScore) (*pb.Ack, error) {
	scorePayload := map[string]interface{}{
		"task_completed": req.TaskCompleted,
		"quality":        req.Quality,
		"metrics":        req.Metrics,
	}
	scoreJSON, err := json.Marshal(scorePayload)
	if err != nil {
		return &pb.Ack{Ok: false, Message: "failed to marshal score"}, nil
	}

	if err := s.store.ScoreSession(req.SessionId, scoreJSON); err != nil {
		s.logger.Error("failed to score session",
			"session_id", req.SessionId,
			"error", err,
		)
		return &pb.Ack{Ok: false, Message: err.Error()}, nil
	}

	s.logger.Info("session scored",
		"session_id", req.SessionId,
		"quality", req.Quality,
		"task_completed", req.TaskCompleted,
	)

	return &pb.Ack{Ok: true, Message: "score recorded"}, nil
}

// StreamActions implements bidirectional streaming. For each incoming action
// event, the server evaluates policies and streams back the verdict. This is
// the high-performance path for agents that send many actions per session.
func (s *GRPCServer) StreamActions(stream pb.AgentWardenService_StreamActionsServer) error {
	s.logger.Info("stream opened")

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			s.logger.Info("stream closed by client")
			return nil
		}
		if err != nil {
			s.logger.Error("stream recv error", "error", err)
			return err
		}

		verdict, err := s.EvaluateAction(stream.Context(), req)
		if err != nil {
			s.logger.Error("stream evaluate error",
				"session_id", req.SessionId,
				"error", err,
			)
			// Send an error verdict rather than breaking the stream.
			verdict = &pb.Verdict{
				Verdict: policy.EffectDeny,
				Message: fmt.Sprintf("evaluation error: %s", err.Error()),
			}
		}

		if err := stream.Send(verdict); err != nil {
			s.logger.Error("stream send error", "error", err)
			return err
		}
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// buildPolicyContext translates a proto ActionEvent into the internal
// policy.ActionContext used by the policy engine.
func (s *GRPCServer) buildPolicyContext(req *pb.ActionEvent) policy.ActionContext {
	params := make(map[string]interface{})
	if req.Action.ParamsJson != "" {
		_ = json.Unmarshal([]byte(req.Action.ParamsJson), &params)
	}

	sessionCost := float64(0)
	actionCount := 0
	if req.Context != nil {
		sessionCost = req.Context.SessionCost
		actionCount = int(req.Context.SessionActionCount)
	}

	// Populate agent.daily_cost from the cost tracker.
	agentDailyCost := float64(0)
	if s.cost != nil {
		agentDailyCost = s.cost.GetAgentCost(req.AgentId)
	}

	return policy.ActionContext{
		Action: policy.ActionInfo{
			Type:   req.Action.Type,
			Name:   req.Action.Name,
			Params: params,
			Target: req.Action.Target,
		},
		Session: policy.SessionInfo{
			ID:          req.SessionId,
			AgentID:     req.AgentId,
			Cost:        sessionCost,
			ActionCount: actionCount,
		},
		Agent: policy.AgentInfo{
			ID:        req.AgentId,
			Name:      req.AgentId,
			DailyCost: agentDailyCost,
		},
	}
}

// recordTrace persists the action and its verdict to the trace store.
func (s *GRPCServer) recordTrace(req *pb.ActionEvent, result policy.PolicyResult, traceID string, latencyMs int32) {
	reqBody, _ := json.Marshal(req.Action)
	metaBytes, _ := json.Marshal(req.Metadata)

	status := trace.StatusAllowed
	switch result.Effect {
	case policy.EffectDeny:
		status = trace.StatusDenied
	case policy.EffectTerminate:
		status = trace.StatusTerminated
	case policy.EffectApprove:
		status = trace.StatusPending
	case policy.EffectThrottle:
		status = trace.StatusThrottled
	}

	t := &trace.Trace{
		ID:           traceID,
		SessionID:    req.SessionId,
		AgentID:      req.AgentId,
		Timestamp:    time.Now().UTC(),
		ActionType:   trace.ActionType(req.Action.Type),
		ActionName:   req.Action.Name,
		RequestBody:  reqBody,
		Status:       status,
		PolicyName:   result.PolicyName,
		PolicyReason: result.Message,
		LatencyMs:    int64(latencyMs),
		Metadata:     metaBytes,
	}

	if err := s.store.InsertTrace(t); err != nil {
		s.logger.Error("failed to insert trace",
			"trace_id", traceID,
			"error", err,
		)
	}

	// Update session action count.
	if err := s.sessions.IncrementActions(req.SessionId, trace.ActionType(req.Action.Type)); err != nil {
		s.logger.Warn("failed to increment session actions",
			"session_id", req.SessionId,
			"error", err,
		)
	}
}

// runDetection fires the async anomaly detection pipeline.
func (s *GRPCServer) runDetection(req *pb.ActionEvent) {
	if s.detection == nil {
		return
	}

	sessionCost := float64(0)
	if req.Context != nil {
		sessionCost = req.Context.SessionCost
	}

	s.detection.Analyze(detection.ActionEvent{
		SessionID:  req.SessionId,
		AgentID:    req.AgentId,
		ActionType: req.Action.Type,
		ActionName: req.Action.Name,
		Signature:  req.Action.Type + ":" + req.Action.Name + ":" + req.Action.Target,
		CostUSD:    sessionCost,
	})
}

// sendViolationAlert dispatches a policy violation alert.
func (s *GRPCServer) sendViolationAlert(req *pb.ActionEvent, result policy.PolicyResult) {
	if s.alerts == nil {
		return
	}

	severity := "warning"
	if result.Effect == policy.EffectTerminate {
		severity = "critical"
	}

	s.alerts.Send(alert.Alert{
		Type:      "policy_violation",
		Severity:  severity,
		Title:     fmt.Sprintf("Policy %q triggered: %s", result.PolicyName, result.Effect),
		Message:   result.Message,
		AgentID:   req.AgentId,
		SessionID: req.SessionId,
		Details: map[string]interface{}{
			"action_type": req.Action.Type,
			"action_name": req.Action.Name,
			"target":      req.Action.Target,
			"policy":      result.PolicyName,
			"effect":      result.Effect,
		},
	})
}
