import { AgentWarden } from "./client.js";
import type { AgentWardenConfig } from "./client.js";

/**
 * Default ports to scan when auto-discovering an AgentWarden instance.
 * Port 6777 is the default, followed by common alternative ports.
 */
const DISCOVERY_PORTS = [6777, 6778, 6779, 8677];

/**
 * Environment variable name for explicitly setting the AgentWarden URL.
 */
const ENV_VAR = "AGENTWARDEN_URL";

/**
 * Timeout in milliseconds for each health-check probe during discovery.
 */
const PROBE_TIMEOUT_MS = 1500;

/**
 * Probes a single URL to check if an AgentWarden instance is running.
 * Returns the URL if reachable, or `null` if not.
 */
async function probe(url: string): Promise<string | null> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);

    const res = await fetch(`${url}/api/health`, {
      method: "GET",
      signal: controller.signal,
    });

    clearTimeout(timeout);

    if (res.ok) {
      const body = await res.json() as Record<string, unknown>;
      if (body && body.status === "ok") {
        return url;
      }
    }
  } catch {
    // Connection refused, timeout, or other network error — not reachable.
  }
  return null;
}

/**
 * Discovers a running AgentWarden instance by checking well-known locations.
 *
 * Discovery order:
 * 1. `AGENTWARDEN_URL` environment variable (if set)
 * 2. `http://localhost:6777` (default port)
 * 3. Common alternative ports: 6778, 6779, 8677
 *
 * Each candidate is probed with a health-check request (`GET /api/health`).
 * The first responding instance wins.
 *
 * @returns The base URL of the discovered instance, or `null` if none found.
 *
 * @example
 * ```typescript
 * import { discover } from '@agentwarden/sdk';
 *
 * const url = await discover();
 * if (url) {
 *   console.log(`Found AgentWarden at ${url}`);
 * } else {
 *   console.log('No AgentWarden instance found');
 * }
 * ```
 */
export async function discover(): Promise<string | null> {
  // 1. Check environment variable first.
  const envUrl = typeof process !== "undefined" ? process.env[ENV_VAR] : undefined;
  if (envUrl) {
    const normalized = envUrl.replace(/\/+$/, "");
    const result = await probe(normalized);
    if (result) return result;
  }

  // 2. Probe known ports concurrently.
  const candidates = DISCOVERY_PORTS.map((port) => `http://localhost:${port}`);
  const results = await Promise.all(candidates.map(probe));

  // Return the first successful probe.
  for (const result of results) {
    if (result) return result;
  }

  return null;
}

/**
 * Creates an auto-configured `AgentWarden` client by discovering the proxy
 * automatically.
 *
 * Combines {@link discover} with the `AgentWarden` constructor. If no
 * instance is found, throws an error with actionable guidance.
 *
 * @param opts - Partial configuration. `agentId` is required. `proxyUrl`
 *   is auto-discovered if not provided.
 * @returns A fully configured `AgentWarden` client instance.
 * @throws {Error} If no AgentWarden instance can be discovered and no
 *   `proxyUrl` is provided.
 *
 * @example
 * ```typescript
 * import { autoConnect } from '@agentwarden/sdk';
 * import OpenAI from 'openai';
 *
 * const warden = await autoConnect({ agentId: 'my-agent' });
 * const client = warden.wrapOpenAI(new OpenAI());
 * ```
 */
export async function autoConnect(
  opts: Partial<AgentWardenConfig> & { agentId: string },
): Promise<AgentWarden> {
  // If the caller already provided a proxyUrl, skip discovery.
  if (opts.proxyUrl) {
    return new AgentWarden(opts as AgentWardenConfig);
  }

  const url = await discover();
  if (!url) {
    throw new Error(
      "AgentWarden auto-discovery failed: no running instance found. " +
        "Set the AGENTWARDEN_URL environment variable or pass proxyUrl explicitly.",
    );
  }

  return new AgentWarden({
    ...opts,
    proxyUrl: url,
  });
}
