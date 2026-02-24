import { useEffect, useState } from 'react';
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
  Legend,
} from 'recharts';
import { Loader2 } from 'lucide-react';
import { getTraces, getSessions, getStats } from '@/lib/api';
import type { Trace, Session, SystemStats } from '@/lib/types';
import { formatCost, formatNumber } from '@/lib/format';
import StatCard from '@/components/StatCard';

const CHART_COLORS = [
  '#0c8ee9',
  '#8b5cf6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#ec4899',
  '#06b6d4',
  '#84cc16',
];

const tooltipStyle = {
  backgroundColor: '#111827',
  border: '1px solid #374151',
  borderRadius: '8px',
  fontSize: '12px',
};

export default function Costs() {
  const [traces, setTraces] = useState<Trace[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      getTraces({ limit: 100 }),
      getSessions({ limit: 50 }),
      getStats(),
    ])
      .then(([tracesData, sessionsData, statsData]) => {
        setTraces(tracesData.traces || []);
        setSessions(sessionsData.sessions || []);
        setStats(statsData);
      })
      .catch((err) => console.error('Failed to load cost data:', err))
      .finally(() => setLoading(false));
  }, []);

  // Cost over time (bucketed by hour)
  const costOverTime = (() => {
    const buckets = new Map<string, number>();
    for (const t of [...traces].reverse()) {
      const d = new Date(t.timestamp);
      const key = `${(d.getMonth() + 1).toString().padStart(2, '0')}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:00`;
      buckets.set(key, (buckets.get(key) || 0) + t.cost_usd);
    }
    return Array.from(buckets.entries()).map(([time, cost]) => ({ time, cost }));
  })();

  // Cost by model (pie)
  const costByModel = (() => {
    const map = new Map<string, number>();
    for (const t of traces) {
      const model = t.model || 'unknown';
      map.set(model, (map.get(model) || 0) + t.cost_usd);
    }
    return Array.from(map.entries())
      .map(([name, value]) => ({ name, value }))
      .sort((a, b) => b.value - a.value);
  })();

  // Cost by agent (bar)
  const costByAgent = (() => {
    const map = new Map<string, number>();
    for (const t of traces) {
      map.set(t.agent_id, (map.get(t.agent_id) || 0) + t.cost_usd);
    }
    return Array.from(map.entries())
      .map(([agent, cost]) => ({ agent, cost }))
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 10);
  })();

  // Top sessions by cost
  const topSessions = [...sessions]
    .sort((a, b) => b.total_cost - a.total_cost)
    .slice(0, 10);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 text-gray-500 animate-spin" />
      </div>
    );
  }

  const totalCost = stats?.total_cost ?? traces.reduce((s, t) => s + t.cost_usd, 0);
  const avgCostPerTrace = traces.length > 0 ? totalCost / traces.length : 0;
  const totalTokens = traces.reduce((s, t) => s + t.tokens_in + t.tokens_out, 0);

  return (
    <div className="space-y-8 animate-fade-in">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Cost Analytics</h1>
        <p className="text-sm text-gray-500 mt-1">
          Token usage and cost breakdown across agents and models
        </p>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard label="Total Cost" value={formatCost(totalCost)} />
        <StatCard label="Avg Cost/Trace" value={formatCost(avgCostPerTrace)} />
        <StatCard label="Total Tokens" value={formatNumber(totalTokens)} />
      </div>

      {/* Cost Over Time */}
      {costOverTime.length > 1 && (
        <div className="card p-5">
          <h2 className="text-sm font-medium text-gray-400 mb-4">
            Cost Over Time
          </h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={costOverTime}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
                <XAxis dataKey="time" stroke="#6b7280" fontSize={11} tickLine={false} />
                <YAxis
                  stroke="#6b7280"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={(v) => `$${v.toFixed(3)}`}
                />
                <Tooltip
                  contentStyle={tooltipStyle}
                  formatter={(value: number) => [formatCost(value), 'Cost']}
                />
                <Line
                  type="monotone"
                  dataKey="cost"
                  stroke="#0c8ee9"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Two column: Pie + Bar */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Cost by Model */}
        {costByModel.length > 0 && (
          <div className="card p-5">
            <h2 className="text-sm font-medium text-gray-400 mb-4">
              Cost by Model
            </h2>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={costByModel}
                    dataKey="value"
                    nameKey="name"
                    cx="50%"
                    cy="50%"
                    innerRadius={50}
                    outerRadius={90}
                    paddingAngle={2}
                    strokeWidth={0}
                  >
                    {costByModel.map((_, i) => (
                      <Cell
                        key={i}
                        fill={CHART_COLORS[i % CHART_COLORS.length]}
                      />
                    ))}
                  </Pie>
                  <Tooltip
                    contentStyle={tooltipStyle}
                    formatter={(value: number) => formatCost(value)}
                  />
                  <Legend
                    wrapperStyle={{ fontSize: '11px', color: '#9ca3af' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </div>
        )}

        {/* Cost by Agent */}
        {costByAgent.length > 0 && (
          <div className="card p-5">
            <h2 className="text-sm font-medium text-gray-400 mb-4">
              Cost by Agent
            </h2>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={costByAgent} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
                  <XAxis
                    type="number"
                    stroke="#6b7280"
                    fontSize={11}
                    tickLine={false}
                    tickFormatter={(v) => `$${v.toFixed(3)}`}
                  />
                  <YAxis
                    type="category"
                    dataKey="agent"
                    stroke="#6b7280"
                    fontSize={11}
                    tickLine={false}
                    width={120}
                  />
                  <Tooltip
                    contentStyle={tooltipStyle}
                    formatter={(value: number) => [formatCost(value), 'Cost']}
                  />
                  <Bar dataKey="cost" fill="#8b5cf6" radius={[0, 4, 4, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        )}
      </div>

      {/* Top Sessions by Cost */}
      {topSessions.length > 0 && (
        <div className="card">
          <div className="px-5 py-4 border-b border-gray-800">
            <h2 className="text-sm font-medium text-gray-400">
              Top Sessions by Cost
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Session
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Agent
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Actions
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Status
                  </th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">
                    Cost
                  </th>
                </tr>
              </thead>
              <tbody>
                {topSessions.map((s) => (
                  <tr
                    key={s.id}
                    className="border-b border-gray-800/50 hover:bg-gray-800/30"
                  >
                    <td className="px-4 py-2.5 font-mono text-xs text-brand-400">
                      {s.id.slice(0, 12)}...
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-gray-400">
                      {s.agent_id}
                    </td>
                    <td className="px-4 py-2.5 text-gray-400">
                      {formatNumber(s.action_count)}
                    </td>
                    <td className="px-4 py-2.5">
                      <span
                        className={`text-xs ${
                          s.status === 'active'
                            ? 'text-emerald-400'
                            : 'text-gray-500'
                        }`}
                      >
                        {s.status}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs text-gray-200">
                      {formatCost(s.total_cost)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
