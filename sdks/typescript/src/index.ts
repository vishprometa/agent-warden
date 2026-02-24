/**
 * @agentwarden/sdk — TypeScript SDK for AgentWarden runtime governance proxy.
 *
 * @example
 * ```typescript
 * import { AgentWarden } from '@agentwarden/sdk';
 * import OpenAI from 'openai';
 *
 * const warden = new AgentWarden({ agentId: 'my-agent' });
 * const client = warden.wrapOpenAI(new OpenAI());
 * ```
 *
 * @packageDocumentation
 */

// Main client
export { AgentWarden, HEADER_AGENT_ID, HEADER_SESSION_ID, HEADER_METADATA, HEADER_TRACE_ID } from "./client.js";
export type { AgentWardenConfig } from "./client.js";

// Management API client
export { ManagementClient } from "./management.js";

// Proxy configuration
export { createWardenProxy } from "./proxy.js";

// Auto-discovery
export { discover, autoConnect } from "./discovery.js";

// Session management
export { WardenSession } from "./session.js";

// Express/Hono middleware
export { wardenMiddleware } from "./middleware.js";

// All types
export type {
  // Domain models
  Trace,
  Session,
  Agent,
  AgentStats,
  PolicyConfig,
  Approval,
  Violation,
  SystemStats,

  // Enums / unions
  ActionType,
  TraceStatus,
  SessionStatus,
  PolicyEffect,
  ApprovalStatus,

  // Query filters
  SessionFilter,
  TraceFilter,

  // Response wrappers
  ListSessionsResponse,
  GetSessionResponse,
  ListTracesResponse,
  SearchTracesResponse,
  ListAgentsResponse,
  ListPoliciesResponse,
  ListApprovalsResponse,
  ListViolationsResponse,
  StatusResponse,
  HealthResponse,

  // Proxy types
  WardenProxyOptions,
  WardenProxy,
  WardenProxyConfig,

  // Middleware types
  MiddlewareOptions,
  MiddlewareRequest,
  MiddlewareResponse,
  MiddlewareNext,

  // Session types
  SessionMetrics,
} from "./types.js";

// Error class (re-exported as value, not just type)
export { AgentWardenError } from "./types.js";
