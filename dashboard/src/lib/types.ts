export interface Trace {
  id: string;
  session_id: string;
  agent_id: string;
  timestamp: string;
  action_type: string;
  action_name: string;
  status: string;
  policy_name?: string;
  policy_reason?: string;
  model?: string;
  tokens_in: number;
  tokens_out: number;
  cost_usd: number;
  latency_ms: number;
  hash: string;
  prev_hash: string;
}

export interface Session {
  id: string;
  agent_id: string;
  started_at: string;
  ended_at?: string;
  status: string;
  total_cost: number;
  action_count: number;
}

export interface Agent {
  id: string;
  name: string;
  created_at: string;
}

export interface AgentStats {
  total_sessions: number;
  total_traces: number;
  total_cost: number;
  avg_cost_per_session: number;
}

export interface PolicyConfig {
  name: string;
  type?: string;
  condition: string;
  effect: string;
  message: string;
  approvers?: string[];
}

export interface Approval {
  id: string;
  session_id: string;
  trace_id: string;
  policy_name: string;
  status: string;
  action_summary: any;
  created_at: string;
  timeout_at: string;
}

export interface Violation {
  id: string;
  trace_id: string;
  session_id: string;
  agent_id: string;
  policy_name: string;
  action_type: string;
  severity: string;
  message: string;
  created_at: string;
}

export interface SystemStats {
  total_traces: number;
  total_sessions: number;
  active_sessions: number;
  total_agents: number;
  total_violations: number;
  total_cost: number;
  traces_per_minute?: number;
}

export interface AgentVersion {
  id: string;
  agent_id: string;
  version: number;
  status: 'active' | 'candidate' | 'shadow' | 'retired' | 'rolled_back';
  config: Record<string, any>;
  diff_from_prev?: Record<string, any>;
  shadow_test_results?: {
    total: number;
    passed: number;
    failed: number;
    details?: Array<{ test: string; result: string; message?: string }>;
  };
  created_at: string;
  promoted_at?: string;
  rolled_back_at?: string;
  reason?: string;
}

export interface PolicyDryRunResult {
  policy_name: string;
  effect: string;
  matched: boolean;
  message: string;
}
