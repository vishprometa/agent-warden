import { Routes, Route } from 'react-router-dom';
import Sidebar from '@/components/Sidebar';
import Overview from '@/pages/Overview';
import LiveFeed from '@/pages/LiveFeed';
import Sessions from '@/pages/Sessions';
import SessionDetail from '@/pages/SessionDetail';
import SessionReplay from '@/pages/SessionReplay';
import Agents from '@/pages/Agents';
import AgentDetail from '@/pages/AgentDetail';
import EvolutionHistory from '@/pages/EvolutionHistory';
import Costs from '@/pages/Costs';
import Policies from '@/pages/Policies';
import PolicyEditor from '@/pages/PolicyEditor';
import Approvals from '@/pages/Approvals';
import Violations from '@/pages/Violations';

export default function App() {
  return (
    <div className="min-h-screen bg-gray-950">
      <Sidebar />
      <main className="ml-60 min-h-screen">
        <div className="max-w-7xl mx-auto px-6 py-8">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/live" element={<LiveFeed />} />
            <Route path="/sessions" element={<Sessions />} />
            <Route path="/sessions/:id" element={<SessionDetail />} />
            <Route path="/sessions/:id/replay" element={<SessionReplay />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/agents/:id" element={<AgentDetail />} />
            <Route path="/agents/:id/evolution" element={<EvolutionHistory />} />
            <Route path="/costs" element={<Costs />} />
            <Route path="/policies" element={<Policies />} />
            <Route path="/policies/editor" element={<PolicyEditor />} />
            <Route path="/approvals" element={<Approvals />} />
            <Route path="/violations" element={<Violations />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}
