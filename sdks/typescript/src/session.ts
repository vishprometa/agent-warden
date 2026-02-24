import { AgentWarden } from './client.js';
import { ActionDenied, ActionPendingApproval } from './errors.js';
import type {
  SessionConfig,
  ActionEvent,
  Verdict,
  ToolCallOptions,
  ApiCallOptions,
  DbQueryOptions,
  ChatOptions,
  ScoreOptions,
} from './types.js';

/**
 * A governed session that evaluates every agent action against policies
 * before allowing execution. Created via `AgentWarden.session()`.
 *
 * Each action method (tool, apiCall, dbQuery, chat) follows the same pattern:
 * 1. Build an ActionEvent describing the intended action
 * 2. POST it to the server for policy evaluation
 * 3. If denied/terminate: throw ActionDenied
 * 4. If pending approval: throw ActionPendingApproval
 * 5. If allowed: execute the callback (if provided) and return the result
 */
export class Session {
  private sessionId: string | undefined;
  private readonly agentId: string;
  private readonly agentVersion: string;
  private readonly metadata: Record<string, string>;
  private readonly client: AgentWarden;

  private actionCount = 0;
  private totalCost = 0;
  private startedAt: number;

  constructor(client: AgentWarden, config: SessionConfig) {
    this.client = client;
    this.agentId = config.agentId;
    this.agentVersion = config.agentVersion || '1.0.0';
    this.metadata = config.metadata || {};
    this.startedAt = Date.now();
  }

  /** @internal - Called by AgentWarden.session() to register the session with the server. */
  async start(): Promise<void> {
    const result = await this.client._post('/api/sessions', {
      agent_id: this.agentId,
      agent_version: this.agentVersion,
      metadata: this.metadata,
    }) as { session_id: string };
    this.sessionId = result.session_id;
    this.startedAt = Date.now();
  }

  /** Returns the server-assigned session ID. */
  get id(): string | undefined {
    return this.sessionId;
  }

  // ---------------------------------------------------------------------------
  // Action methods
  // ---------------------------------------------------------------------------

  /**
   * Evaluate and optionally execute a tool call.
   *
   * @returns The verdict if no `execute` callback is provided,
   *          or the result of the callback if allowed.
   */
  async tool<T = unknown>(options: ToolCallOptions<T>): Promise<T | Verdict> {
    const verdict = await this.evaluate({
      type: 'tool.call',
      name: options.name,
      params: options.params || {},
      target: options.target || '',
    });

    if (options.execute) {
      return options.execute();
    }
    return verdict;
  }

  /**
   * Evaluate and optionally execute an API call.
   *
   * @returns The verdict if no `execute` callback is provided,
   *          or the result of the callback if allowed.
   */
  async apiCall<T = unknown>(options: ApiCallOptions<T>): Promise<T | Verdict> {
    const verdict = await this.evaluate({
      type: 'api.request',
      name: options.name,
      params: options.params || {},
      target: options.target || '',
    });

    if (options.execute) {
      return options.execute();
    }
    return verdict;
  }

  /**
   * Evaluate and optionally execute a database query.
   *
   * @returns The verdict if no `execute` callback is provided,
   *          or the result of the callback if allowed.
   */
  async dbQuery<T = unknown>(options: DbQueryOptions<T>): Promise<T | Verdict> {
    const verdict = await this.evaluate({
      type: 'db.query',
      name: options.query,
      params: { query: options.query },
      target: options.target || '',
    });

    if (options.execute) {
      return options.execute();
    }
    return verdict;
  }

  /**
   * Evaluate and optionally execute a chat/LLM call.
   *
   * @returns The verdict if no `execute` callback is provided,
   *          or the result of the callback if allowed.
   */
  async chat<T = unknown>(options: ChatOptions<T>): Promise<T | Verdict> {
    const verdict = await this.evaluate({
      type: 'llm.chat',
      name: options.model,
      params: { model: options.model, messages: options.messages },
      target: options.model,
    });

    if (options.execute) {
      return options.execute();
    }
    return verdict;
  }

  /**
   * Score this session with quality/completion metrics.
   * Sent to the server for observability and analytics.
   */
  async score(options: ScoreOptions): Promise<void> {
    if (!this.sessionId) {
      throw new Error('Session not started. Call start() first.');
    }
    await this.client._post(`/api/sessions/${this.sessionId}/score`, {
      task_completed: options.taskCompleted,
      quality: options.quality,
      metadata: options.metadata,
    });
  }

  /**
   * End this session. No further actions can be evaluated after this.
   */
  async end(): Promise<void> {
    if (!this.sessionId) {
      throw new Error('Session not started. Call start() first.');
    }
    await this.client._post(`/api/sessions/${this.sessionId}/end`, {});
  }

  // ---------------------------------------------------------------------------
  // Internal
  // ---------------------------------------------------------------------------

  /**
   * Build an ActionEvent and send it to the server for policy evaluation.
   * Throws ActionDenied or ActionPendingApproval if the action is not allowed.
   */
  private async evaluate(action: {
    type: string;
    name: string;
    params: Record<string, unknown>;
    target: string;
  }): Promise<Verdict> {
    if (!this.sessionId) {
      throw new Error('Session not started. Call start() first.');
    }

    this.actionCount++;
    const durationSeconds = (Date.now() - this.startedAt) / 1000;

    const event: ActionEvent = {
      session_id: this.sessionId,
      agent_id: this.agentId,
      agent_version: this.agentVersion,
      action: {
        type: action.type,
        name: action.name,
        params_json: JSON.stringify(action.params),
        target: action.target,
      },
      context: {
        session_cost: this.totalCost,
        session_action_count: this.actionCount,
        session_duration_seconds: durationSeconds,
      },
      metadata: this.metadata,
    };

    const verdict = await this.client._post('/api/evaluate', event) as Verdict;

    this.handleVerdict(verdict);

    return verdict;
  }

  /**
   * Inspect a verdict and throw the appropriate error if the action is blocked.
   */
  private handleVerdict(verdict: Verdict): void {
    switch (verdict.verdict) {
      case 'deny':
      case 'terminate':
        throw new ActionDenied(
          verdict.policy_name || 'unknown',
          verdict.message || `Action ${verdict.verdict} by policy`,
          verdict.suggestions || [],
        );

      case 'approve':
        if (verdict.approval_id) {
          throw new ActionPendingApproval(
            verdict.approval_id,
            verdict.policy_name || 'unknown',
            verdict.timeout_seconds || 300,
          );
        }
        break;

      case 'allow':
      case 'throttle':
        // Allowed — proceed with execution.
        break;
    }
  }
}
