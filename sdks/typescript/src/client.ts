import { AgentWardenConfig, SessionConfig } from './types.js';
import { Session } from './session.js';

export class AgentWarden {
  private host: string;
  private port: number;
  private apiKey?: string;
  private baseUrl: string;

  constructor(config: AgentWardenConfig = {}) {
    this.host = config.host || process.env.AGENTWARDEN_HOST || 'localhost';
    this.port = config.port || parseInt(process.env.AGENTWARDEN_PORT || '6777');
    this.apiKey = config.apiKey || process.env.AGENTWARDEN_API_KEY;
    this.baseUrl = `http://${this.host}:${this.port}`;
  }

  async session(config: SessionConfig): Promise<Session> {
    const session = new Session(this, config);
    await session.start();
    return session;
  }

  /** @internal */
  async _post(path: string, data: unknown): Promise<unknown> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    const resp = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(data),
    });
    if (!resp.ok) {
      throw new Error(`AgentWarden request failed: ${resp.status} ${resp.statusText}`);
    }
    return resp.json();
  }
}
