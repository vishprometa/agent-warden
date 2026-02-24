import { ManagementClient } from "./management.js";

const DEFAULT_PROXY_URL = "http://localhost:6777";

/**
 * Header keys used by the AgentWarden proxy for request metadata.
 * These align with the Go constants in internal/proxy/proxy.go.
 */
export const HEADER_AGENT_ID = "X-AgentWarden-Agent-Id";
export const HEADER_SESSION_ID = "X-AgentWarden-Session-Id";
export const HEADER_METADATA = "X-AgentWarden-Metadata";
export const HEADER_TRACE_ID = "X-AgentWarden-Trace-Id";

/**
 * Configuration for the AgentWarden SDK.
 */
export interface AgentWardenConfig {
  /**
   * Base URL of the AgentWarden proxy.
   * @default "http://localhost:6777"
   */
  proxyUrl?: string;

  /** Unique identifier for this agent. Sent as `X-AgentWarden-Agent-Id`. */
  agentId: string;

  /**
   * Optional session ID. When provided, all requests are grouped under this
   * session. When omitted, the proxy auto-creates sessions.
   */
  sessionId?: string;

  /**
   * Arbitrary metadata attached to every proxied request via the
   * `X-AgentWarden-Metadata` header (JSON-encoded).
   */
  metadata?: Record<string, unknown>;
}

/**
 * The main AgentWarden SDK class.
 *
 * Wraps an OpenAI-compatible client to route requests through the
 * AgentWarden governance proxy. Also provides access to the management API.
 *
 * @example
 * ```typescript
 * import { AgentWarden } from '@agentwarden/sdk';
 * import OpenAI from 'openai';
 *
 * const warden = new AgentWarden({ agentId: 'my-agent' });
 * const client = warden.wrapOpenAI(new OpenAI());
 *
 * // All requests now flow through the AgentWarden proxy
 * const response = await client.chat.completions.create({
 *   model: 'gpt-4',
 *   messages: [{ role: 'user', content: 'Hello' }],
 * });
 * ```
 */
export class AgentWarden {
  private readonly config: Required<Pick<AgentWardenConfig, "proxyUrl" | "agentId">> &
    Pick<AgentWardenConfig, "sessionId" | "metadata">;

  private _management: ManagementClient | undefined;

  constructor(config: AgentWardenConfig) {
    this.config = {
      proxyUrl: (config.proxyUrl ?? DEFAULT_PROXY_URL).replace(/\/+$/, ""),
      agentId: config.agentId,
      sessionId: config.sessionId,
      metadata: config.metadata,
    };
  }

  // -----------------------------------------------------------------------
  // Public API
  // -----------------------------------------------------------------------

  /**
   * Wraps an OpenAI client instance to route all requests through the
   * AgentWarden proxy.
   *
   * Works by modifying the client's `baseURL` to point at the proxy and
   * injecting AgentWarden headers into `defaultHeaders`. The proxy then
   * forwards the request to the real upstream (OpenAI, Anthropic, etc.)
   * with the original `Authorization` header intact.
   *
   * The original client is mutated and returned for convenience.
   *
   * @param client - An OpenAI SDK client instance (or any API client with
   *   `baseURL` and `defaultHeaders` properties).
   * @returns The same client, now routing through the proxy.
   */
  wrapOpenAI<T extends OpenAILike>(client: T): T {
    // Point the client at the proxy instead of the upstream provider.
    // The proxy reads the original path (e.g. /v1/chat/completions)
    // and forwards it to the configured upstream.
    client.baseURL = this.getBaseUrl();

    // Merge AgentWarden headers into the client's default headers.
    const wardenHeaders = this.getHeaders();
    const existing = client.defaultHeaders ?? {};

    client.defaultHeaders = {
      ...existing,
      ...wardenHeaders,
    };

    return client;
  }

  /**
   * Returns the proxy base URL suitable for use as an OpenAI `baseURL`.
   * Includes the `/v1` path suffix expected by OpenAI-compatible clients.
   */
  getBaseUrl(): string {
    return `${this.config.proxyUrl}/v1`;
  }

  /**
   * Returns the headers that AgentWarden injects into every proxied request.
   * Useful when integrating with HTTP clients that don't follow the OpenAI
   * SDK pattern.
   */
  getHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      [HEADER_AGENT_ID]: this.config.agentId,
    };

    if (this.config.sessionId) {
      headers[HEADER_SESSION_ID] = this.config.sessionId;
    }

    if (this.config.metadata && Object.keys(this.config.metadata).length > 0) {
      headers[HEADER_METADATA] = JSON.stringify(this.config.metadata);
    }

    return headers;
  }

  /**
   * Creates a new `AgentWarden` instance scoped to a specific session.
   * All other configuration (proxyUrl, agentId, metadata) is inherited.
   *
   * @param sessionId - Session ID to scope to. If omitted, a new session
   *   will be auto-created by the proxy on the first request.
   */
  withSession(sessionId?: string): AgentWarden {
    return new AgentWarden({
      proxyUrl: this.config.proxyUrl,
      agentId: this.config.agentId,
      sessionId,
      metadata: this.config.metadata,
    });
  }

  /**
   * Returns a lazily-initialized management API client.
   * The management API shares the same base URL as the proxy.
   */
  management(): ManagementClient {
    if (!this._management) {
      this._management = new ManagementClient(this.config.proxyUrl);
    }
    return this._management;
  }
}

// ---------------------------------------------------------------------------
// Internal type for duck-typing OpenAI-compatible clients
// ---------------------------------------------------------------------------

/**
 * Minimal interface for OpenAI-compatible client instances.
 * This allows `wrapOpenAI` to work without importing the openai package.
 */
interface OpenAILike {
  baseURL: string;
  defaultHeaders?: Record<string, string | null | undefined>;
}
