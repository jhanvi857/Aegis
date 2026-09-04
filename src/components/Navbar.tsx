import React, { useState } from 'react';
import {
  Activity,
  ShieldAlert,
  Clock,
  RotateCcw,
  FileText,
  Bell,
} from 'lucide-react';
import { useTelemetryStore, calculateSystemRisk } from '../store/useTelemetryStore';
import { IncidentReportModal } from './IncidentReportModal';

interface NavbarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
}

export const Navbar: React.FC<NavbarProps> = ({ activeTab: _activeTab, setActiveTab: _setActiveTab }) => {
  const {
    services,
    activeFaults,
    alerts,
    resetDemoState,
    isTimeTravelActive,
    setTimeTravelActive,
  } = useTelemetryStore();

  const [showReportModal, setShowReportModal] = useState(false);
  const [showAlertsDropdown, setShowAlertsDropdown] = useState(false);

  const risk = calculateSystemRisk(services, activeFaults);
  const isCritical = risk >= 80;
  const isDegraded = risk >= 40 && risk < 80;

  const activeAlertsCount = alerts.filter((a) => !a.resolved).length;

  return (
    <>
      <header className="h-16 bg-[#0F172A] border-b border-[#1E293B] px-6 flex items-center justify-between sticky top-0 z-40">
        {/* Brand & Logo */}
        <div className="flex items-center space-x-3">
          <div className="w-9 h-9 bg-sky-500 rounded-lg flex items-center justify-center text-white font-bold text-lg shadow-md border border-sky-400/40">
            <Activity className="w-5 h-5 text-white" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <span className="font-bold text-white text-base tracking-wider uppercase font-mono">
                PULSE
              </span>
              <span className="text-xs bg-sky-950 text-sky-400 border border-sky-700 px-2 py-0.5 rounded font-mono font-bold tracking-tight">
                OBSERVABILITY v2.4
              </span>
            </div>
            <p className="text-[11px] text-slate-400 font-mono">
              Cloud Infrastructure Control Console
            </p>
          </div>
        </div>

        {/* System Health Status Indicator Banner */}
        <div className="hidden md:flex items-center space-x-4 bg-[#07090E] border border-[#1E293B] px-4 py-1.5 rounded-lg shadow-inner">
          <div className="flex items-center space-x-2">
            <span className="text-xs font-medium text-slate-400 uppercase font-mono">
              System Health:
            </span>
            <span
              className={`inline-flex items-center px-2.5 py-0.5 rounded text-xs font-bold font-mono border ${
                isCritical
                  ? 'bg-rose-950/90 text-rose-300 border-rose-700 animate-pulse-subtle'
                  : isDegraded
                  ? 'bg-amber-950/90 text-amber-300 border-amber-700'
                  : 'bg-emerald-950/90 text-emerald-300 border-emerald-700'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full mr-1.5 ${
                  isCritical
                    ? 'bg-rose-500'
                    : isDegraded
                    ? 'bg-amber-400'
                    : 'bg-emerald-400'
                }`}
              />
              {isCritical ? 'CRITICAL' : isDegraded ? 'DEGRADED' : 'HEALTHY'}
            </span>
          </div>

          <div className="h-4 w-px bg-[#1E293B]" />

          <div className="flex items-center space-x-2 text-xs font-mono">
            <span className="text-slate-400">Risk Score:</span>
            <span
              className={`font-bold ${
                risk > 70
                  ? 'text-rose-400'
                  : risk > 30
                  ? 'text-amber-400'
                  : 'text-emerald-400'
              }`}
            >
              {risk}%
            </span>
          </div>

          {activeFaults.length > 0 && (
            <>
              <div className="h-4 w-px bg-[#1E293B]" />
              <div className="flex items-center space-x-1.5 text-xs text-rose-400 font-mono font-bold animate-pulse">
                <ShieldAlert className="w-3.5 h-3.5" />
                <span>FAULTS INJECTED ({activeFaults.length})</span>
              </div>
            </>
          )}
        </div>

        {/* Global Toolbar Quick Controls */}
        <div className="flex items-center space-x-3 font-mono">
          {/* Time Travel Replay Toggle Button */}
          <button
            onClick={() => setTimeTravelActive(!isTimeTravelActive)}
            className={`flex items-center space-x-1.5 px-3.5 py-1.5 rounded text-xs font-bold border transition-all ${
              isTimeTravelActive
                ? 'bg-purple-950 text-purple-200 border-purple-600 shadow-md'
                : 'bg-[#07090E] text-slate-300 border-[#1E293B] hover:border-slate-500 hover:text-white'
            }`}
            title="Scrub back through telemetry history"
          >
            <Clock className="w-3.5 h-3.5 text-purple-400" />
            <span>{isTimeTravelActive ? 'TIME TRAVEL: ON' : 'TIME TRAVEL'}</span>
          </button>

          {/* Incident Report Modal Button */}
          <button
            onClick={() => setShowReportModal(true)}
            className="flex items-center space-x-1.5 bg-[#07090E] hover:bg-[#1E293B] text-slate-200 border border-[#1E293B] hover:border-slate-500 px-3.5 py-1.5 rounded text-xs font-bold transition-all"
          >
            <FileText className="w-3.5 h-3.5 text-sky-400" />
            <span className="hidden sm:inline">REPORT</span>
          </button>

          {/* Reset Demo State Button */}
          <button
            onClick={resetDemoState}
            className="flex items-center space-x-1.5 bg-[#07090E] hover:bg-[#1E293B] text-slate-200 border border-[#1E293B] hover:border-slate-500 px-3.5 py-1.5 rounded text-xs font-bold transition-all"
            title="Reset system to healthy baseline"
          >
            <RotateCcw className="w-3.5 h-3.5 text-emerald-400" />
            <span className="hidden lg:inline">RESET</span>
          </button>

          {/* Alert Bell Dropdown */}
          <div className="relative">
            <button
              onClick={() => setShowAlertsDropdown(!showAlertsDropdown)}
              className="relative p-2 text-slate-300 hover:text-white bg-[#07090E] border border-[#1E293B] hover:border-slate-500 rounded-lg transition-all"
            >
              <Bell className="w-4 h-4" />
              {activeAlertsCount > 0 && (
                <span className="absolute -top-1 -right-1 bg-rose-600 text-white text-[10px] font-mono font-bold w-4 h-4 rounded-full flex items-center justify-center border border-rose-950">
                  {activeAlertsCount}
                </span>
              )}
            </button>

            {showAlertsDropdown && (
              <div className="absolute right-0 mt-2 w-80 bg-[#0F172A] border border-[#1E293B] rounded-xl shadow-2xl z-50 p-4 font-mono text-xs">
                <div className="flex items-center justify-between border-b border-[#1E293B] pb-2.5 mb-3">
                  <span className="font-bold text-white uppercase tracking-wide">
                    Live Alerts ({alerts.length})
                  </span>
                  <button
                    onClick={() => setShowAlertsDropdown(false)}
                    className="text-slate-400 hover:text-white font-bold"
                  >
                    Close
                  </button>
                </div>

                <div className="max-h-60 overflow-y-auto space-y-2">
                  {alerts.map((alert) => (
                    <div
                      key={alert.id}
                      className={`p-2.5 rounded-lg border ${
                        alert.severity === 'critical'
                          ? 'bg-rose-950/50 border-rose-700/80 text-rose-200'
                          : alert.severity === 'warning'
                          ? 'bg-amber-950/50 border-amber-700/80 text-amber-200'
                          : 'bg-sky-950/50 border-sky-700/80 text-sky-200'
                      }`}
                    >
                      <div className="flex justify-between items-center text-[10px] text-slate-400 mb-1">
                        <span className="font-bold text-white">{alert.serviceName}</span>
                        <span>{alert.timestamp}</span>
                      </div>
                      <p>{alert.message}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </header>

      {showReportModal && (
        <IncidentReportModal onClose={() => setShowReportModal(false)} />
      )}
    </>
  );
};
