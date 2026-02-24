import { useEffect, useState, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Loader2, MonitorDot } from 'lucide-react';
import { getSessions } from '@/lib/api';
import type { Session } from '@/lib/types';
import { formatCost, formatNumber, timeAgo } from '@/lib/format';
import StatusBadge from '@/components/StatusBadge';
import DataTable, { type Column } from '@/components/DataTable';

const PAGE_SIZE = 50;

export default function Sessions() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [offset, setOffset] = useState(0);

  const statusFilter = searchParams.get('status') || '';

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getSessions({
        limit: PAGE_SIZE,
        offset,
        status: statusFilter || undefined,
      });
      setSessions(data.sessions || []);
      setTotal(data.total ?? 0);
    } catch (err) {
      console.error('Failed to load sessions:', err);
    } finally {
      setLoading(false);
    }
  }, [offset, statusFilter]);

  useEffect(() => {
    load();
  }, [load]);

  const setStatus = (status: string) => {
    setOffset(0);
    if (status) {
      setSearchParams({ status });
    } else {
      setSearchParams({});
    }
  };

  const columns: Column<Session>[] = [
    {
      key: 'id',
      header: 'Session ID',
      render: (s) => (
        <span className="font-mono text-xs text-brand-400">{s.id.slice(0, 12)}...</span>
      ),
    },
    {
      key: 'agent_id',
      header: 'Agent',
      render: (s) => <span className="font-mono text-xs">{s.agent_id}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      render: (s) => <StatusBadge status={s.status} />,
    },
    {
      key: 'action_count',
      header: 'Actions',
      sortable: true,
      render: (s) => formatNumber(s.action_count),
    },
    {
      key: 'total_cost',
      header: 'Cost',
      sortable: true,
      render: (s) => <span className="font-mono text-xs">{formatCost(s.total_cost)}</span>,
    },
    {
      key: 'started_at',
      header: 'Started',
      sortable: true,
      render: (s) => (
        <span className="text-gray-500" title={s.started_at}>
          {timeAgo(s.started_at)}
        </span>
      ),
    },
    {
      key: 'ended_at',
      header: 'Ended',
      render: (s) =>
        s.ended_at ? (
          <span className="text-gray-500" title={s.ended_at}>
            {timeAgo(s.ended_at)}
          </span>
        ) : (
          <span className="text-gray-600">&mdash;</span>
        ),
    },
  ];

  const statusOptions = ['', 'active', 'completed', 'terminated', 'paused'];

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Sessions</h1>
        <p className="text-sm text-gray-500 mt-1">
          Agent sessions with activity and cost tracking
        </p>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <select
          className="select"
          value={statusFilter}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="">All statuses</option>
          {statusOptions.filter(Boolean).map((s) => (
            <option key={s} value={s}>
              {s.charAt(0).toUpperCase() + s.slice(1)}
            </option>
          ))}
        </select>

        <span className="text-xs text-gray-500">
          {total} session{total !== 1 ? 's' : ''}
        </span>
      </div>

      {/* Table */}
      <div className="card">
        {loading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="w-5 h-5 text-gray-500 animate-spin" />
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={sessions}
            keyField="id"
            onRowClick={(s) => navigate(`/sessions/${s.id}`)}
            emptyMessage="No sessions found"
          />
        )}
      </div>

      {/* Pagination */}
      {total > PAGE_SIZE && (
        <div className="flex items-center justify-between">
          <button
            className="btn-ghost"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            Previous
          </button>
          <span className="text-xs text-gray-500">
            Showing {offset + 1}&ndash;{Math.min(offset + PAGE_SIZE, total)} of {total}
          </span>
          <button
            className="btn-ghost"
            disabled={offset + PAGE_SIZE >= total}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
