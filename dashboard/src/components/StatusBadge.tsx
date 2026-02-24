interface StatusBadgeProps {
  status: string;
  size?: 'sm' | 'md';
}

const statusStyles: Record<string, string> = {
  allowed: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20',
  active: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20',
  approved: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20',
  denied: 'bg-red-500/15 text-red-400 border-red-500/20',
  terminated: 'bg-red-500/15 text-red-400 border-red-500/20',
  throttled: 'bg-yellow-500/15 text-yellow-400 border-yellow-500/20',
  pending: 'bg-orange-500/15 text-orange-400 border-orange-500/20',
  paused: 'bg-orange-500/15 text-orange-400 border-orange-500/20',
  timed_out: 'bg-gray-500/15 text-gray-400 border-gray-500/20',
  completed: 'bg-gray-500/15 text-gray-400 border-gray-500/20',
};

const defaultStyle = 'bg-gray-500/15 text-gray-400 border-gray-500/20';

export default function StatusBadge({ status, size = 'sm' }: StatusBadgeProps) {
  const style = statusStyles[status] || defaultStyle;
  const sizeClasses = size === 'sm' ? 'text-xs px-2 py-0.5' : 'text-sm px-2.5 py-1';

  return (
    <span
      className={`inline-flex items-center font-medium rounded-full border ${style} ${sizeClasses}`}
    >
      {status}
    </span>
  );
}
