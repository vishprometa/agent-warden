import { useRef, useEffect, useState } from 'react';
import {
  Radio,
  Pause,
  Play,
  Trash2,
  MessageSquare,
  Wrench,
  Globe,
  Zap,
  Circle,
} from 'lucide-react';
import { useWebSocket } from '@/hooks/useWebSocket';
import StatusBadge from '@/components/StatusBadge';
import { timeAgo, formatCost, formatLatency } from '@/lib/format';
import type { Trace } from '@/lib/types';

const actionTypeIcons: Record<string, typeof MessageSquare> = {
  'llm.chat': MessageSquare,
  'tool.call': Wrench,
  'api.request': Globe,
};

function ActionIcon({ actionType }: { actionType: string }) {
  const Icon = actionTypeIcons[actionType] || Zap;
  return <Icon size={14} className="text-gray-500 shrink-0" />;
}

function TraceRow({ trace }: { trace: Trace }) {
  return (
    <div className="flex items-start gap-3 px-4 py-3 border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors animate-fade-in">
      {/* Action icon */}
      <div className="mt-1">
        <ActionIcon actionType={trace.action_type} />
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="font-mono text-xs text-brand-400">
            {trace.action_type}
          </span>
          <span className="text-gray-300 text-sm font-medium truncate">
            {trace.action_name}
          </span>
        </div>
        <div className="flex items-center gap-3 text-xs text-gray-500">
          <span>Agent: {trace.agent_id}</span>
          {trace.model && <span>Model: {trace.model}</span>}
          {trace.tokens_in + trace.tokens_out > 0 && (
            <span>
              {trace.tokens_in + trace.tokens_out} tokens
            </span>
          )}
          <span>{formatLatency(trace.latency_ms)}</span>
          {trace.cost_usd > 0 && (
            <span className="font-mono">{formatCost(trace.cost_usd)}</span>
          )}
        </div>
        {trace.policy_name && (
          <div className="mt-1 text-xs text-gray-500">
            Policy: <span className="text-gray-400">{trace.policy_name}</span>
            {trace.policy_reason && (
              <> &mdash; {trace.policy_reason}</>
            )}
          </div>
        )}
      </div>

      {/* Right side */}
      <div className="flex flex-col items-end gap-1 shrink-0">
        <StatusBadge status={trace.status} />
        <span className="text-xs text-gray-600" title={trace.timestamp}>
          {timeAgo(trace.timestamp)}
        </span>
      </div>
    </div>
  );
}

export default function LiveFeed() {
  const [paused, setPaused] = useState(false);
  const { traces, connected, error, clear } = useWebSocket({
    maxBuffer: 300,
    enabled: !paused,
  });
  const containerRef = useRef<HTMLDivElement>(null);

  // Keep scrolled to top when new traces arrive (only if not scrolled down)
  useEffect(() => {
    if (containerRef.current && containerRef.current.scrollTop < 100) {
      containerRef.current.scrollTop = 0;
    }
  }, [traces.length]);

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-100">Live Feed</h1>
          <p className="text-sm text-gray-500 mt-1">
            Real-time trace stream via WebSocket
          </p>
        </div>

        <div className="flex items-center gap-3">
          {/* Connection indicator */}
          <div className="flex items-center gap-2 text-xs">
            <Circle
              size={8}
              className={
                connected
                  ? 'fill-emerald-400 text-emerald-400 animate-pulse-dot'
                  : 'fill-red-400 text-red-400'
              }
            />
            <span className={connected ? 'text-emerald-400' : 'text-red-400'}>
              {connected ? 'Connected' : 'Disconnected'}
            </span>
          </div>

          {/* Pause/Resume */}
          <button
            onClick={() => setPaused(!paused)}
            className="btn-ghost flex items-center gap-1.5"
          >
            {paused ? <Play size={14} /> : <Pause size={14} />}
            {paused ? 'Resume' : 'Pause'}
          </button>

          {/* Clear */}
          <button
            onClick={clear}
            className="btn-ghost flex items-center gap-1.5"
          >
            <Trash2 size={14} />
            Clear
          </button>
        </div>
      </div>

      {error && (
        <div className="card p-3 border-red-800 bg-red-950/30 text-red-400 text-sm">
          {error}
        </div>
      )}

      {/* Trace count */}
      <div className="flex items-center gap-2 text-xs text-gray-500">
        <Radio size={12} />
        <span>{traces.length} traces in buffer</span>
        {paused && (
          <span className="text-orange-400 font-medium">(paused)</span>
        )}
      </div>

      {/* Feed */}
      <div className="card overflow-hidden">
        <div
          ref={containerRef}
          className="max-h-[calc(100vh-280px)] overflow-y-auto"
        >
          {traces.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-gray-500">
              <Radio size={32} className="mb-3 opacity-30" />
              <p className="text-sm">Waiting for trace events...</p>
              <p className="text-xs mt-1">
                Traces will appear here in real time as agents execute actions
              </p>
            </div>
          ) : (
            traces.map((trace) => (
              <TraceRow key={trace.id} trace={trace} />
            ))
          )}
        </div>
      </div>
    </div>
  );
}
