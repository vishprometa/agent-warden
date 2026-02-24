import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts';
import { MonitorDot, Activity, DollarSign, AlertTriangle, Loader2 } from 'lucide-react';
import { getStats, getTraces } from '@/lib/api';
import type { SystemStats, Trace } from '@/lib/types';
import { formatCost, formatNumber, timeAgo, formatLatency } from '@/lib/format';
import StatCard from '@/components/StatCard';
import StatusBadge from '@/components/StatusBadge';
import DataTable, { type Column } from '@/components/DataTable';

export default function Overview() {
  const navigate = useNavigate();
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [statsData, tracesData] = await Promise.all([
          getStats(),
          getTraces({ limit: 20 }),
        ]);
        setStats(statsData);
        setTraces(tracesData.traces || []);
      } catch (err) {
        console.error('Failed to load overview data:', err);
      } finally {
        setLoading(false);
      }
    }
    load();
    const interval = setInterval(load, 15000);
    return () => clearInterval(interval);
  }, []);

  // Build a simple cost-over-time chart from recent traces
  const costChartData = (() => {
    if (!traces.length) return [];
    const buckets = new Map<string, number>();
    for (const t of [...traces].reverse()) {
      const d = new Date(t.timestamp);
      const key = `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
      buckets.set(key, (buckets.get(key) || 0) + t.cost_usd);
    }
    return Array.from(buckets.entries()).map(([time, cost]) => ({ time, cost }));
  })();

  const traceColumns: Column<Trace>[] = [
    {
      key: 'timestamp',
      header: 'Time',
      render: (t) => (
        <span title={t.timestamp} className="text-gray-500">
          {timeAgo(t.timestamp)}
        </span>
      ),
    },
    { key: 'agent_id', header: 'Agent', render: (t) => (
      <span className="font-mono text-xs">{t.agent_id}</span>
    )},
    { key: 'action_type', header: 'Action', render: (t) => (
      <span className="font-mono text-xs text-gray-400">{t.action_type}</span>
    )},
    { key: 'action_name', header: 'Name', render: (t) => (
      <span className="text-gray-300 truncate max-w-[200px] block">{t.action_name}</span>
    )},
    {
      key: 'status',
      header: 'Status',
      render: (t) => <StatusBadge status={t.status} />,
    },
    {
      key: 'latency_ms',
      header: 'Latency',
      render: (t) => <span className="text-gray-500">{formatLatency(t.latency_ms)}</span>,
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

  return (
    <div className="space-y-8 animate-fade-in">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Overview</h1>
        <p className="text-sm text-gray-500 mt-1">
          System-wide agent activity and governance metrics
        </p>
      </div>

      {/* Stat Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          label="Active Sessions"
          value={formatNumber(stats?.active_sessions ?? 0)}
          icon={<MonitorDot size={18} />}
        />
        <StatCard
          label="Total Traces"
          value={formatNumber(stats?.total_traces ?? 0)}
          icon={<Activity size={18} />}
        />
        <StatCard
          label="Total Cost"
          value={formatCost(stats?.total_cost ?? 0)}
          icon={<DollarSign size={18} />}
        />
        <StatCard
          label="Violations"
          value={formatNumber(stats?.total_violations ?? 0)}
          icon={<AlertTriangle size={18} />}
          changeType={
            (stats?.total_violations ?? 0) > 0 ? 'negative' : 'neutral'
          }
        />
      </div>

      {/* Cost Chart */}
      {costChartData.length > 1 && (
        <div className="card p-5">
          <h2 className="text-sm font-medium text-gray-400 mb-4">
            Cost Over Recent Activity
          </h2>
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={costChartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
                <XAxis
                  dataKey="time"
                  stroke="#6b7280"
                  fontSize={11}
                  tickLine={false}
                />
                <YAxis
                  stroke="#6b7280"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={(v) => `$${v.toFixed(3)}`}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#111827',
                    border: '1px solid #374151',
                    borderRadius: '8px',
                    fontSize: '12px',
                  }}
                  formatter={(value: number) => [formatCost(value), 'Cost']}
                />
                <Line
                  type="monotone"
                  dataKey="cost"
                  stroke="#0c8ee9"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, fill: '#0c8ee9' }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Recent Traces */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800">
          <h2 className="text-sm font-medium text-gray-400">Recent Traces</h2>
        </div>
        <DataTable
          columns={traceColumns}
          data={traces}
          keyField="id"
          onRowClick={(t) => navigate(`/sessions/${t.session_id}`)}
          emptyMessage="No traces recorded yet"
          compact
        />
      </div>
    </div>
  );
}
