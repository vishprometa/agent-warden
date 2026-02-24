import { NavLink } from 'react-router-dom';
import {
  Shield,
  LayoutDashboard,
  Radio,
  MonitorDot,
  Bot,
  DollarSign,
  FileCheck,
  ShieldAlert,
  AlertTriangle,
} from 'lucide-react';

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Overview' },
  { to: '/live', icon: Radio, label: 'Live Feed' },
  { to: '/sessions', icon: MonitorDot, label: 'Sessions' },
  { to: '/agents', icon: Bot, label: 'Agents' },
  { to: '/costs', icon: DollarSign, label: 'Costs' },
  { to: '/policies', icon: FileCheck, label: 'Policies' },
  { to: '/approvals', icon: ShieldAlert, label: 'Approvals' },
  { to: '/violations', icon: AlertTriangle, label: 'Violations' },
];

export default function Sidebar() {
  return (
    <aside className="fixed top-0 left-0 h-screen w-60 bg-gray-900 border-r border-gray-800 flex flex-col z-30">
      {/* Logo */}
      <div className="flex items-center gap-2.5 px-5 h-16 border-b border-gray-800 shrink-0">
        <Shield className="w-6 h-6 text-brand-500" />
        <span className="text-lg font-semibold tracking-tight text-gray-100">
          AgentWarden
        </span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-3 px-3">
        <ul className="space-y-0.5">
          {navItems.map(({ to, icon: Icon, label }) => (
            <li key={to}>
              <NavLink
                to={to}
                end={to === '/'}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors duration-150 ${
                    isActive
                      ? 'bg-gray-800 text-gray-100'
                      : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800/60'
                  }`
                }
              >
                <Icon className="w-4.5 h-4.5 shrink-0" size={18} />
                {label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>

      {/* Footer */}
      <div className="px-5 py-4 border-t border-gray-800 text-xs text-gray-600">
        AgentWarden v0.1
      </div>
    </aside>
  );
}
