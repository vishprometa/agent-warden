import type {
  GetSessionResponse,
  HealthResponse,
  ListAgentsResponse,
  ListApprovalsResponse,
  ListPoliciesResponse,
  ListSessionsResponse,
  ListTracesResponse,
  ListViolationsResponse,
  SearchTracesResponse,
  SessionFilter,
  StatusResponse,
  SystemStats,
  Trace,
  TraceFilter,
} from "./types.js";
import { AgentWardenError } from "./types.js";

const DEFAULT_MANAGEMENT_URL = "http://localhost:6777";

/**
 * Build a URL with query parameters, omitting undefined/null values.
 */
function buildUrl(base: string, path: string, params?: Record<string, unknown>): string {
  const url = new URL(path, base);
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url.toString();
}

/**
 * Client for the AgentWarden management API.
 *
 * All methods use native `fetch` — zero external dependencies.
 * The management API runs on the same port as the proxy by default
 * (routes are mounted under `/api/`).
 *
 * @example
 * ```typescript
 * import { ManagementClient } from '@agentwarden/sdk/management';
 *
 * const mgmt = new ManagementClient('http://localhost:6777');
 * const { sessions } = await mgmt.listSessions({ status: 'active' });
 * ```
 */
export class ManagementClient {
  private readonly baseUrl: string;

  constructor(baseUrl?: string) {
    this.baseUrl = (baseUrl ?? DEFAULT_MANAGEMENT_URL).replace(/\/+$/, "");
  }

  // -----------------------------------------------------------------------
  // Internal helpers
  // -----------------------------------------------------------------------

  private async request<T>(method: string, path: string, params?: Record<string, unknown>): Promise<T> {
    const url = buildUrl(this.baseUrl, path, method === "GET" ? params : undefined);

    const init: RequestInit = {
      method,
      headers: { "Content-Type": "application/json" },
    };

    if (method !== "GET" && params) {
      init.body = JSON.stringify(params);
    }

    const res = await fetch(url, init);

    if (!res.ok) {
      let body: unknown;
      try {
        body = await res.json();
      } catch {
        body = await res.text();
      }
      throw new AgentWardenError(
        `AgentWarden API error: ${res.status} ${res.statusText}`,
        res.status,
        body,
      );
    }

    return (await res.json()) as T;
  }

  private get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
    return this.request<T>("GET", path, params);
  }

  private post<T>(path: string, params?: Record<string, unknown>): Promise<T> {
    return this.request<T>("POST", path, params);
  }

  private del<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  // -----------------------------------------------------------------------
  // System
  // -----------------------------------------------------------------------

  /** Health check. Returns `{ status: "ok" }` when the server is running. */
  health(): Promise<HealthResponse> {
    return this.get("/api/health");
  }

  /** Aggregate system-wide metrics. */
  stats(): Promise<SystemStats> {
    return this.get("/api/stats");
  }

  // -----------------------------------------------------------------------
  // Sessions
  // -----------------------------------------------------------------------

  /** List sessions with optional filtering. */
  listSessions(params?: SessionFilter): Promise<ListSessionsResponse> {
    return this.get("/api/sessions", params as Record<string, unknown>);
  }

  /** Get a single session and its associated traces. */
  getSession(id: string): Promise<GetSessionResponse> {
    return this.get(`/api/sessions/${encodeURIComponent(id)}`);
  }

  /** Terminate an active session. */
  terminateSession(id: string): Promise<StatusResponse> {
    return this.del(`/api/sessions/${encodeURIComponent(id)}`);
  }

  // -----------------------------------------------------------------------
  // Traces
  // -----------------------------------------------------------------------

  /** List traces with optional filtering. */
  listTraces(params?: TraceFilter): Promise<ListTracesResponse> {
    return this.get("/api/traces", params as Record<string, unknown>);
  }

  /** Get a single trace by ID. */
  getTrace(id: string): Promise<Trace> {
    return this.get(`/api/traces/${encodeURIComponent(id)}`);
  }

  /** Full-text search across traces. */
  searchTraces(query: string, limit?: number): Promise<SearchTracesResponse> {
    return this.get("/api/traces/search", { q: query, limit });
  }

  // -----------------------------------------------------------------------
  // Agents
  // -----------------------------------------------------------------------

  /** List all registered agents. */
  listAgents(): Promise<ListAgentsResponse> {
    return this.get("/api/agents");
  }

  // -----------------------------------------------------------------------
  // Policies
  // -----------------------------------------------------------------------

  /** List all configured policies. */
  listPolicies(): Promise<ListPoliciesResponse> {
    return this.get("/api/policies");
  }

  /** Hot-reload policies from the configuration file. */
  reloadPolicies(): Promise<StatusResponse> {
    return this.post("/api/policies/reload");
  }

  // -----------------------------------------------------------------------
  // Approvals
  // -----------------------------------------------------------------------

  /** List pending human-in-the-loop approval requests. */
  listApprovals(): Promise<ListApprovalsResponse> {
    return this.get("/api/approvals");
  }

  /** Approve a pending action. */
  approve(id: string): Promise<StatusResponse> {
    return this.post(`/api/approvals/${encodeURIComponent(id)}/approve`);
  }

  /** Deny a pending action. */
  deny(id: string): Promise<StatusResponse> {
    return this.post(`/api/approvals/${encodeURIComponent(id)}/deny`);
  }

  // -----------------------------------------------------------------------
  // Violations
  // -----------------------------------------------------------------------

  /** List policy violations, optionally filtered by agent ID. */
  listViolations(agentId?: string, limit?: number): Promise<ListViolationsResponse> {
    return this.get("/api/violations", { agent_id: agentId, limit });
  }
}
