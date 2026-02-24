import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Loader2, StopCircle } from 'lucide-react';
import { getSession, terminateSession } from '@/lib/api';
import type { Session, Trace } from '@/lib/types';
import { formatCost, formatNumber, timeAgo, formatLatency, formatDateTime } from '@/lib/format';
import StatusBadge from '@/components/StatusBadge';
import DataTable, { type Column } from '@/components/DataTable';

export default function SessionDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [session, setSession] = useState<Session | null>(null);
  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);
  const [terminating, setTerminating] = useState(false);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    getSession(id)
      .then((data) => {
        setSession(data.session);
        setTraces(data.traces || []);
      })
      .catch((err) => console.error('Failed to load session:', err))
      .finally(() => setLoading(false));
  }, [id]);

  const handleTerminate = async () => {
    if (!id || !confirm('Terminate this session? This action cannot be undone.')) return;
    setTerminating(true);
    try {
      await terminateSession(id);
      // Reload
      const data = await getSession(id);
      setSession(data.session);
      setTraces(data.traces || []);
    } catch (err) {
      console.error('Failed to terminate session:', err);
    } finally {
      setTerminating(false);
    }
  };

  const traceColumns: Column<Trace>[] = [
    {
      key: 'timestamp',
      header: 'Time',
      render: (t) => (
        <span className="text-gray-500 text-xs" title={t.timestamp}>
          {formatDateTime(t.timestamp)}
        </span>
      ),
    },
    {
      key: 'action_type',
      header: 'Type',
      render: (t) => (
        <span className="font-mono text-xs text-gray-400">{t.action_type}</span>
      ),
    },
    {
      key: 'action_name',
      header: 'Name',
      render: (t) => (
        <span className="text-gray-300 text-sm truncate max-w-[250px] block">
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
      key: 'model',
      header: 'Model',
      render: (t) => (
        <span className="text-gray-500 text-xs">{t.model || '\u2014'}</span>
      ),
    },
    {
      key: 'tokens',
      header: 'Tokens',
      render: (t) => (
        <span className="text-gray-500 text-xs">
          {t.tokens_in + t.tokens_out > 0
            ? `${formatNumber(t.tokens_in)} / ${formatNumber(t.tokens_out)}`
            : '\u2014'}
        </span>
      ),
    },
    {
      key: 'latency_ms',
      header: 'Latency',
      sortable: true,
      render: (t) => (
        <span className="text-gray-500 text-xs">{formatLatency(t.latency_ms)}</span>
      ),
    },
    {
      key: 'cost_usd',
      header: 'Cost',
      sortable: true,
      render: (t) => (
        <span className="font-mono text-xs">{formatCost(t.cost_usd)}</span>
      ),
    },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 text-gray-500 animate-spin" />
      </div>
    );
  }

  if (!session) {
    return (
      <div className="text-center py-20">
        <p className="text-gray-500">Session not found</p>
        <button className="btn-ghost mt-4" onClick={() => navigate('/sessions')}>
          Back to Sessions
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Back */}
      <button
        onClick={() => navigate('/sessions')}
        className="btn-ghost flex items-center gap-1.5 -ml-3"
      >
        <ArrowLeft size={16} />
        Sessions
      </button>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <h1 className="text-2xl font-bold text-gray-100 font-mono text-lg">
              {session.id}
            </h1>
            <StatusBadge status={session.status} size="md" />
          </div>
          <p className="text-sm text-gray-500">
            Agent: <span className="text-gray-400">{session.agent_id}</span>
            {' \u00b7 '}
            Started: <span className="text-gray-400">{formatDateTime(session.started_at)}</span>
            {session.ended_at && (
              <>
                {' \u00b7 '}
                Ended: <span className="text-gray-400">{formatDateTime(session.ended_at)}</span>
              </>
            )}
          </p>
        </div>

        {session.status === 'active' && (
          <button
            className="btn-danger flex items-center gap-2"
            onClick={handleTerminate}
            disabled={terminating}
          >
            <StopCircle size={16} />
            {terminating ? 'Terminating...' : 'Terminate'}
          </button>
        )}
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-3 gap-4">
        <div className="card p-4">
          <span className="text-xs text-gray-500 block mb-1">Actions</span>
          <span className="text-xl font-semibold">{formatNumber(session.action_count)}</span>
        </div>
        <div className="card p-4">
          <span className="text-xs text-gray-500 block mb-1">Total Cost</span>
          <span className="text-xl font-semibold font-mono">{formatCost(session.total_cost)}</span>
        </div>
        <div className="card p-4">
          <span className="text-xs text-gray-500 block mb-1">Duration</span>
          <span className="text-xl font-semibold">
            {session.ended_at
              ? timeAgo(session.started_at).replace(' ago', '')
              : 'Ongoing'}
          </span>
        </div>
      </div>

      {/* Traces Timeline */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800 flex items-center justify-between">
          <h2 className="text-sm font-medium text-gray-400">
            Trace Timeline ({traces.length})
          </h2>
        </div>
        <DataTable
          columns={traceColumns}
          data={traces}
          keyField="id"
          emptyMessage="No traces in this session"
          compact
        />
      </div>
    </div>
  );
}
