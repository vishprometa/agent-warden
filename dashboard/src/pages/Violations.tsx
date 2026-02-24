import { useEffect, useState, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Loader2, AlertTriangle } from 'lucide-react';
import { getViolations, getAgents } from '@/lib/api';
import type { Violation, Agent } from '@/lib/types';
import { timeAgo } from '@/lib/format';
import StatusBadge from '@/components/StatusBadge';

function severityColor(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'text-red-400 bg-red-500/10 border-red-500/20';
    case 'high':
      return 'text-orange-400 bg-orange-500/10 border-orange-500/20';
    case 'medium':
      return 'text-yellow-400 bg-yellow-500/10 border-yellow-500/20';
    case 'low':
      return 'text-gray-400 bg-gray-500/10 border-gray-500/20';
    default:
      return 'text-gray-400 bg-gray-500/10 border-gray-500/20';
  }
}

export default function Violations() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [violations, setViolations] = useState<Violation[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  const agentFilter = searchParams.get('agent_id') || '';

  const load = useCallback(async () => {
    try {
      const [vData, aData] = await Promise.all([
        getViolations({
          agent_id: agentFilter || undefined,
          limit: 100,
        }),
        getAgents(),
      ]);
      setViolations(vData.violations || []);
      setAgents(aData.agents || []);
    } catch (err) {
      console.error('Failed to load violations:', err);
    } finally {
      setLoading(false);
    }
  }, [agentFilter]);

  useEffect(() => {
    load();
  }, [load]);

  const setAgent = (agentId: string) => {
    if (agentId) {
      setSearchParams({ agent_id: agentId });
    } else {
      setSearchParams({});
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 text-gray-500 animate-spin" />
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Violations</h1>
        <p className="text-sm text-gray-500 mt-1">
          Policy violations and denied actions across all agents
        </p>
      </div>

      {/* Filter */}
      <div className="flex items-center gap-3">
        <select
          className="select"
          value={agentFilter}
          onChange={(e) => setAgent(e.target.value)}
        >
          <option value="">All agents</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name} ({a.id})
            </option>
          ))}
        </select>

        <span className="text-xs text-gray-500">
          {violations.length} violation{violations.length !== 1 ? 's' : ''}
        </span>
      </div>

      {violations.length === 0 ? (
        <div className="card p-12 flex flex-col items-center text-gray-500">
          <AlertTriangle size={32} className="mb-3 opacity-30" />
          <p className="text-sm">No violations recorded</p>
          <p className="text-xs mt-1">
            Violations appear when agent actions are denied or terminated by policies
          </p>
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Severity
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Agent
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Policy
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Action
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Message
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Time
                </th>
              </tr>
            </thead>
            <tbody>
              {violations.map((v) => (
                <tr
                  key={v.id}
                  className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors"
                >
                  <td className="px-4 py-3">
                    <span
                      className={`text-xs font-medium px-2 py-0.5 rounded-full border ${severityColor(v.severity)}`}
                    >
                      {v.severity}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-gray-400">
                    {v.agent_id}
                  </td>
                  <td className="px-4 py-3 text-gray-300 text-sm">
                    {v.policy_name}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-gray-400">
                    {v.action_type}
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-sm max-w-xs truncate">
                    {v.message}
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs" title={v.created_at}>
                    {timeAgo(v.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
