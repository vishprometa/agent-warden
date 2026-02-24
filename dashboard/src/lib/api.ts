import type {
  Trace,
  Session,
  Agent,
  AgentStats,
  PolicyConfig,
  Approval,
  Violation,
  SystemStats,
} from './types';

class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
    ...options,
  });

  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new ApiError(body || `HTTP ${res.status}`, res.status);
  }

  return res.json();
}

// Health
export function getHealth(): Promise<{ status: string }> {
  return request('/api/health');
}

// Stats
export function getStats(): Promise<SystemStats> {
  return request('/api/stats');
}

// Sessions
export function getSessions(params?: {
  limit?: number;
  offset?: number;
  agent_id?: string;
  status?: string;
}): Promise<{ sessions: Session[]; total: number }> {
  const sp = new URLSearchParams();
  if (params?.limit) sp.set('limit', String(params.limit));
  if (params?.offset) sp.set('offset', String(params.offset));
  if (params?.agent_id) sp.set('agent_id', params.agent_id);
  if (params?.status) sp.set('status', params.status);
  const qs = sp.toString();
  return request(`/api/sessions${qs ? `?${qs}` : ''}`);
}

export function getSession(id: string): Promise<{ session: Session; traces: Trace[] }> {
  return request(`/api/sessions/${id}`);
}

export function terminateSession(id: string): Promise<{ status: string }> {
  return request(`/api/sessions/${id}`, { method: 'DELETE' });
}

// Traces
export function getTraces(params?: {
  limit?: number;
  offset?: number;
  session_id?: string;
  agent_id?: string;
  action_type?: string;
  status?: string;
}): Promise<{ traces: Trace[]; total: number }> {
  const sp = new URLSearchParams();
  if (params?.limit) sp.set('limit', String(params.limit));
  if (params?.offset) sp.set('offset', String(params.offset));
  if (params?.session_id) sp.set('session_id', params.session_id);
  if (params?.agent_id) sp.set('agent_id', params.agent_id);
  if (params?.action_type) sp.set('action_type', params.action_type);
  if (params?.status) sp.set('status', params.status);
  const qs = sp.toString();
  return request(`/api/traces${qs ? `?${qs}` : ''}`);
}

export function getTrace(id: string): Promise<Trace> {
  return request(`/api/traces/${id}`);
}

export function searchTraces(q: string, limit = 50): Promise<{ traces: Trace[] }> {
  const sp = new URLSearchParams({ q, limit: String(limit) });
  return request(`/api/traces/search?${sp}`);
}

// Agents
export function getAgents(): Promise<{ agents: Agent[] }> {
  return request('/api/agents');
}

export function getAgent(id: string): Promise<Agent> {
  return request(`/api/agents/${id}`);
}

export function getAgentStats(id: string): Promise<AgentStats> {
  return request(`/api/agents/${id}/stats`);
}

// Policies
export function getPolicies(): Promise<{ policies: PolicyConfig[] }> {
  return request('/api/policies');
}

export function reloadPolicies(): Promise<{ status: string }> {
  return request('/api/policies/reload', { method: 'POST' });
}

// Approvals
export function getApprovals(): Promise<{ approvals: Approval[] }> {
  return request('/api/approvals');
}

export function approveApproval(id: string): Promise<{ status: string }> {
  return request(`/api/approvals/${id}/approve`, { method: 'POST' });
}

export function denyApproval(id: string): Promise<{ status: string }> {
  return request(`/api/approvals/${id}/deny`, { method: 'POST' });
}

// Violations
export function getViolations(params?: {
  agent_id?: string;
  limit?: number;
}): Promise<{ violations: Violation[] }> {
  const sp = new URLSearchParams();
  if (params?.agent_id) sp.set('agent_id', params.agent_id);
  if (params?.limit) sp.set('limit', String(params.limit));
  const qs = sp.toString();
  return request(`/api/violations${qs ? `?${qs}` : ''}`);
}
