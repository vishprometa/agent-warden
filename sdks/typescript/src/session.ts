import { AgentWarden } from "./client.js";
import type { SessionMetrics } from "./types.js";

/**
 * Enhanced session wrapper that provides scoped execution, scoring,
 * and lifecycle management for an AgentWarden session.
 *
 * A `WardenSession` is always tied to a specific `AgentWarden` client
 * and session ID. It wraps the management API for session-specific
 * operations while providing a clean interface for structured execution.
 *
 * @example
 * ```typescript
 * import { AgentWarden, WardenSession } from '@agentwarden/sdk';
 *
 * const warden = new AgentWarden({ agentId: 'my-agent' });
 * const session = new WardenSession(warden, 'task-123');
 *
 * await session.run(async (s) => {
 *   // All operations in this block are scoped to the session
 *   const client = s.wrapOpenAI(new OpenAI());
 *   const response = await client.chat.completions.create({ ... });
 *   return response;
 * });
 *
 * await session.score({ accuracy: 0.95, latency: 120 });
 * await session.end();
 * ```
 */
export class WardenSession {
  /**
   * The `AgentWarden` client scoped to this session.
   * Created lazily on first access.
   */
  private _scopedClient: AgentWarden | undefined;

  /**
   * Creates a new `WardenSession`.
   *
   * @param client - The parent `AgentWarden` client instance.
   * @param id - Unique session identifier. All requests made through
   *   this session are grouped under this ID.
   */
  constructor(
    private readonly client: AgentWarden,
    public readonly id: string,
  ) {}

  /**
   * Returns an `AgentWarden` client scoped to this session.
   * The scoped client inherits all configuration from the parent
   * but attaches this session's ID to every request.
   */
  private getScopedClient(): AgentWarden {
    if (!this._scopedClient) {
      this._scopedClient = this.client.withSession(this.id);
    }
    return this._scopedClient;
  }

  /**
   * Wraps an OpenAI-compatible client to route requests through this session.
   *
   * Convenience method equivalent to `client.withSession(id).wrapOpenAI(openai)`.
   *
   * @param openaiClient - An OpenAI SDK client instance (or any client with
   *   `baseURL` and `defaultHeaders` properties).
   * @returns The same client, now routing through the proxy under this session.
   */
  wrapOpenAI<T extends { baseURL: string; defaultHeaders?: Record<string, string | null | undefined> }>(
    openaiClient: T,
  ): T {
    return this.getScopedClient().wrapOpenAI(openaiClient);
  }

  /**
   * Execute a function within this session context.
   *
   * The callback receives this `WardenSession` instance, allowing
   * access to `wrapOpenAI()` and other session-scoped methods.
   * If the callback throws, the error propagates to the caller.
   *
   * @param fn - Async function to execute within the session context.
   * @returns The return value of the callback.
   *
   * @example
   * ```typescript
   * const result = await session.run(async (s) => {
   *   const client = s.wrapOpenAI(new OpenAI());
   *   return await client.chat.completions.create({
   *     model: 'gpt-4',
   *     messages: [{ role: 'user', content: 'Hello' }],
   *   });
   * });
   * ```
   */
  async run<T>(fn: (session: WardenSession) => Promise<T>): Promise<T> {
    return fn(this);
  }

  /**
   * Score this session with performance metrics.
   *
   * Metrics are arbitrary key-value pairs (e.g., accuracy, latency,
   * token efficiency). They are persisted on the session record and
   * can be queried via the management API.
   *
   * Uses `POST /api/sessions/:id/score` on the management API.
   *
   * @param metrics - Key-value pairs where values are numbers.
   *
   * @example
   * ```typescript
   * await session.score({
   *   accuracy: 0.95,
   *   latency_ms: 1200,
   *   tokens_used: 450,
   * });
   * ```
   */
  async score(metrics: SessionMetrics): Promise<void> {
    const mgmt = this.client.management();
    const baseUrl = (mgmt as unknown as { baseUrl: string }).baseUrl;

    const url = `${baseUrl}/api/sessions/${encodeURIComponent(this.id)}/score`;
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ score: metrics }),
    });

    if (!res.ok) {
      const body = await res.text();
      throw new Error(`Failed to score session ${this.id}: ${res.status} ${body}`);
    }
  }

  /**
   * End this session by terminating it via the management API.
   *
   * Once ended, subsequent requests using this session ID will
   * be rejected by the proxy.
   *
   * @example
   * ```typescript
   * await session.end();
   * ```
   */
  async end(): Promise<void> {
    await this.client.management().terminateSession(this.id);
  }
}
