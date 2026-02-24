/**
 * Action types categorizing intercepted agent operations.
 */
export type ActionType =
  | "llm.chat"
  | "llm.embedding"
  | "tool.call"
  | "api.request"
  | "db.query"
  | "file.write"
  | "code.exec"
  | "mcp.tool";

/**
 * Policy evaluation result status for a trace.
 */
export type TraceStatus =
  | "allowed"
  | "denied"
  | "terminated"
  | "approved"
  | "pending"
  | "throttled";

/**
 * Session lifecycle status.
 */
export type SessionStatus = "active" | "completed" | "terminated" | "paused";

/**
 * Policy effect when a condition matches.
 */
export type PolicyEffect = "allow" | "deny" | "terminate" | "throttle" | "approve";

/**
 * Approval lifecycle status.
 */
export type ApprovalStatus = "pending" | "approved" | "denied" | "timed_out";

// ---------------------------------------------------------------------------
// Core domain models — aligned with Go structs in internal/trace/models.go
// ---------------------------------------------------------------------------

/**
 * A single intercepted action with full governance context.
 * Each trace is part of a hash chain for tamper-evident audit trails.
 */
export interface Trace {
  id: string;
  session_id: string;
  agent_id: string;
  timestamp: string;
  action_type: ActionType;
  action_name?: string;
  request_body?: unknown;
  response_body?: unknown;
  status: TraceStatus;
  policy_name?: string;
  policy_reason?: string;
  latency_ms: number;
  tokens_in: number;
  tokens_out: number;
  cost_usd: number;
  metadata?: Record<string, unknown>;
  prev_hash: string;
  hash: string;
  model?: string;
}

/**
 * A group of related agent actions forming a logical unit of work.
 */
export interface Session {
  id: string;
  agent_id: string;
  started_at: string;
  ended_at?: string | null;
  status: SessionStatus;
  total_cost: number;
  action_count: number;
  metadata?: Record<string, unknown>;
  score?: Record<string, unknown>;
}

/**
 * A registered agent identity.
 */
export interface Agent {
  id: string;
  name: string;
  created_at: string;
  current_version?: string;
  config?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

/**
 * Aggregated metrics for a single agent.
 */
export interface AgentStats {
  agent_id: string;
  total_sessions: number;
  active_sessions: number;
  total_cost: number;
  total_actions: number;
  total_violations: number;
  avg_cost_per_session: number;
  completion_rate: number;
  error_rate: number;
}

/**
 * A governance policy configuration.
 */
export interface PolicyConfig {
  name: string;
  condition: string;
  effect: PolicyEffect;
  message?: string;
  type?: string;
  delay?: string;
  prompt?: string;
  model?: string;
  approvers?: string[];
  timeout?: string;
  timeout_effect?: string;
}

/**
 * A pending human-in-the-loop approval request.
 */
export interface Approval {
  id: string;
  session_id: string;
  trace_id: string;
  policy_name: string;
  action_summary?: unknown;
  status: ApprovalStatus;
  created_at: string;
  resolved_at?: string | null;
  resolved_by?: string;
  timeout_at: string;
}

/**
 * A recorded policy violation event.
 */
export interface Violation {
  id: string;
  trace_id: string;
  session_id: string;
  agent_id: string;
  policy_name: string;
  effect: string;
  timestamp: string;
  action_summary?: unknown;
}

/**
 * Aggregate system-wide metrics.
 */
export interface SystemStats {
  total_traces: number;
  total_sessions: number;
  active_sessions: number;
  total_agents: number;
  total_cost: number;
  total_violations: number;
  pending_approvals: number;
}

// ---------------------------------------------------------------------------
// Query filter types — aligned with Go filter structs
// ---------------------------------------------------------------------------

/**
 * Query parameters for listing sessions.
 */
export interface SessionFilter {
  agent_id?: string;
  status?: SessionStatus;
  since?: string;
  limit?: number;
  offset?: number;
}

/**
 * Query parameters for listing traces.
 */
export interface TraceFilter {
  session_id?: string;
  agent_id?: string;
  action_type?: ActionType;
  status?: TraceStatus;
  limit?: number;
  offset?: number;
}

// ---------------------------------------------------------------------------
// API response wrappers
// ---------------------------------------------------------------------------

export interface ListSessionsResponse {
  sessions: Session[];
  total: number;
}

export interface GetSessionResponse {
  session: Session;
  traces: Trace[];
}

export interface ListTracesResponse {
  traces: Trace[];
  total: number;
}

export interface SearchTracesResponse {
  traces: Trace[];
}

export interface ListAgentsResponse {
  agents: Agent[];
}

export interface ListPoliciesResponse {
  policies: PolicyConfig[];
}

export interface ListApprovalsResponse {
  approvals: Approval[];
}

export interface ListViolationsResponse {
  violations: Violation[];
}

export interface StatusResponse {
  status: string;
}

export interface HealthResponse {
  status: string;
}

// ---------------------------------------------------------------------------
// Proxy configuration types
// ---------------------------------------------------------------------------

/**
 * Options for creating a transparent AgentWarden proxy configuration.
 */
export interface WardenProxyOptions {
  /** Unique identifier for this agent. */
  agentId: string;

  /**
   * Base URL of the AgentWarden proxy.
   * @default "http://localhost:6777"
   */
  proxyUrl?: string;

  /** Optional session ID to scope all requests under. */
  sessionId?: string;

  /** Arbitrary metadata attached to every proxied request. */
  metadata?: Record<string, unknown>;
}

/**
 * Client configuration object that can be spread into any OpenAI-compatible
 * client constructor for zero-code-change integration.
 */
export interface WardenProxyConfig {
  /** The proxy base URL with `/v1` suffix for OpenAI compatibility. */
  baseURL: string;

  /** AgentWarden headers injected into every request. */
  defaultHeaders: Record<string, string>;
}

/**
 * A transparent proxy wrapper returned by `createWardenProxy()`.
 * Provides the raw config plus helper methods for session scoping
 * and management API access.
 */
export interface WardenProxy {
  /** Spread this into any OpenAI-compatible client constructor. */
  config: WardenProxyConfig;

  /**
   * Returns a new `WardenProxy` scoped to a specific session.
   * @param id - Session ID. If omitted, the proxy auto-creates one.
   */
  session(id?: string): WardenProxy;

  /** Returns a `ManagementClient` for the same proxy instance. */
  management(): import("./management.js").ManagementClient;
}

/**
 * Options for the Express/Hono middleware.
 */
export interface MiddlewareOptions {
  /** Unique identifier for this agent. */
  agentId: string;

  /**
   * Base URL of the AgentWarden proxy.
   * @default "http://localhost:6777"
   */
  proxyUrl?: string;

  /**
   * Extract a session ID from the incoming request.
   * When provided, the returned string is sent as `X-AgentWarden-Session-Id`.
   */
  sessionFrom?: (req: MiddlewareRequest) => string | undefined;

  /**
   * Extract arbitrary metadata from the incoming request.
   * The returned object is JSON-encoded into `X-AgentWarden-Metadata`.
   */
  metadataFrom?: (req: MiddlewareRequest) => Record<string, unknown> | undefined;

  /**
   * Route patterns to exclude from tracing.
   * Matched against `req.path` using simple string prefix comparison.
   */
  exclude?: string[];
}

/**
 * Minimal request interface used by the middleware.
 * Compatible with Express, Hono, and other Node.js HTTP frameworks.
 */
export interface MiddlewareRequest {
  method: string;
  path: string;
  url: string;
  headers: Record<string, string | string[] | undefined>;
}

/**
 * Minimal response interface used by the middleware.
 * Compatible with Express and similar frameworks.
 */
export interface MiddlewareResponse {
  setHeader(name: string, value: string): void;
  on(event: string, listener: (...args: unknown[]) => void): void;
}

/**
 * Standard Express/Connect-style next function.
 */
export type MiddlewareNext = (err?: unknown) => void;

/**
 * Performance metrics for scoring a session.
 */
export interface SessionMetrics {
  [key: string]: number;
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

/**
 * Error thrown by the SDK when a management API request fails.
 */
export class AgentWardenError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(message: string, status: number, body?: unknown) {
    super(message);
    this.name = "AgentWardenError";
    this.status = status;
    this.body = body;
  }
}
