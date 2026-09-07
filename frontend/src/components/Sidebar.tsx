import React from 'react';
import {
  LayoutDashboard,
  Server,
  GitFork,
  BrainCircuit,
  Flame,
  ShieldCheck,
  FlaskConical,
  Settings,
} from 'lucide-react';

interface SidebarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ activeTab, setActiveTab }) => {
  const navItems = [
    { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { id: 'services', label: 'Services', icon: Server },
    { id: 'graph', label: 'Dependency Graph', icon: GitFork, isHero: true },
    { id: 'predictions', label: 'Predictions', icon: BrainCircuit },
    { id: 'chaos', label: 'Chaos Engine', icon: Flame },
    { id: 'recovery', label: 'Recovery', icon: ShieldCheck },
    { id: 'experiments', label: 'Experiments', icon: FlaskConical },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];

  return (
    <aside className="w-60 bg-[#151517] border-r border-[#26262B] flex flex-col justify-between shrink-0 font-sans">
      <div className="py-4">
        {/* Operations Section */}
        <div className="px-4 mb-2 text-[10px] font-semibold text-gray-400 tracking-[0.08em] uppercase">
          OPERATIONS
        </div>
        <nav className="space-y-0.5 px-2 mb-4">
          {navItems.slice(0, 4).map((item) => {
            const Icon = item.icon;
            const isActive = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => setActiveTab(item.id)}
                className={`w-full flex items-center justify-between px-3 py-1.5 rounded-md text-[13px] leading-[20px] transition-all ${
                  isActive
                    ? 'bg-[#840032]/25 text-white font-semibold border border-[#840032] shadow-sm'
                    : 'text-gray-400 hover:text-gray-100 hover:bg-[#1E1E22] font-medium'
                }`}
              >
                <div className="flex items-center space-x-2.5">
                  <Icon
                    className={`w-4 h-4 ${
                      isActive ? 'text-[#e2588a]' : 'text-gray-400'
                    }`}
                  />
                  <span>{item.label}</span>
                </div>
                {item.isHero && (
                  <span className="text-[9px] bg-[#840032] text-white px-1.5 py-0.5 rounded font-semibold">
                    CORE
                  </span>
                )}
              </button>
            );
          })}
        </nav>

        {/* Resilience Section */}
        <div className="px-4 mb-2 text-[10px] font-semibold text-gray-400 tracking-[0.08em] uppercase">
          CONTROL & CHAOS
        </div>
        <nav className="space-y-0.5 px-2">
          {navItems.slice(4).map((item) => {
            const Icon = item.icon;
            const isActive = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => setActiveTab(item.id)}
                className={`w-full flex items-center justify-between px-3 py-1.5 rounded-md text-[13px] leading-[20px] transition-all ${
                  isActive
                    ? 'bg-[#840032]/25 text-white font-semibold border border-[#840032] shadow-sm'
                    : 'text-gray-400 hover:text-gray-100 hover:bg-[#1E1E22] font-medium'
                }`}
              >
                <div className="flex items-center space-x-2.5">
                  <Icon
                    className={`w-4 h-4 ${
                      isActive ? 'text-[#e2588a]' : 'text-gray-400'
                    }`}
                  />
                  <span>{item.label}</span>
                </div>
              </button>
            );
          })}
        </nav>
      </div>

      {/* Cluster Footer Info Panel */}
      <div className="p-4 border-t border-[#26262B] bg-[#0B0B0C]">
        <div className="text-[11px] text-gray-400 space-y-1.5">
          <div className="flex justify-between items-center">
            <span>Cluster:</span>
            <span className="font-mono text-gray-200 font-medium">aegis-mesh-01</span>
          </div>
          <div className="flex justify-between items-center">
            <span>Namespace:</span>
            <span className="font-mono text-gray-300">production</span>
          </div>
          <div className="flex justify-between items-center">
            <span>Telemetry:</span>
            <span className="font-mono text-[#3F8E4F] font-semibold text-[10px]">LIVE (1s)</span>
          </div>
        </div>
      </div>
    </aside>
  );
};
