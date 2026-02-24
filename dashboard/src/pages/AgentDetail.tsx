import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Loader2 } from 'lucide-react';
import { getAgent, getAgentStats, getTraces, getSessions } from '@/lib/api';
import type { Agent, AgentStats, Trace, Session } from '@/lib/types';
import { formatCost, formatNumber, timeAgo, formatDateTime } from '@/lib/format';
import StatCard from '@/components/StatCard';
import StatusBadge from '@/components/StatusBadge';
import DataTable, { type Column } from '@/components/DataTable';

export default function AgentDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [agent, setAgent] = useState<Agent | null>(null);
  const [stats, setStats] = useState<AgentStats | null>(null);
  const [recentTraces, setRecentTraces] = useState<Trace[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    Promise.all([
      getAgent(id),
      getAgentStats(id),
      getTraces({ agent_id: id, limit: 15 }),
      getSessions({ agent_id: id, limit: 10 }),
    ])
      .then(([agentData, statsData, tracesData, sessionsData]) => {
        setAgent(agentData);
        setStats(statsData);
        setRecentTraces(tracesData.traces || []);
        setSessions(sessionsData.sessions || []);
      })
      .catch((err) => console.error('Failed to load agent:', err))
      .finally(() => setLoading(false));
  }, [id]);

  const sessionColumns: Column<Session>[] = [
    {
      key: 'id',
      header: 'Session',
      render: (s) => (
        <span className="font-mono text-xs text-brand-400">{s.id.slice(0, 12)}...</span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (s) => <StatusBadge status={s.status} />,
    },
    {
      key: 'action_count',
      header: 'Actions',
      render: (s) => formatNumber(s.action_count),
    },
    {
      key: 'total_cost',
      header: 'Cost',
      render: (s) => <span className="font-mono text-xs">{formatCost(s.total_cost)}</span>,
    },
    {
      key: 'started_at',
      header: 'Started',
      render: (s) => (
        <span className="text-gray-500" title={s.started_at}>
          {timeAgo(s.started_at)}
        </span>
      ),
    },
  ];

  const traceColumns: Column<Trace>[] = [
    {
      key: 'timestamp',
      header: 'Time',
      render: (t) => (
        <span className="text-gray-500 text-xs" title={t.timestamp}>
          {timeAgo(t.timestamp)}
        </span>
      ),
    },
    {
      key: 'action_type',
      header: 'Action',
      render: (t) => (
        <span className="font-mono text-xs text-gray-400">{t.action_type}</span>
      ),
    },
    {
      key: 'action_name',
      header: 'Name',
      render: (t) => (
        <span className="text-gray-300 text-sm truncate max-w-[200px] block">
          {t.action_name}
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (t) => <StatusBadge status={t.status} />,
    },
    {
      key: 'cost_usd',
      header: 'Cost',
      render: (t) => <span className="font-mono text-xs">{formatCost(t.cost_usd)}</span>,
    },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 text-gray-500 animate-spin" />
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="text-center py-20">
        <p className="text-gray-500">Agent not found</p>
        <button className="btn-ghost mt-4" onClick={() => navigate('/agents')}>
          Back to Agents
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      <button
        onClick={() => navigate('/agents')}
        className="btn-ghost flex items-center gap-1.5 -ml-3"
      >
        <ArrowLeft size={16} />
        Agents
      </button>

      <div>
        <h1 className="text-2xl font-bold text-gray-100">{agent.name}</h1>
        <p className="text-sm text-gray-500 mt-1">
          ID: <span className="font-mono text-gray-400">{agent.id}</span>
          {' \u00b7 '}
          Created: <span className="text-gray-400">{formatDateTime(agent.created_at)}</span>
        </p>
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <StatCard label="Sessions" value={formatNumber(stats.total_sessions)} />
          <StatCard label="Traces" value={formatNumber(stats.total_traces)} />
          <StatCard label="Total Cost" value={formatCost(stats.total_cost)} />
          <StatCard
            label="Avg Cost/Session"
            value={formatCost(stats.avg_cost_per_session)}
          />
        </div>
      )}

      {/* Sessions */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800">
          <h2 className="text-sm font-medium text-gray-400">Recent Sessions</h2>
        </div>
        <DataTable
          columns={sessionColumns}
          data={sessions}
          keyField="id"
          onRowClick={(s) => navigate(`/sessions/${s.id}`)}
          emptyMessage="No sessions for this agent"
          compact
        />
      </div>

      {/* Recent Traces */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800">
          <h2 className="text-sm font-medium text-gray-400">Recent Traces</h2>
        </div>
        <DataTable
          columns={traceColumns}
          data={recentTraces}
          keyField="id"
          emptyMessage="No traces for this agent"
          compact
        />
      </div>
    </div>
  );
}
