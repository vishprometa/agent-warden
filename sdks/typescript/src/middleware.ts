import {
  HEADER_AGENT_ID,
  HEADER_SESSION_ID,
  HEADER_METADATA,
  HEADER_TRACE_ID,
} from "./client.js";
import type {
  MiddlewareOptions,
  MiddlewareRequest,
  MiddlewareResponse,
  MiddlewareNext,
} from "./types.js";

const DEFAULT_PROXY_URL = "http://localhost:6777";

/**
 * Generates a lightweight unique trace ID using timestamp + random hex.
 * Not cryptographically secure, but sufficient for trace correlation.
 */
function generateTraceId(): string {
  const timestamp = Date.now().toString(36);
  const random = Math.random().toString(36).substring(2, 10);
  return `${timestamp}-${random}`;
}

/**
 * Express/Hono/Connect middleware that automatically traces API requests
 * through AgentWarden.
 *
 * When mounted, the middleware:
 * 1. Generates a unique trace ID for each request
 * 2. Injects AgentWarden headers into the response for correlation
 * 3. Records the request as a trace on the AgentWarden proxy (fire-and-forget)
 * 4. Passes control to the next middleware immediately (non-blocking)
 *
 * The middleware is designed to be transparent: it never blocks or modifies
 * the request/response flow, and trace recording failures are silently ignored.
 *
 * @param opts - Middleware configuration options.
 * @returns An Express/Connect-compatible request handler.
 *
 * @example Express
 * ```typescript
 * import express from 'express';
 * import { wardenMiddleware } from '@agentwarden/sdk';
 *
 * const app = express();
 * app.use(wardenMiddleware({
 *   agentId: 'my-api-server',
 *   sessionFrom: (req) => req.headers['x-session-id'] as string,
 *   exclude: ['/health', '/metrics'],
 * }));
 * ```
 *
 * @example Hono
 * ```typescript
 * import { Hono } from 'hono';
 * import { wardenMiddleware } from '@agentwarden/sdk';
 *
 * const app = new Hono();
 * app.use('*', wardenMiddleware({ agentId: 'my-api' }));
 * ```
 */
export function wardenMiddleware(
  opts: MiddlewareOptions,
): (req: MiddlewareRequest, res: MiddlewareResponse, next: MiddlewareNext) => void {
  const proxyUrl = (opts.proxyUrl ?? DEFAULT_PROXY_URL).replace(/\/+$/, "");
  const traceEndpoint = `${proxyUrl}/api/traces`;
  const excludePaths = opts.exclude ?? [];

  return function agentWardenMiddleware(
    req: MiddlewareRequest,
    res: MiddlewareResponse,
    next: MiddlewareNext,
  ): void {
    // Skip excluded paths.
    if (excludePaths.some((prefix) => req.path.startsWith(prefix))) {
      next();
      return;
    }

    // Generate trace ID and inject into response headers for correlation.
    const traceId = generateTraceId();

    // Extract session ID and metadata from the request if extractors are provided.
    const sessionId = opts.sessionFrom?.(req);
    const metadata = opts.metadataFrom?.(req);

    // Set response headers for downstream correlation.
    res.setHeader(HEADER_TRACE_ID, traceId);
    res.setHeader(HEADER_AGENT_ID, opts.agentId);
    if (sessionId) {
      res.setHeader(HEADER_SESSION_ID, sessionId);
    }

    // Record the start time for latency measurement.
    const startTime = Date.now();

    // Listen for response finish to record the trace.
    res.on("finish", () => {
      const latencyMs = Date.now() - startTime;

      // Build trace headers for the recording request.
      const traceHeaders: Record<string, string> = {
        "Content-Type": "application/json",
        [HEADER_AGENT_ID]: opts.agentId,
        [HEADER_TRACE_ID]: traceId,
      };

      if (sessionId) {
        traceHeaders[HEADER_SESSION_ID] = sessionId;
      }

      if (metadata && Object.keys(metadata).length > 0) {
        traceHeaders[HEADER_METADATA] = JSON.stringify(metadata);
      }

      // Fire-and-forget: record the trace asynchronously.
      // Failures are silently ignored to avoid impacting the request flow.
      fetch(traceEndpoint, {
        method: "POST",
        headers: traceHeaders,
        body: JSON.stringify({
          action_type: "api.request",
          action_name: `${req.method} ${req.path}`,
          latency_ms: latencyMs,
          metadata: {
            method: req.method,
            path: req.path,
            ...(metadata ?? {}),
          },
        }),
      }).catch(() => {
        // Intentionally swallowed — tracing must never break the app.
      });
    });

    // Pass control immediately — the middleware is fully non-blocking.
    next();
  };
}
