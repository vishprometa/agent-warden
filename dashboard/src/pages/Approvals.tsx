import { useEffect, useState, useCallback } from 'react';
import { Loader2, ShieldCheck, ShieldX, Clock, ShieldAlert } from 'lucide-react';
import { getApprovals, approveApproval, denyApproval } from '@/lib/api';
import type { Approval } from '@/lib/types';
import { timeAgo, formatDateTime } from '@/lib/format';
import StatusBadge from '@/components/StatusBadge';

export default function Approvals() {
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [loading, setLoading] = useState(true);
  const [actioningId, setActioningId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await getApprovals();
      setApprovals(data.approvals || []);
    } catch (err) {
      console.error('Failed to load approvals:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [load]);

  const handleApprove = async (id: string) => {
    setActioningId(id);
    try {
      await approveApproval(id);
      await load();
    } catch (err) {
      console.error('Failed to approve:', err);
    } finally {
      setActioningId(null);
    }
  };

  const handleDeny = async (id: string) => {
    setActioningId(id);
    try {
      await denyApproval(id);
      await load();
    } catch (err) {
      console.error('Failed to deny:', err);
    } finally {
      setActioningId(null);
    }
  };

  const pending = approvals.filter((a) => a.status === 'pending');
  const resolved = approvals.filter((a) => a.status !== 'pending');

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 text-gray-500 animate-spin" />
      </div>
    );
  }

  return (
    <div className="space-y-8 animate-fade-in">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Approvals</h1>
        <p className="text-sm text-gray-500 mt-1">
          Review and approve or deny agent actions that require human authorization
        </p>
      </div>

      {/* Pending */}
      <div>
        <h2 className="text-sm font-medium text-gray-400 mb-3 flex items-center gap-2">
          <Clock size={14} />
          Pending ({pending.length})
        </h2>

        {pending.length === 0 ? (
          <div className="card p-8 flex flex-col items-center text-gray-500">
            <ShieldAlert size={28} className="mb-2 opacity-30" />
            <p className="text-sm">No pending approvals</p>
          </div>
        ) : (
          <div className="space-y-3">
            {pending.map((approval) => {
              const isTimedOut =
                new Date(approval.timeout_at).getTime() < Date.now();
              return (
                <div
                  key={approval.id}
                  className="card p-5 border-orange-500/20"
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-2">
                        <span className="text-sm font-semibold text-gray-200">
                          {approval.policy_name}
                        </span>
                        <StatusBadge status={approval.status} />
                      </div>

                      <div className="text-xs text-gray-500 space-y-1">
                        <p>
                          Session:{' '}
                          <span className="font-mono text-gray-400">
                            {approval.session_id.slice(0, 12)}...
                          </span>
                        </p>
                        <p>
                          Trace:{' '}
                          <span className="font-mono text-gray-400">
                            {approval.trace_id.slice(0, 12)}...
                          </span>
                        </p>
                        <p>
                          Created:{' '}
                          <span className="text-gray-400" title={approval.created_at}>
                            {timeAgo(approval.created_at)}
                          </span>
                        </p>
                        <p>
                          Timeout:{' '}
                          <span
                            className={
                              isTimedOut ? 'text-red-400' : 'text-gray-400'
                            }
                            title={approval.timeout_at}
                          >
                            {formatDateTime(approval.timeout_at)}
                            {isTimedOut && ' (expired)'}
                          </span>
                        </p>
                      </div>

                      {approval.action_summary && (
                        <div className="mt-3">
                          <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                            Action Summary
                          </span>
                          <pre className="text-xs text-gray-300 bg-gray-800 px-3 py-2 rounded font-mono overflow-x-auto max-w-xl">
                            {typeof approval.action_summary === 'string'
                              ? approval.action_summary
                              : JSON.stringify(approval.action_summary, null, 2)}
                          </pre>
                        </div>
                      )}
                    </div>

                    {/* Action buttons */}
                    <div className="flex flex-col gap-2 ml-4 shrink-0">
                      <button
                        className="px-4 py-2 bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-400 hover:text-emerald-300 border border-emerald-600/30 rounded-lg text-sm font-medium transition-colors flex items-center gap-2 disabled:opacity-50"
                        onClick={() => handleApprove(approval.id)}
                        disabled={actioningId === approval.id}
                      >
                        <ShieldCheck size={14} />
                        Approve
                      </button>
                      <button
                        className="btn-danger flex items-center gap-2"
                        onClick={() => handleDeny(approval.id)}
                        disabled={actioningId === approval.id}
                      >
                        <ShieldX size={14} />
                        Deny
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Resolved History */}
      {resolved.length > 0 && (
        <div>
          <h2 className="text-sm font-medium text-gray-400 mb-3">
            Resolved History
          </h2>

          <div className="card overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Policy
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Session
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Status
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Created
                  </th>
                </tr>
              </thead>
              <tbody>
                {resolved.map((a) => (
                  <tr
                    key={a.id}
                    className="border-b border-gray-800/50 hover:bg-gray-800/30"
                  >
                    <td className="px-4 py-2.5 text-gray-300">
                      {a.policy_name}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-gray-500">
                      {a.session_id.slice(0, 12)}...
                    </td>
                    <td className="px-4 py-2.5">
                      <StatusBadge status={a.status} />
                    </td>
                    <td className="px-4 py-2.5 text-gray-500 text-xs" title={a.created_at}>
                      {timeAgo(a.created_at)}
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
