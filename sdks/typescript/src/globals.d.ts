/**
 * Ambient declarations for Fetch API globals.
 *
 * These are available natively in Node 18+, Bun, Deno, and all modern browsers.
 * We declare them here instead of including the full "DOM" lib to keep the
 * SDK's type surface minimal and avoid polluting consumers with browser globals.
 */

interface RequestInit {
  method?: string;
  headers?: Record<string, string> | Headers;
  body?: string | ArrayBuffer | ReadableStream | null;
  signal?: AbortSignal;
}

interface Response {
  ok: boolean;
  status: number;
  statusText: string;
  headers: Headers;
  json(): Promise<unknown>;
  text(): Promise<string>;
}

interface Headers {
  get(name: string): string | null;
  set(name: string, value: string): void;
  has(name: string): boolean;
  delete(name: string): void;
  forEach(callback: (value: string, key: string) => void): void;
}

declare function fetch(input: string | URL, init?: RequestInit): Promise<Response>;

declare class URL {
  constructor(url: string, base?: string | URL);
  href: string;
  origin: string;
  protocol: string;
  host: string;
  hostname: string;
  port: string;
  pathname: string;
  search: string;
  hash: string;
  searchParams: URLSearchParams;
  toString(): string;
}

declare class URLSearchParams {
  constructor(init?: string | Record<string, string> | [string, string][]);
  set(name: string, value: string): void;
  get(name: string): string | null;
  has(name: string): boolean;
  delete(name: string): void;
  toString(): string;
  forEach(callback: (value: string, key: string) => void): void;
}

// ---------------------------------------------------------------------------
// AbortController — available in Node 15+ and all modern browsers
// ---------------------------------------------------------------------------

interface AbortSignal {
  readonly aborted: boolean;
}

declare class AbortController {
  readonly signal: AbortSignal;
  abort(): void;
}

// ---------------------------------------------------------------------------
// Timer functions — available globally in Node and browsers
// ---------------------------------------------------------------------------

declare function setTimeout(callback: () => void, ms: number): unknown;
declare function clearTimeout(id: unknown): void;

// ---------------------------------------------------------------------------
// process.env — Node.js only, guarded with typeof checks at runtime
// ---------------------------------------------------------------------------

declare const process: { env: Record<string, string | undefined> } | undefined;
