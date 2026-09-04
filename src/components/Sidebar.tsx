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
    <aside className="w-64 bg-[#0F172A] border-r border-[#1E293B] flex flex-col justify-between shrink-0 font-mono">
      <div className="py-4">
        <div className="px-4 mb-3 text-[10px] uppercase font-bold text-slate-500 tracking-wider">
          Navigation Console
        </div>
        <nav className="space-y-1 px-3">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => setActiveTab(item.id)}
                className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-lg text-xs font-bold transition-all ${
                  isActive
                    ? 'bg-[#1E293B] text-sky-400 border border-sky-500/50 shadow-sm'
                    : 'text-slate-400 hover:text-slate-200 hover:bg-[#1E293B]/60'
                }`}
              >
                <div className="flex items-center space-x-3">
                  <Icon
                    className={`w-4 h-4 ${
                      isActive ? 'text-sky-400' : 'text-slate-400'
                    }`}
                  />
                  <span>{item.label}</span>
                </div>
                {item.isHero && (
                  <span className="text-[9px] bg-purple-950 text-purple-300 border border-purple-700 px-1.5 py-0.5 rounded font-bold">
                    HERO
                  </span>
                )}
              </button>
            );
          })}
        </nav>
      </div>

      {/* Cluster Footer Info Panel */}
      <div className="p-4 border-t border-[#1E293B] bg-[#07090E]">
        <div className="text-[11px] font-mono text-slate-400 space-y-1.5">
          <div className="flex justify-between">
            <span className="text-slate-500">Cluster:</span>
            <span className="text-slate-200 font-bold">us-east-k8s-01</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">Namespace:</span>
            <span className="text-slate-300">production</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">Telemetry Engine:</span>
            <span className="text-emerald-400 font-bold">LIVE (1s)</span>
          </div>
        </div>
      </div>
    </aside>
  );
};
