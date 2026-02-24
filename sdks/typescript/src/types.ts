export interface AgentWardenConfig {
  host?: string;          // default: localhost
  port?: number;          // default: 6777
  grpcPort?: number;      // default: 6778
  useGrpc?: boolean;      // default: false
  apiKey?: string;
}

export interface SessionConfig {
  agentId: string;
  agentVersion?: string;
  metadata?: Record<string, string>;
}

export interface ActionEvent {
  session_id: string;
  agent_id: string;
  agent_version: string;
  action: {
    type: string;
    name: string;
    params_json: string;
    target: string;
  };
  context: {
    session_cost: number;
    session_action_count: number;
    session_duration_seconds: number;
  };
  metadata: Record<string, string>;
}

export interface Verdict {
  verdict: 'allow' | 'deny' | 'terminate' | 'approve' | 'throttle';
  trace_id: string;
  policy_name?: string;
  message?: string;
  approval_id?: string;
  timeout_seconds?: number;
  latency_ms: number;
  suggestions?: string[];
}

export interface ToolCallOptions<T = unknown> {
  name: string;
  params?: Record<string, unknown>;
  target?: string;
  execute?: () => T | Promise<T>;
}

export interface ApiCallOptions<T = unknown> {
  name: string;
  params?: Record<string, unknown>;
  target?: string;
  execute?: () => T | Promise<T>;
}

export interface DbQueryOptions<T = unknown> {
  query: string;
  target?: string;
  execute?: () => T | Promise<T>;
}

export interface ChatOptions<T = unknown> {
  model: string;
  messages: Array<{ role: string; content: string }>;
  execute?: () => T | Promise<T>;
}

export interface ScoreOptions {
  taskCompleted?: boolean;
  quality?: number;
  metadata?: Record<string, unknown>;
}
