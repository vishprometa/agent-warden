import { useEffect, useState } from 'react';
import { Loader2, RefreshCw, Shield, AlertTriangle } from 'lucide-react';
import { getPolicies, reloadPolicies } from '@/lib/api';
import type { PolicyConfig } from '@/lib/types';

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

export default function Policies() {
  const [policies, setPolicies] = useState<PolicyConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [reloading, setReloading] = useState(false);
  const [reloadMessage, setReloadMessage] = useState('');

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
      // Refresh the list
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
          <h1 className="text-2xl font-bold text-gray-100">Policies</h1>
          <p className="text-sm text-gray-500 mt-1">
            Governance rules that control agent behavior
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

      {policies.length === 0 ? (
        <div className="card p-12 flex flex-col items-center text-gray-500">
          <Shield size={32} className="mb-3 opacity-30" />
          <p className="text-sm">No policies configured</p>
        </div>
      ) : (
        <div className="space-y-3">
          {policies.map((policy, idx) => (
            <div key={`${policy.name}-${idx}`} className="card p-5">
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-3">
                  <Shield size={16} className="text-gray-500 shrink-0 mt-0.5" />
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
              </div>

              {/* Condition */}
              <div className="mb-3">
                <span className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                  Condition
                </span>
                <code className="text-xs text-gray-300 bg-gray-800 px-3 py-2 rounded block font-mono whitespace-pre-wrap break-all">
                  {policy.condition}
                </code>
              </div>

              {/* Message */}
              <div className="flex items-start gap-2 text-sm text-gray-400">
                <AlertTriangle size={14} className="shrink-0 mt-0.5 text-gray-600" />
                <span>{policy.message}</span>
              </div>

              {/* Approvers */}
              {policy.approvers && policy.approvers.length > 0 && (
                <div className="mt-3 pt-3 border-t border-gray-800">
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
          ))}
        </div>
      )}
    </div>
  );
}
