import type { ReactNode } from 'react';

interface StatCardProps {
  label: string;
  value: string | number;
  icon?: ReactNode;
  change?: string;
  changeType?: 'positive' | 'negative' | 'neutral';
}

export default function StatCard({
  label,
  value,
  icon,
  change,
  changeType = 'neutral',
}: StatCardProps) {
  const changeColor =
    changeType === 'positive'
      ? 'text-emerald-400'
      : changeType === 'negative'
        ? 'text-red-400'
        : 'text-gray-500';

  return (
    <div className="card p-5">
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-gray-400 font-medium">{label}</span>
        {icon && <span className="text-gray-600">{icon}</span>}
      </div>
      <div className="flex items-end gap-2">
        <span className="text-2xl font-semibold text-gray-100 tracking-tight">
          {value}
        </span>
        {change && (
          <span className={`text-xs font-medium mb-0.5 ${changeColor}`}>
            {change}
          </span>
        )}
      </div>
    </div>
  );
}
