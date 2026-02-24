import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Loader2, Bot } from 'lucide-react';
import { getAgents } from '@/lib/api';
import type { Agent } from '@/lib/types';
import { timeAgo } from '@/lib/format';
import DataTable, { type Column } from '@/components/DataTable';

export default function Agents() {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getAgents()
      .then((data) => setAgents(data.agents || []))
      .catch((err) => console.error('Failed to load agents:', err))
      .finally(() => setLoading(false));
  }, []);

  const columns: Column<Agent>[] = [
    {
      key: 'id',
      header: 'Agent ID',
      render: (a) => (
        <div className="flex items-center gap-2">
          <Bot size={14} className="text-gray-500" />
          <span className="font-mono text-xs text-brand-400">{a.id}</span>
        </div>
      ),
    },
    {
      key: 'name',
      header: 'Name',
      render: (a) => <span className="text-gray-200 font-medium">{a.name}</span>,
    },
    {
      key: 'created_at',
      header: 'Created',
      sortable: true,
      render: (a) => (
        <span className="text-gray-500" title={a.created_at}>
          {timeAgo(a.created_at)}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Agents</h1>
        <p className="text-sm text-gray-500 mt-1">
          Registered AI agents under governance
        </p>
      </div>

      <div className="card">
        {loading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="w-5 h-5 text-gray-500 animate-spin" />
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={agents}
            keyField="id"
            onRowClick={(a) => navigate(`/agents/${a.id}`)}
            emptyMessage="No agents registered yet"
          />
        )}
      </div>
    </div>
  );
}
