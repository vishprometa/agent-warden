import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  Loader2,
  Clock,
  ChevronDown,
  ChevronRight,
  ShieldCheck,
  ShieldX,
  ShieldAlert,
  Link2,
} from 'lucide-react';
import { getSession } from '@/lib/api';
import type { Session, Trace } from '@/lib/types';
import { formatCost, formatNumber, formatLatency, formatDateTime } from '@/lib/format';
import StatCard from '@/components/StatCard';

function statusColor(status: string): string {
  switch (status) {
    case 'allowed':
    case 'approved':
      return 'border-emerald-500/40 bg-emerald-500/5';
    case 'denied':
    case 'terminated':
      return 'border-red-500/40 bg-red-500/5';
    case 'throttled':
    case 'pending':
      return 'border-yellow-500/40 bg-yellow-500/5';
    default:
      return 'border-gray-700 bg-gray-800/30';
  }
}

function statusDotColor(status: string): string {
  switch (status) {
    case 'allowed':
    case 'approved':
      return 'bg-emerald-400';
    case 'denied':
    case 'terminated':
      return 'bg-red-400';
    case 'throttled':
    case 'pending':
      return 'bg-yellow-400';
    default:
      return 'bg-gray-500';
  }
}

function statusIcon(status: string) {
  switch (status) {
    case 'allowed':
    case 'approved':
      return <ShieldCheck size={14} className="text-emerald-400" />;
    case 'denied':
    case 'terminated':
      return <ShieldX size={14} className="text-red-400" />;
    case 'throttled':
    case 'pending':
      return <ShieldAlert size={14} className="text-yellow-400" />;
    default:
      return <Clock size={14} className="text-gray-500" />;
  }
}

export default function SessionReplay() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [session, setSession] = useState<Session | null>(null);
  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);

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

  // Compute summary stats
  const totalCost = traces.reduce((sum, t) => sum + t.cost_usd, 0);
  const totalActions = traces.length;
  const avgLatency =
    totalActions > 0
      ? Math.round(traces.reduce((sum, t) => sum + t.latency_ms, 0) / totalActions)
      : 0;

  let durationStr = '--';
  if (traces.length >= 2) {
    const first = new Date(traces[0].timestamp).getTime();
    const last = new Date(traces[traces.length - 1].timestamp).getTime();
    const diffMs = Math.abs(last - first);
    if (diffMs < 1000) durationStr = `${diffMs}ms`;
    else if (diffMs < 60000) durationStr = `${(diffMs / 1000).toFixed(1)}s`;
    else durationStr = `${(diffMs / 60000).toFixed(1)}m`;
  } else if (session) {
    if (session.ended_at) {
      const diffMs =
        new Date(session.ended_at).getTime() - new Date(session.started_at).getTime();
      if (diffMs < 1000) durationStr = `${diffMs}ms`;
      else if (diffMs < 60000) durationStr = `${(diffMs / 1000).toFixed(1)}s`;
      else durationStr = `${(diffMs / 60000).toFixed(1)}m`;
    } else {
      durationStr = 'Ongoing';
    }
  }

  // Check hash chain integrity
  const hashChainValid = traces.every((trace, idx) => {
    if (idx === 0) return true;
    return trace.prev_hash === traces[idx - 1].hash;
  });

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
        onClick={() => navigate(`/sessions/${id}`)}
        className="btn-ghost flex items-center gap-1.5 -ml-3"
      >
        <ArrowLeft size={16} />
        Session Detail
      </button>

      {/* Header */}
      <div>
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-2xl font-bold text-gray-100">Session Replay</h1>
          <div
            className={`flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full border ${
              hashChainValid
                ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
                : 'text-red-400 bg-red-500/10 border-red-500/20'
            }`}
          >
            <Link2 size={12} />
            {hashChainValid ? 'Chain valid' : 'Chain broken'}
          </div>
        </div>
        <p className="text-sm text-gray-500">
          Session:{' '}
          <span className="font-mono text-gray-400">{session.id}</span>
          {' \u00b7 '}
          Agent: <span className="text-gray-400">{session.agent_id}</span>
        </p>
      </div>

      {/* Summary Stats */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard label="Total Cost" value={formatCost(totalCost)} />
        <StatCard label="Total Actions" value={formatNumber(totalActions)} />
        <StatCard label="Duration" value={durationStr} />
        <StatCard label="Avg Latency" value={formatLatency(avgLatency)} />
      </div>

      {/* Timeline */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800">
          <h2 className="text-sm font-medium text-gray-400">
            Action Timeline ({traces.length})
          </h2>
        </div>

        {traces.length === 0 ? (
          <div className="px-5 py-12 text-center text-gray-500 text-sm">
            No traces in this session
          </div>
        ) : (
          <div className="divide-y divide-gray-800/50">
            {traces.map((trace, idx) => {
              const isExpanded = expandedId === trace.id;
              return (
                <div key={trace.id}>
                  {/* Timeline row */}
                  <button
                    className={`w-full px-5 py-3 flex items-center gap-4 text-left hover:bg-gray-800/30 transition-colors ${statusColor(trace.status)}`}
                    onClick={() => setExpandedId(isExpanded ? null : trace.id)}
                  >
                    {/* Timeline dot */}
                    <div className="flex flex-col items-center shrink-0">
                      <div
                        className={`w-2.5 h-2.5 rounded-full ${statusDotColor(trace.status)}`}
                      />
                      {idx < traces.length - 1 && (
                        <div className="w-px h-4 bg-gray-700 mt-1" />
                      )}
                    </div>

                    {/* Content */}
                    <div className="flex-1 min-w-0 flex items-center gap-3">
                      {/* Status icon */}
                      {statusIcon(trace.status)}

                      {/* Time */}
                      <span
                        className="text-xs text-gray-500 shrink-0 w-28"
                        title={trace.timestamp}
                      >
                        {formatDateTime(trace.timestamp)}
                      </span>

                      {/* Action type */}
                      <span className="font-mono text-xs text-gray-400 shrink-0 w-24">
                        {trace.action_type}
                      </span>

                      {/* Action name */}
                      <span className="text-sm text-gray-300 truncate flex-1">
                        {trace.action_name}
                      </span>

                      {/* Model */}
                      <span className="text-xs text-gray-500 shrink-0 w-20 truncate">
                        {trace.model || '\u2014'}
                      </span>

                      {/* Latency */}
                      <span className="text-xs text-gray-500 shrink-0 w-16 text-right">
                        {formatLatency(trace.latency_ms)}
                      </span>

                      {/* Cost */}
                      <span className="font-mono text-xs text-gray-400 shrink-0 w-16 text-right">
                        {formatCost(trace.cost_usd)}
                      </span>

                      {/* Expand icon */}
                      {isExpanded ? (
                        <ChevronDown size={14} className="text-gray-500 shrink-0" />
                      ) : (
                        <ChevronRight size={14} className="text-gray-500 shrink-0" />
                      )}
                    </div>
                  </button>

                  {/* Expanded detail */}
                  {isExpanded && (
                    <div className="px-5 pb-4 pt-2 ml-10 space-y-3 bg-gray-900/50">
                      <div className="grid grid-cols-2 gap-4">
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Trace ID
                          </span>
                          <code className="text-xs text-gray-300 font-mono">
                            {trace.id}
                          </code>
                        </div>
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Session ID
                          </span>
                          <code className="text-xs text-gray-300 font-mono">
                            {trace.session_id}
                          </code>
                        </div>
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Tokens (In / Out)
                          </span>
                          <span className="text-xs text-gray-300">
                            {formatNumber(trace.tokens_in)} / {formatNumber(trace.tokens_out)}
                          </span>
                        </div>
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Status
                          </span>
                          <span
                            className={`text-xs font-medium ${
                              trace.status === 'allowed'
                                ? 'text-emerald-400'
                                : trace.status === 'denied'
                                  ? 'text-red-400'
                                  : trace.status === 'throttled'
                                    ? 'text-yellow-400'
                                    : 'text-gray-400'
                            }`}
                          >
                            {trace.status}
                          </span>
                        </div>
                      </div>

                      {/* Policy info */}
                      {trace.policy_name && (
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Policy
                          </span>
                          <span className="text-xs text-gray-300">{trace.policy_name}</span>
                          {trace.policy_reason && (
                            <p className="text-xs text-gray-500 mt-0.5">
                              {trace.policy_reason}
                            </p>
                          )}
                        </div>
                      )}

                      {/* Hash chain */}
                      <div>
                        <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                          Hash Chain
                        </span>
                        <div className="text-xs font-mono space-y-1">
                          <div className="text-gray-500">
                            prev: <span className="text-gray-400">{trace.prev_hash || 'genesis'}</span>
                          </div>
                          <div className="text-gray-500">
                            hash: <span className="text-gray-400">{trace.hash}</span>
                          </div>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
