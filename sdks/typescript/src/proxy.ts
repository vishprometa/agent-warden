import { ManagementClient } from "./management.js";
import {
  HEADER_AGENT_ID,
  HEADER_SESSION_ID,
  HEADER_METADATA,
} from "./client.js";
import type {
  WardenProxyOptions,
  WardenProxy,
  WardenProxyConfig,
} from "./types.js";

const DEFAULT_PROXY_URL = "http://localhost:6777";

/**
 * Builds the header map for a given set of proxy options.
 */
function buildHeaders(opts: WardenProxyOptions): Record<string, string> {
  const headers: Record<string, string> = {
    [HEADER_AGENT_ID]: opts.agentId,
  };

  if (opts.sessionId) {
    headers[HEADER_SESSION_ID] = opts.sessionId;
  }

  if (opts.metadata && Object.keys(opts.metadata).length > 0) {
    headers[HEADER_METADATA] = JSON.stringify(opts.metadata);
  }

  return headers;
}

/**
 * Creates a transparent proxy configuration that can be applied to any
 * OpenAI-compatible client with zero code changes.
 *
 * The returned object contains a `config` property with `baseURL` and
 * `defaultHeaders` that can be spread directly into client constructors.
 *
 * @param opts - Proxy configuration options.
 * @returns A `WardenProxy` object with config, session scoping, and management access.
 *
 * @example
 * ```typescript
 * import { createWardenProxy } from '@agentwarden/sdk';
 * import OpenAI from 'openai';
 *
 * const proxy = createWardenProxy({ agentId: 'my-agent' });
 * const openai = new OpenAI({ ...proxy.config });
 *
 * // All requests now flow through the AgentWarden proxy
 * const response = await openai.chat.completions.create({
 *   model: 'gpt-4',
 *   messages: [{ role: 'user', content: 'Hello' }],
 * });
 * ```
 *
 * @example Session scoping
 * ```typescript
 * const proxy = createWardenProxy({ agentId: 'my-agent' });
 * const scoped = proxy.session('user-session-123');
 * const openai = new OpenAI({ ...scoped.config });
 * ```
 */
export function createWardenProxy(opts: WardenProxyOptions): WardenProxy {
  const proxyUrl = (opts.proxyUrl ?? DEFAULT_PROXY_URL).replace(/\/+$/, "");
  const headers = buildHeaders(opts);

  const config: WardenProxyConfig = {
    baseURL: `${proxyUrl}/v1`,
    defaultHeaders: headers,
  };

  let _management: ManagementClient | undefined;

  return {
    config,

    /**
     * Returns a new `WardenProxy` scoped to a specific session.
     * All other configuration (proxyUrl, agentId, metadata) is inherited.
     *
     * @param id - Session ID to scope to. If omitted, the proxy auto-creates
     *   a new session on the first request.
     */
    session(id?: string): WardenProxy {
      return createWardenProxy({
        agentId: opts.agentId,
        proxyUrl: proxyUrl,
        sessionId: id,
        metadata: opts.metadata,
      });
    },

    /**
     * Returns a lazily-initialized `ManagementClient` for the same proxy instance.
     * Use this to query sessions, traces, policies, and approvals.
     */
    management(): ManagementClient {
      if (!_management) {
        _management = new ManagementClient(proxyUrl);
      }
      return _management;
    },
  };
}
