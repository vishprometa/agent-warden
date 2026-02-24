import { useEffect, useState, type ReactNode } from 'react';
import {
  Loader2,
  RefreshCw,
  Shield,
  Play,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import { getPolicies, reloadPolicies, dryRunPolicy } from '@/lib/api';
import type { PolicyConfig, PolicyDryRunResult } from '@/lib/types';

function effectColor(effect: string): string {
  switch (effect) {
    case 'allow':
      return 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20';
    case 'deny':
      return 'text-red-400 bg-red-500/10 border-red-500/20';
    case 'terminate':
      return 'text-red-400 bg-red-500/10 border-red-500/20';
    case 'throttle':
      return 'text-yellow-400 bg-yellow-500/10 border-yellow-500/20';
    case 'approve':
      return 'text-orange-400 bg-orange-500/10 border-orange-500/20';
    default:
      return 'text-gray-400 bg-gray-500/10 border-gray-500/20';
  }
}

/** Simple CEL syntax highlighting */
function highlightCEL(expr: string): ReactNode {
  const keywords = ['&&', '||', '!', '==', '!=', '>=', '<=', '>', '<', 'in', 'true', 'false'];
  const parts: ReactNode[] = [];
  let remaining = expr;
  let key = 0;

  while (remaining.length > 0) {
    // String literals
    const strMatch = remaining.match(/^("[^"]*"|'[^']*')/);
    if (strMatch) {
      parts.push(
        <span key={key++} className="text-emerald-400">
          {strMatch[0]}
        </span>,
      );
      remaining = remaining.slice(strMatch[0].length);
      continue;
    }

    // Numbers
    const numMatch = remaining.match(/^\d+(\.\d+)?/);
    if (numMatch) {
      parts.push(
        <span key={key++} className="text-amber-400">
          {numMatch[0]}
        </span>,
      );
      remaining = remaining.slice(numMatch[0].length);
      continue;
    }

    // Operators and keywords
    let foundKeyword = false;
    for (const kw of keywords) {
      if (remaining.startsWith(kw)) {
        const nextChar = remaining[kw.length];
        // For word-like keywords, ensure word boundary
        if (/^[a-zA-Z]/.test(kw) && nextChar && /[a-zA-Z0-9_]/.test(nextChar)) continue;
        parts.push(
          <span key={key++} className="text-purple-400 font-semibold">
            {kw}
          </span>,
        );
        remaining = remaining.slice(kw.length);
        foundKeyword = true;
        break;
      }
    }
    if (foundKeyword) continue;

    // Identifiers with dots (e.g., action.type)
    const identMatch = remaining.match(/^[a-zA-Z_][a-zA-Z0-9_.]*(?:\([^)]*\))?/);
    if (identMatch) {
      const ident = identMatch[0];
      if (ident.includes('.')) {
        parts.push(
          <span key={key++} className="text-blue-400">
            {ident}
          </span>,
        );
      } else {
        parts.push(
          <span key={key++} className="text-gray-300">
            {ident}
          </span>,
        );
      }
      remaining = remaining.slice(ident.length);
      continue;
    }

    // Default: single character
    parts.push(
      <span key={key++} className="text-gray-400">
        {remaining[0]}
      </span>,
    );
    remaining = remaining.slice(1);
  }

  return <>{parts}</>;
}

const defaultContext = JSON.stringify(
  {
    action: {
      type: 'tool_call',
      name: 'execute_sql',
      model: 'gpt-4',
    },
    session: {
      agent_id: 'agent-1',
      total_cost: 1.5,
    },
  },
  null,
  2,
);

export default function PolicyEditor() {
  const [policies, setPolicies] = useState<PolicyConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [reloading, setReloading] = useState(false);
  const [reloadMessage, setReloadMessage] = useState('');
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null);

  // Dry run state
  const [contextJson, setContextJson] = useState(defaultContext);
  const [dryRunResults, setDryRunResults] = useState<PolicyDryRunResult[] | null>(null);
  const [dryRunError, setDryRunError] = useState('');
  const [dryRunning, setDryRunning] = useState(false);

  useEffect(() => {
    getPolicies()
      .then((data) => setPolicies(data.policies || []))
      .catch((err) => console.error('Failed to load policies:', err))
      .finally(() => setLoading(false));
  }, []);

  const handleReload = async () => {
    setReloading(true);
    setReloadMessage('');
    try {
      const data = await reloadPolicies();
      setReloadMessage(data.status === 'reloaded' ? 'Policies reloaded' : data.status);
      const pData = await getPolicies();
      setPolicies(pData.policies || []);
    } catch (err) {
      setReloadMessage('Failed to reload policies');
      console.error(err);
    } finally {
      setReloading(false);
      setTimeout(() => setReloadMessage(''), 3000);
    }
  };

  const handleDryRun = async () => {
    setDryRunError('');
    setDryRunResults(null);
    setDryRunning(true);
    try {
      const parsed = JSON.parse(contextJson);
      const data = await dryRunPolicy(parsed);
      setDryRunResults(data.results || []);
    } catch (err: any) {
      if (err instanceof SyntaxError) {
        setDryRunError('Invalid JSON: ' + err.message);
      } else {
        setDryRunError(err.message || 'Dry run failed');
      }
    } finally {
      setDryRunning(false);
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-100">Policy Editor</h1>
          <p className="text-sm text-gray-500 mt-1">
            Inspect, test, and reload governance policies
          </p>
        </div>
        <div className="flex items-center gap-3">
          {reloadMessage && (
            <span className="text-xs text-emerald-400 animate-fade-in">
              {reloadMessage}
            </span>
          )}
          <button
            className="btn-primary flex items-center gap-2"
            onClick={handleReload}
            disabled={reloading}
          >
            <RefreshCw size={14} className={reloading ? 'animate-spin' : ''} />
            Reload Policies
          </button>
        </div>
      </div>

      {/* Policy Cards */}
      {policies.length === 0 ? (
        <div className="card p-12 flex flex-col items-center text-gray-500">
          <Shield size={32} className="mb-3 opacity-30" />
          <p className="text-sm">No policies configured</p>
        </div>
      ) : (
        <div className="space-y-3">
          {policies.map((policy, idx) => {
            const isExpanded = expandedIdx === idx;
            return (
              <div key={`${policy.name}-${idx}`} className="card">
                {/* Header row - clickable */}
                <button
                  className="w-full px-5 py-4 flex items-center justify-between text-left"
                  onClick={() => setExpandedIdx(isExpanded ? null : idx)}
                >
                  <div className="flex items-center gap-3">
                    {isExpanded ? (
                      <ChevronDown size={16} className="text-gray-500 shrink-0" />
                    ) : (
                      <ChevronRight size={16} className="text-gray-500 shrink-0" />
                    )}
                    <Shield size={16} className="text-gray-500 shrink-0" />
                    <div>
                      <h3 className="text-sm font-semibold text-gray-200">
                        {policy.name}
                      </h3>
                      {policy.type && (
                        <span className="text-xs text-gray-500 font-mono">
                          {policy.type}
                        </span>
                      )}
                    </div>
                  </div>
                  <span
                    className={`text-xs font-medium px-2.5 py-1 rounded-full border ${effectColor(policy.effect)}`}
                  >
                    {policy.effect}
                  </span>
                </button>

                {/* Expanded details */}
                {isExpanded && (
                  <div className="px-5 pb-5 space-y-3 border-t border-gray-800 pt-4">
                    {/* CEL Condition */}
                    <div>
                      <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                        Condition (CEL)
                      </span>
                      <pre className="text-xs bg-gray-800 px-4 py-3 rounded font-mono whitespace-pre-wrap break-all leading-relaxed">
                        {highlightCEL(policy.condition)}
                      </pre>
                    </div>

                    {/* Message */}
                    <div className="flex items-start gap-2 text-sm text-gray-400">
                      <AlertTriangle
                        size={14}
                        className="shrink-0 mt-0.5 text-gray-600"
                      />
                      <span>{policy.message}</span>
                    </div>

                    {/* Approvers */}
                    {policy.approvers && policy.approvers.length > 0 && (
                      <div className="pt-3 border-t border-gray-800">
                        <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                          Approvers
                        </span>
                        <div className="flex gap-2 flex-wrap">
                          {policy.approvers.map((a) => (
                            <span
                              key={a}
                              className="text-xs bg-gray-800 text-gray-400 px-2 py-1 rounded"
                            >
                              {a}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Dry Run Section */}
      <div className="card">
        <div className="px-5 py-4 border-b border-gray-800">
          <h2 className="text-sm font-medium text-gray-400">Policy Dry Run</h2>
          <p className="text-xs text-gray-500 mt-0.5">
            Test an action context against all loaded policies
          </p>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="text-xs text-gray-500 uppercase tracking-wider block mb-2">
              Action Context (JSON)
            </label>
            <textarea
              className="w-full h-48 bg-gray-800 border border-gray-700 rounded-lg px-4 py-3 text-xs font-mono text-gray-300 focus:outline-none focus:border-gray-600 resize-y"
              value={contextJson}
              onChange={(e) => setContextJson(e.target.value)}
              spellCheck={false}
            />
          </div>

          <button
            className="btn-primary flex items-center gap-2"
            onClick={handleDryRun}
            disabled={dryRunning}
          >
            <Play size={14} />
            {dryRunning ? 'Evaluating...' : 'Evaluate Policies'}
          </button>

          {/* Error */}
          {dryRunError && (
            <div className="flex items-start gap-2 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3">
              <AlertTriangle size={14} className="shrink-0 mt-0.5" />
              <span className="text-xs">{dryRunError}</span>
            </div>
          )}

          {/* Results */}
          {dryRunResults && (
            <div className="space-y-2">
              <span className="text-xs text-gray-500 uppercase tracking-wider block">
                Results ({dryRunResults.length} policies evaluated)
              </span>
              {dryRunResults.length === 0 ? (
                <p className="text-sm text-gray-500">No policies matched</p>
              ) : (
                dryRunResults.map((result, idx) => (
                  <div
                    key={idx}
                    className={`flex items-center justify-between px-4 py-3 rounded-lg border ${
                      result.matched
                        ? effectColor(result.effect)
                        : 'bg-gray-800/50 border-gray-700 text-gray-500'
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <span
                        className={`w-2 h-2 rounded-full shrink-0 ${
                          result.matched ? 'bg-current' : 'bg-gray-600'
                        }`}
                      />
                      <div>
                        <span className="text-sm font-medium">{result.policy_name}</span>
                        <p className="text-xs opacity-70 mt-0.5">{result.message}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <span
                        className={`text-xs font-medium px-2 py-0.5 rounded-full border ${effectColor(result.effect)}`}
                      >
                        {result.effect}
                      </span>
                      <span className="text-xs">
                        {result.matched ? 'MATCHED' : 'no match'}
                      </span>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
