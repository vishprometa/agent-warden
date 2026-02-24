import { useEffect, useState, type ReactNode } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  Loader2,
  GitBranch,
  ArrowUpCircle,
  RotateCcw,
  CheckCircle2,
  XCircle,
  Clock,
  ChevronDown,
  ChevronRight,
  FlaskConical,
} from 'lucide-react';
import { getAgent, getAgentVersions, promoteVersion, rollbackVersion } from '@/lib/api';
import type { Agent, AgentVersion } from '@/lib/types';
import { formatDateTime } from '@/lib/format';

function versionStatusStyle(status: string): string {
  switch (status) {
    case 'active':
      return 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20';
    case 'candidate':
      return 'text-blue-400 bg-blue-500/10 border-blue-500/20';
    case 'shadow':
      return 'text-purple-400 bg-purple-500/10 border-purple-500/20';
    case 'retired':
      return 'text-gray-400 bg-gray-500/10 border-gray-500/20';
    case 'rolled_back':
      return 'text-orange-400 bg-orange-500/10 border-orange-500/20';
    default:
      return 'text-gray-400 bg-gray-500/10 border-gray-500/20';
  }
}

function versionStatusIcon(status: string) {
  switch (status) {
    case 'active':
      return <CheckCircle2 size={16} className="text-emerald-400" />;
    case 'candidate':
      return <ArrowUpCircle size={16} className="text-blue-400" />;
    case 'shadow':
      return <FlaskConical size={16} className="text-purple-400" />;
    case 'retired':
      return <Clock size={16} className="text-gray-500" />;
    case 'rolled_back':
      return <RotateCcw size={16} className="text-orange-400" />;
    default:
      return <GitBranch size={16} className="text-gray-500" />;
  }
}

function renderDiff(diff: Record<string, any> | undefined): ReactNode {
  if (!diff || Object.keys(diff).length === 0) {
    return <span className="text-xs text-gray-500 italic">No changes from previous version</span>;
  }

  return (
    <div className="space-y-1">
      {Object.entries(diff).map(([key, value]) => {
        const strValue =
          typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value);
        const isRemoval =
          strValue === 'null' || strValue === 'undefined' || strValue === '""';

        return (
          <div key={key} className="font-mono text-xs flex">
            <span className="text-gray-500 w-40 shrink-0 truncate" title={key}>
              {key}:
            </span>
            <pre
              className={`flex-1 whitespace-pre-wrap break-all ${
                isRemoval ? 'text-red-400' : 'text-emerald-400'
              }`}
            >
              {isRemoval ? `- ${strValue}` : `+ ${strValue}`}
            </pre>
          </div>
        );
      })}
    </div>
  );
}

export default function EvolutionHistory() {
  const { id: agentId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [agent, setAgent] = useState<Agent | null>(null);
  const [versions, setVersions] = useState<AgentVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState('');

  const loadData = async () => {
    if (!agentId) return;
    try {
      const [agentData, versionsData] = await Promise.all([
        getAgent(agentId),
        getAgentVersions(agentId),
      ]);
      setAgent(agentData);
      setVersions(versionsData.versions || []);
    } catch (err) {
      console.error('Failed to load agent evolution:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [agentId]);

  const handlePromote = async (versionId: string) => {
    if (!agentId) return;
    setActionLoading(versionId);
    setActionMessage('');
    try {
      await promoteVersion(agentId, versionId);
      setActionMessage('Version promoted');
      await loadData();
    } catch (err: any) {
      setActionMessage(err.message || 'Promote failed');
    } finally {
      setActionLoading(null);
      setTimeout(() => setActionMessage(''), 3000);
    }
  };

  const handleRollback = async (versionId: string) => {
    if (!agentId) return;
    setActionLoading(versionId);
    setActionMessage('');
    try {
      await rollbackVersion(agentId, versionId);
      setActionMessage('Version rolled back');
      await loadData();
    } catch (err: any) {
      setActionMessage(err.message || 'Rollback failed');
    } finally {
      setActionLoading(null);
      setTimeout(() => setActionMessage(''), 3000);
    }
  };

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
      {/* Back */}
      <button
        onClick={() => navigate(`/agents/${agentId}`)}
        className="btn-ghost flex items-center gap-1.5 -ml-3"
      >
        <ArrowLeft size={16} />
        {agent.name}
      </button>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-100">Evolution History</h1>
          <p className="text-sm text-gray-500 mt-1">
            Version progression for{' '}
            <span className="text-gray-400">{agent.name}</span>
            {' \u00b7 '}
            <span className="font-mono text-gray-400">{agent.id}</span>
          </p>
        </div>
        {actionMessage && (
          <span className="text-xs text-emerald-400 animate-fade-in">
            {actionMessage}
          </span>
        )}
      </div>

      {/* Version Timeline */}
      {versions.length === 0 ? (
        <div className="card p-12 flex flex-col items-center text-gray-500">
          <GitBranch size={32} className="mb-3 opacity-30" />
          <p className="text-sm">No versions recorded</p>
        </div>
      ) : (
        <div className="space-y-0">
          {versions.map((version, idx) => {
            const isExpanded = expandedId === version.id;
            const isActionLoading = actionLoading === version.id;
            const canPromote =
              version.status === 'candidate' || version.status === 'shadow';
            const canRollback = version.status === 'active';

            return (
              <div key={version.id} className="relative">
                {/* Timeline connector */}
                {idx < versions.length - 1 && (
                  <div className="absolute left-[23px] top-14 bottom-0 w-px bg-gray-800" />
                )}

                <div className="card mb-3">
                  {/* Header row */}
                  <button
                    className="w-full px-5 py-4 flex items-center gap-4 text-left"
                    onClick={() => setExpandedId(isExpanded ? null : version.id)}
                  >
                    {/* Timeline node */}
                    <div className="shrink-0">{versionStatusIcon(version.status)}</div>

                    {/* Version info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-gray-200">
                          v{version.version}
                        </span>
                        <span
                          className={`text-xs font-medium px-2 py-0.5 rounded-full border ${versionStatusStyle(version.status)}`}
                        >
                          {version.status}
                        </span>
                      </div>
                      <div className="text-xs text-gray-500 mt-0.5">
                        Created {formatDateTime(version.created_at)}
                        {version.reason && (
                          <>
                            {' \u00b7 '}
                            <span className="text-gray-400">{version.reason}</span>
                          </>
                        )}
                      </div>
                    </div>

                    {/* Actions */}
                    <div className="flex items-center gap-2 shrink-0">
                      {canPromote && (
                        <button
                          className="btn-primary text-xs px-3 py-1.5 flex items-center gap-1.5"
                          onClick={(e) => {
                            e.stopPropagation();
                            handlePromote(version.id);
                          }}
                          disabled={isActionLoading}
                        >
                          <ArrowUpCircle size={12} />
                          {isActionLoading ? 'Promoting...' : 'Promote'}
                        </button>
                      )}
                      {canRollback && (
                        <button
                          className="text-xs px-3 py-1.5 flex items-center gap-1.5 rounded-lg border border-orange-500/30 text-orange-400 hover:bg-orange-500/10 transition-colors"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleRollback(version.id);
                          }}
                          disabled={isActionLoading}
                        >
                          <RotateCcw size={12} />
                          {isActionLoading ? 'Rolling back...' : 'Rollback'}
                        </button>
                      )}

                      {isExpanded ? (
                        <ChevronDown size={14} className="text-gray-500" />
                      ) : (
                        <ChevronRight size={14} className="text-gray-500" />
                      )}
                    </div>
                  </button>

                  {/* Expanded details */}
                  {isExpanded && (
                    <div className="px-5 pb-5 space-y-4 border-t border-gray-800 pt-4">
                      {/* Timestamps */}
                      <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Created
                          </span>
                          <span className="text-xs text-gray-300">
                            {formatDateTime(version.created_at)}
                          </span>
                        </div>
                        {version.promoted_at && (
                          <div>
                            <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                              Promoted
                            </span>
                            <span className="text-xs text-gray-300">
                              {formatDateTime(version.promoted_at)}
                            </span>
                          </div>
                        )}
                        {version.rolled_back_at && (
                          <div>
                            <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                              Rolled Back
                            </span>
                            <span className="text-xs text-gray-300">
                              {formatDateTime(version.rolled_back_at)}
                            </span>
                          </div>
                        )}
                      </div>

                      {/* Diff from previous */}
                      <div>
                        <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">
                          Changes from Previous Version
                        </span>
                        <div className="bg-gray-800 rounded-lg px-4 py-3">
                          {renderDiff(version.diff_from_prev)}
                        </div>
                      </div>

                      {/* Shadow test results */}
                      {version.shadow_test_results && (
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">
                            Shadow Test Results
                          </span>
                          <div className="bg-gray-800 rounded-lg p-4 space-y-3">
                            {/* Summary bar */}
                            <div className="flex items-center gap-4">
                              <div className="flex items-center gap-1.5">
                                <span className="text-xs text-gray-500">Total:</span>
                                <span className="text-sm font-medium text-gray-200">
                                  {version.shadow_test_results.total}
                                </span>
                              </div>
                              <div className="flex items-center gap-1.5">
                                <CheckCircle2 size={12} className="text-emerald-400" />
                                <span className="text-sm font-medium text-emerald-400">
                                  {version.shadow_test_results.passed}
                                </span>
                              </div>
                              <div className="flex items-center gap-1.5">
                                <XCircle size={12} className="text-red-400" />
                                <span className="text-sm font-medium text-red-400">
                                  {version.shadow_test_results.failed}
                                </span>
                              </div>
                              {/* Progress bar */}
                              <div className="flex-1 h-2 bg-gray-700 rounded-full overflow-hidden">
                                <div
                                  className="h-full bg-emerald-500 rounded-full transition-all"
                                  style={{
                                    width: `${
                                      version.shadow_test_results.total > 0
                                        ? (version.shadow_test_results.passed /
                                            version.shadow_test_results.total) *
                                          100
                                        : 0
                                    }%`,
                                  }}
                                />
                              </div>
                            </div>

                            {/* Detail rows */}
                            {version.shadow_test_results.details &&
                              version.shadow_test_results.details.length > 0 && (
                                <div className="space-y-1 pt-2 border-t border-gray-700">
                                  {version.shadow_test_results.details.map(
                                    (detail, dIdx) => (
                                      <div
                                        key={dIdx}
                                        className="flex items-center gap-2 text-xs"
                                      >
                                        {detail.result === 'passed' ? (
                                          <CheckCircle2
                                            size={12}
                                            className="text-emerald-400 shrink-0"
                                          />
                                        ) : (
                                          <XCircle
                                            size={12}
                                            className="text-red-400 shrink-0"
                                          />
                                        )}
                                        <span className="text-gray-300">
                                          {detail.test}
                                        </span>
                                        {detail.message && (
                                          <span className="text-gray-500 ml-auto truncate max-w-[300px]">
                                            {detail.message}
                                          </span>
                                        )}
                                      </div>
                                    ),
                                  )}
                                </div>
                              )}
                          </div>
                        </div>
                      )}

                      {/* Config preview */}
                      {version.config && Object.keys(version.config).length > 0 && (
                        <div>
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">
                            Configuration
                          </span>
                          <pre className="text-xs font-mono text-gray-400 bg-gray-800 rounded-lg px-4 py-3 overflow-x-auto max-h-64 whitespace-pre-wrap">
                            {JSON.stringify(version.config, null, 2)}
                          </pre>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
