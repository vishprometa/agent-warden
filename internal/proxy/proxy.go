// Package proxy implements the AgentWarden HTTP reverse proxy. It intercepts
// requests between AI agents and upstream LLM providers, applying governance
// policies, tracking costs, detecting anomalies, and producing an immutable
// audit trail of every action.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"

	"github.com/oklog/ulid/v2"
)

// Header keys used by AgentWarden for request/response metadata.
const (
	HeaderAgentID   = "X-AgentWarden-Agent-Id"
	HeaderSessionID = "X-AgentWarden-Session-Id"
	HeaderMetadata  = "X-AgentWarden-Metadata"
	HeaderTraceID   = "X-AgentWarden-Trace-Id"
)

// PolicyEngine evaluates governance policies against an action context.
// Implemented by the policy package.
type PolicyEngine interface {
	// Evaluate checks all policies against the given context and returns
	// the resulting status, the matched policy name, and a human-readable reason.
	Evaluate(ctx ActionContext) (trace.TraceStatus, string, string)
}

// DetectionEngine performs anomaly detection on intercepted actions.
// Implemented by the detection package.
type DetectionEngine interface {
	// Feed provides a trace to the detection engine for analysis.
	// Returns an anomaly description if one is detected, or empty string.
	Feed(t *trace.Trace) string
}

// CostTracker estimates token counts and costs for LLM requests.
// Implemented by the cost package.
type CostTracker interface {
	// CountTokens estimates input and output token counts from request/response bodies.
	CountTokens(model string, requestBody, responseBody []byte) (tokensIn, tokensOut int)

	// CalculateCost computes the USD cost given model, token counts.
	CalculateCost(model string, tokensIn, tokensOut int) float64
}

// AlertManager dispatches alerts for policy violations and anomalies.
// Implemented by the alert package.
type AlertManager interface {
	// SendAlert dispatches an alert with the given severity and message.
	SendAlert(severity, message string, details map[string]any)
}

// ActionContext holds the full context for a single intercepted action,
// used by the policy engine for evaluation.
type ActionContext struct {
	SessionID    string
	AgentID      string
	ActionType   trace.ActionType
	ActionName   string
	Model        string
	RequestBody  json.RawMessage
	SessionCost  float64
	ActionCount  int
	ActionWindow func(actionType string, window time.Duration) int
}

// Proxy is the central HTTP reverse proxy that intercepts agent-to-LLM traffic.
// It coordinates classification, policy evaluation, cost tracking, anomaly
// detection, and trace storage for every request.
type Proxy struct {
	cfg        *config.Config
	store      trace.Store
	sessions   *session.Manager
	classifier *Classifier
	router     *Router
	streamer   *SSEStreamer

	// Pluggable engines. These are optional; if nil, the corresponding
	// step in the pipeline is skipped.
	policyEngine    PolicyEngine
	detectionEngine DetectionEngine
	costTracker     CostTracker
	alertManager    AlertManager

	server *http.Server
	logger *slog.Logger
}

// Option configures the Proxy via functional options.
type Option func(*Proxy)

// WithPolicyEngine sets the policy evaluation engine.
func WithPolicyEngine(e PolicyEngine) Option {
	return func(p *Proxy) { p.policyEngine = e }
}

// WithDetectionEngine sets the anomaly detection engine.
func WithDetectionEngine(e DetectionEngine) Option {
	return func(p *Proxy) { p.detectionEngine = e }
}

// WithCostTracker sets the cost tracking engine.
func WithCostTracker(t CostTracker) Option {
	return func(p *Proxy) { p.costTracker = t }
}

// WithAlertManager sets the alert dispatcher.
func WithAlertManager(m AlertManager) Option {
	return func(p *Proxy) { p.alertManager = m }
}

// New creates a new Proxy with the given configuration, store, and options.
func New(cfg *config.Config, store trace.Store, logger *slog.Logger, opts ...Option) *Proxy {
	if logger == nil {
		logger = slog.Default()
	}

	p := &Proxy{
		cfg:        cfg,
		store:      store,
		sessions:   session.NewManager(store, logger),
		classifier: NewClassifier(logger),
		router:     NewRouter(&cfg.Upstream, logger),
		streamer:   NewSSEStreamer(logger),
		logger:     logger.With("component", "proxy.Proxy"),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// SessionManager returns the proxy's session manager for external access
// (e.g., by the API layer).
func (p *Proxy) SessionManager() *session.Manager {
	return p.sessions
}

// Start begins listening on the specified port. This method blocks until
// the server is shut down.
func (p *Proxy) Start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleRequest)

	p.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disabled for SSE streaming.
		IdleTimeout:  120 * time.Second,
	}

	p.logger.Info("proxy starting", "port", port)
	return p.server.ListenAndServe()
}

// ServeHTTP implements http.Handler, allowing the proxy to be mounted
// in a larger HTTP server alongside other handlers (e.g., the management API).
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handleRequest(w, r)
}

// Shutdown gracefully stops the proxy server.
func (p *Proxy) Shutdown() error {
	if p.server == nil {
		return nil
	}
	p.logger.Info("proxy shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.server.Shutdown(ctx)
}

// handleRequest is the main HTTP handler implementing the full interception pipeline.
func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// --- Step 1: Extract AgentWarden headers ---
	agentID := r.Header.Get(HeaderAgentID)
	sessionID := r.Header.Get(HeaderSessionID)
	metadataRaw := r.Header.Get(HeaderMetadata)

	// Default agent ID if not provided.
	if agentID == "" {
		agentID = "anonymous"
	}

	// Parse metadata header as JSON if present.
	var metadata json.RawMessage
	if metadataRaw != "" {
		metadata = json.RawMessage(metadataRaw)
		// Validate it's valid JSON.
		if !json.Valid(metadata) {
			metadata = nil
		}
	}

	// --- Step 2: Create or retrieve session ---
	sess, err := p.sessions.GetOrCreate(agentID, sessionID, metadata)
	if err != nil {
		p.logger.Error("failed to get/create session", "error", err)
		p.respondError(w, http.StatusInternalServerError, "session_error", "Failed to initialize session")
		return
	}

	// Check if session is paused.
	if p.sessions.IsPaused(sess.ID) {
		p.respondError(w, http.StatusServiceUnavailable, "session_paused", "Session is paused due to detected anomaly")
		return
	}

	// --- Step 3: Capture request body ---
	reqBody, err := CaptureRequestBody(r)
	if err != nil {
		p.logger.Error("failed to capture request body", "error", err)
		p.respondError(w, http.StatusBadRequest, "body_read_error", "Failed to read request body")
		return
	}

	// --- Step 4: Classify the action ---
	actionType, actionName, model := p.classifier.Classify(r, reqBody)

	// --- Step 5: Build action context and evaluate policies ---
	traceID := generateTraceID()

	if p.policyEngine != nil {
		actCtx := ActionContext{
			SessionID:   sess.ID,
			AgentID:     agentID,
			ActionType:  actionType,
			ActionName:  actionName,
			Model:       model,
			RequestBody: reqBody,
			SessionCost: p.sessions.TotalCost(sess.ID),
			ActionCount: sess.ActionCount,
			ActionWindow: func(at string, window time.Duration) int {
				return p.sessions.GetActionCount(sess.ID, at, window)
			},
		}

		status, policyName, reason := p.policyEngine.Evaluate(actCtx)

		if status == trace.StatusDenied || status == trace.StatusTerminated {
			p.logger.Warn("request blocked by policy",
				"trace_id", traceID,
				"session_id", sess.ID,
				"agent_id", agentID,
				"policy", policyName,
				"effect", status,
				"reason", reason,
			)

			// Store the denied/terminated trace.
			t := &trace.Trace{
				ID:           traceID,
				SessionID:    sess.ID,
				AgentID:      agentID,
				Timestamp:    startTime,
				ActionType:   actionType,
				ActionName:   actionName,
				RequestBody:  reqBody,
				Status:       status,
				PolicyName:   policyName,
				PolicyReason: reason,
				LatencyMs:    time.Since(startTime).Milliseconds(),
				Model:        model,
			}
			p.storeTrace(t, sess.ID)

			// Increment action count even for denied requests.
			p.sessions.IncrementActions(sess.ID, actionType)

			// If terminated, end the session.
			if status == trace.StatusTerminated {
				if err := p.sessions.Terminate(sess.ID); err != nil {
					p.logger.Error("failed to terminate session", "session_id", sess.ID, "error", err)
				}
				if p.alertManager != nil {
					p.alertManager.SendAlert("critical", fmt.Sprintf("Session terminated: %s", reason), map[string]any{
						"session_id":  sess.ID,
						"agent_id":    agentID,
						"policy_name": policyName,
					})
				}
			}

			statusCode := http.StatusForbidden
			if status == trace.StatusTerminated {
				statusCode = http.StatusServiceUnavailable
			}
			p.respondPolicyDenied(w, statusCode, traceID, policyName, reason, string(status))
			return
		}
	}

	// --- Step 6: Resolve upstream and forward request ---
	upstreamBase := p.router.ResolveUpstream(model)
	upstreamURL, err := url.Parse(upstreamBase)
	if err != nil {
		p.logger.Error("invalid upstream URL", "upstream", upstreamBase, "error", err)
		p.respondError(w, http.StatusBadGateway, "upstream_error", "Invalid upstream configuration")
		return
	}

	// Remove AgentWarden headers before forwarding upstream.
	r.Header.Del(HeaderAgentID)
	r.Header.Del(HeaderSessionID)
	r.Header.Del(HeaderMetadata)

	// Create a reverse proxy for this request.
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = upstreamURL.Scheme
			req.URL.Host = upstreamURL.Host
			// Preserve the original path; upstream base path is prepended if the
			// request path doesn't already include it.
			if !strings.HasPrefix(req.URL.Path, upstreamURL.Path) {
				req.URL.Path = singleJoiningSlash(upstreamURL.Path, req.URL.Path)
			}
			req.Host = upstreamURL.Host
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			p.logger.Error("upstream request failed",
				"trace_id", traceID,
				"upstream", upstreamBase,
				"error", err,
			)
			p.respondError(rw, http.StatusBadGateway, "upstream_error",
				fmt.Sprintf("Upstream request failed: %v", err))
		},
		ModifyResponse: func(resp *http.Response) error {
			// Inject the trace ID header into the upstream response.
			resp.Header.Set(HeaderTraceID, traceID)
			return nil
		},
	}

	// Use the response recorder to capture the response.
	recorder := newResponseRecorder(w)

	// Check if this is likely to be a streaming response (based on request body).
	isStreamReq := isStreamingRequest(reqBody)

	if isStreamReq {
		// For streaming requests, we use a custom transport that lets us
		// intercept the response and stream SSE events through.
		p.handleStreamingRequest(recorder, r, proxy, upstreamURL, traceID, sess, agentID, actionType, actionName, model, reqBody, startTime)
	} else {
		// For non-streaming requests, use the standard reverse proxy.
		proxy.ServeHTTP(recorder, r)
		p.handleNonStreamingResponse(recorder, traceID, sess, agentID, actionType, actionName, model, reqBody, startTime)
	}
}

// handleStreamingRequest handles SSE streaming responses by forwarding events
// in real time while accumulating the body for trace storage.
func (p *Proxy) handleStreamingRequest(w http.ResponseWriter, r *http.Request, rp *httputil.ReverseProxy, upstream *url.URL, traceID string, sess *trace.Session, agentID string, actionType trace.ActionType, actionName, model string, reqBody []byte, startTime time.Time) {
	// Make the upstream request manually for streaming control.
	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = upstream.Scheme
	outReq.URL.Host = upstream.Host
	if !strings.HasPrefix(outReq.URL.Path, upstream.Path) {
		outReq.URL.Path = singleJoiningSlash(upstream.Path, outReq.URL.Path)
	}
	outReq.Host = upstream.Host
	outReq.RequestURI = ""

	client := &http.Client{
		Timeout: p.cfg.Upstream.Timeout,
		// Do not follow redirects automatically for proxy requests.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(outReq)
	if err != nil {
		p.logger.Error("upstream streaming request failed",
			"trace_id", traceID,
			"error", err,
		)
		p.respondError(w, http.StatusBadGateway, "upstream_error",
			fmt.Sprintf("Upstream request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// Inject trace ID.
	w.Header().Set(HeaderTraceID, traceID)

	var respBody []byte

	if IsSSEResponse(resp) {
		// Stream SSE events through to the client.
		var streamErr error
		respBody, streamErr = p.streamer.StreamSSE(w, resp)
		if streamErr != nil {
			p.logger.Debug("SSE stream ended with error", "trace_id", traceID, "error", streamErr)
		}
	} else {
		// Not actually SSE despite streaming request flag; handle as normal.
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		respBody, _ = io.ReadAll(resp.Body)
		w.Write(respBody)
	}

	p.finalizeTrace(traceID, sess, agentID, actionType, actionName, model, reqBody, respBody, startTime, resp.StatusCode)
}

// handleNonStreamingResponse processes the captured response from a non-streaming
// reverse proxy call.
func (p *Proxy) handleNonStreamingResponse(recorder *responseRecorder, traceID string, sess *trace.Session, agentID string, actionType trace.ActionType, actionName, model string, reqBody []byte, startTime time.Time) {
	respBody := recorder.Body()
	p.finalizeTrace(traceID, sess, agentID, actionType, actionName, model, reqBody, respBody, startTime, recorder.StatusCode())
}

// finalizeTrace performs post-response processing: token counting, cost tracking,
// trace storage, anomaly detection, and action counting.
func (p *Proxy) finalizeTrace(traceID string, sess *trace.Session, agentID string, actionType trace.ActionType, actionName, model string, reqBody, respBody []byte, startTime time.Time, statusCode int) {
	latencyMs := time.Since(startTime).Milliseconds()

	// --- Token counting and cost calculation ---
	var tokensIn, tokensOut int
	var costUSD float64

	if p.costTracker != nil {
		tokensIn, tokensOut = p.costTracker.CountTokens(model, reqBody, respBody)
		costUSD = p.costTracker.CalculateCost(model, tokensIn, tokensOut)
	}

	// --- Determine trace status ---
	status := trace.StatusAllowed
	if statusCode >= 400 {
		status = trace.TraceStatus(fmt.Sprintf("upstream_error_%d", statusCode))
	}

	// --- Build and store trace ---
	t := &trace.Trace{
		ID:           traceID,
		SessionID:    sess.ID,
		AgentID:      agentID,
		Timestamp:    startTime,
		ActionType:   actionType,
		ActionName:   actionName,
		RequestBody:  sanitizeBody(reqBody),
		ResponseBody: sanitizeBody(respBody),
		Status:       status,
		LatencyMs:    latencyMs,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		CostUSD:      costUSD,
		Model:        model,
	}
	p.storeTrace(t, sess.ID)

	// --- Update session cost and action count ---
	if costUSD > 0 {
		if err := p.sessions.AddCost(sess.ID, costUSD); err != nil {
			p.logger.Error("failed to add cost to session", "session_id", sess.ID, "error", err)
		}
	}
	p.sessions.IncrementActions(sess.ID, actionType)

	// --- Feed detection engine ---
	if p.detectionEngine != nil {
		if anomaly := p.detectionEngine.Feed(t); anomaly != "" {
			p.logger.Warn("anomaly detected",
				"trace_id", traceID,
				"session_id", sess.ID,
				"anomaly", anomaly,
			)
			if p.alertManager != nil {
				p.alertManager.SendAlert("warning", anomaly, map[string]any{
					"trace_id":   traceID,
					"session_id": sess.ID,
					"agent_id":   agentID,
				})
			}
		}
	}

	p.logger.Info("request completed",
		"trace_id", traceID,
		"session_id", sess.ID,
		"agent_id", agentID,
		"action_type", actionType,
		"model", model,
		"latency_ms", latencyMs,
		"tokens_in", tokensIn,
		"tokens_out", tokensOut,
		"cost_usd", costUSD,
		"status_code", statusCode,
	)
}

// storeTrace computes the hash chain and persists a trace record.
func (p *Proxy) storeTrace(t *trace.Trace, sessionID string) {
	// Fetch the last trace hash for this session to maintain the chain.
	traces, _, err := p.store.ListTraces(trace.TraceFilter{
		SessionID: sessionID,
		Limit:     1,
	})
	if err != nil {
		p.logger.Error("failed to fetch last trace for hash chain", "session_id", sessionID, "error", err)
		t.PrevHash = trace.ComputeSessionSeed(sessionID)
	} else if len(traces) > 0 {
		t.PrevHash = traces[0].Hash
	} else {
		t.PrevHash = trace.ComputeSessionSeed(sessionID)
	}

	t.Hash = trace.ComputeHash(t)

	if err := p.store.InsertTrace(t); err != nil {
		p.logger.Error("failed to store trace",
			"trace_id", t.ID,
			"session_id", sessionID,
			"error", err,
		)
	}
}

// respondError writes a JSON error response.
func (p *Proxy) respondError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// respondPolicyDenied writes a JSON error response for policy denials with
// additional policy context.
func (p *Proxy) respondPolicyDenied(w http.ResponseWriter, statusCode int, traceID, policyName, reason, effect string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(HeaderTraceID, traceID)
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "policy_" + effect,
			"message": reason,
			"policy":  policyName,
			"effect":  effect,
		},
		"trace_id": traceID,
	})
}

// generateTraceID creates a ULID-based trace ID for globally unique,
// time-ordered identification.
func generateTraceID() string {
	return ulid.Make().String()
}

// isStreamingRequest checks whether the request body indicates a streaming
// request (e.g., "stream": true in the JSON body).
func isStreamingRequest(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var parsed struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return parsed.Stream
}

// sanitizeBody truncates very large bodies to prevent storage bloat.
// Bodies over 1MB are truncated with a marker.
func sanitizeBody(body []byte) json.RawMessage {
	if body == nil || len(body) == 0 {
		return nil
	}
	const maxBodySize = 1024 * 1024 // 1 MB
	if len(body) > maxBodySize {
		truncated := make([]byte, maxBodySize)
		copy(truncated, body[:maxBodySize])
		return json.RawMessage(truncated)
	}
	return json.RawMessage(body)
}

// singleJoiningSlash joins a base path and a relative path with exactly
// one slash between them.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
