import React, { useState } from 'react';
import {
  Shield,
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

  const safeServices = services || [];
  const safeFaults = activeFaults || [];
  const safeAlerts = alerts || [];

  const [showReportModal, setShowReportModal] = useState(false);
  const [showAlertsDropdown, setShowAlertsDropdown] = useState(false);

  const risk = calculateSystemRisk(safeServices, safeFaults);
  const isCritical = risk >= 80;
  const isDegraded = risk >= 40 && risk < 80;

  const activeAlertsCount = safeAlerts.filter((a) => !a.resolved).length;

  return (
    <>
      <header className="h-16 bg-[#151517] border-b border-[#26262B] px-6 flex items-center justify-between sticky top-0 z-40">
        {/* Brand & Logo */}
        <div className="flex items-center space-x-3">
          <div className="w-8 h-8 bg-[#840032] rounded-lg flex items-center justify-center text-white font-bold shadow-sm border border-[#a80f49]/60">
            <Shield className="w-4 h-4 text-white" />
          </div>
          <div>
            <div className="flex items-baseline space-x-2">
              <span className="font-sans font-bold text-[18px] tracking-[-0.02em] leading-[24px] text-white">
                AEGIS
              </span>
              <span className="text-[10px] bg-[#840032]/25 text-[#e2588a] border border-[#840032]/60 px-1.5 py-0.5 rounded font-sans font-semibold tracking-[0.04em] uppercase">
                OPS
              </span>
            </div>
            <p className="text-[11px] font-normal text-gray-400 leading-[16px] font-sans">
              Self-Healing Infrastructure
            </p>
          </div>
        </div>

        {/* System Health Status Indicator Banner */}
        <div className="hidden md:flex items-center space-x-4 bg-[#0B0B0C] border border-[#26262B] px-3.5 py-1.5 rounded-lg">
          <div className="flex items-center space-x-2">
            <span className="text-[10px] font-semibold tracking-[0.04em] uppercase text-gray-400 font-sans">
              SYSTEM
            </span>
            <span
              className={`inline-flex items-center px-2 py-0.5 rounded text-[10px] font-semibold tracking-[0.04em] uppercase font-sans border ${
                isCritical
                  ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/70'
                  : isDegraded
                  ? 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/70'
                  : 'bg-[#3F8E4F]/15 text-[#52b767] border-[#3F8E4F]/70'
              }`}
            >
              <span
                className={`w-1.5 h-1.5 rounded-full mr-1.5 ${
                  isCritical
                    ? 'bg-[#AD2831]'
                    : isDegraded
                    ? 'bg-[#D4A017]'
                    : 'bg-[#3F8E4F]'
                }`}
              />
              {isCritical ? 'CRITICAL' : isDegraded ? 'WARNING' : 'OPERATIONAL'}
            </span>
          </div>

          <div className="h-3.5 w-px bg-[#26262B]" />

          <div className="flex items-center space-x-1.5 text-[10px] font-sans">
            <span className="font-semibold uppercase tracking-[0.04em] text-gray-400">RISK:</span>
            <span
              className={`font-mono text-xs font-semibold ${
                risk > 70
                  ? 'text-[#AD2831]'
                  : risk > 30
                  ? 'text-[#D4A017]'
                  : 'text-[#3F8E4F]'
              }`}
            >
              {risk}%
            </span>
          </div>

          {activeFaults.length > 0 && (
            <>
              <div className="h-3.5 w-px bg-[#26262B]" />
              <div className="flex items-center space-x-1.5 text-[10px] text-[#AD2831] font-sans font-semibold tracking-[0.04em] uppercase">
                <ShieldAlert className="w-3.5 h-3.5" />
                <span>FAULTS ({activeFaults.length})</span>
              </div>
            </>
          )}
        </div>

        {/* Global Toolbar Quick Controls */}
        <div className="flex items-center space-x-3 font-sans">
          {/* Time Travel Replay Toggle Button */}
          <button
            onClick={() => setTimeTravelActive(!isTimeTravelActive)}
            className={`flex items-center space-x-1.5 px-3 py-1.5 rounded text-xs font-semibold border transition-all ${
              isTimeTravelActive
                ? 'bg-[#89023E] text-white border-[#89023E]'
                : 'bg-[#0B0B0C] text-gray-300 border-[#26262B] hover:border-gray-500 hover:text-white'
            }`}
            title="Scrub back through telemetry history"
          >
            <Clock className="w-3.5 h-3.5 text-[#e2588a]" />
            <span>{isTimeTravelActive ? 'TIME TRAVEL: ON' : 'TIME TRAVEL'}</span>
          </button>

          {/* Incident Report Modal Button */}
          <button
            onClick={() => setShowReportModal(true)}
            className="flex items-center space-x-1.5 bg-[#0B0B0C] hover:bg-[#1E1E22] text-gray-200 border border-[#26262B] hover:border-gray-500 px-3 py-1.5 rounded text-xs font-semibold transition-all"
          >
            <FileText className="w-3.5 h-3.5 text-gray-300" />
            <span className="hidden sm:inline">REPORT</span>
          </button>

          {/* Reset Demo State Button */}
          <button
            onClick={resetDemoState}
            className="flex items-center space-x-1.5 bg-[#0B0B0C] hover:bg-[#1E1E22] text-gray-200 border border-[#26262B] hover:border-gray-500 px-3 py-1.5 rounded text-xs font-semibold transition-all"
            title="Reset system to healthy baseline"
          >
            <RotateCcw className="w-3.5 h-3.5 text-[#3F8E4F]" />
            <span className="hidden lg:inline">RESET</span>
          </button>

          {/* Alert Bell Dropdown */}
          <div className="relative">
            <button
              onClick={() => setShowAlertsDropdown(!showAlertsDropdown)}
              className="relative p-2 text-gray-300 hover:text-white bg-[#0B0B0C] border border-[#26262B] hover:border-gray-500 rounded-lg transition-all"
            >
              <Bell className="w-4 h-4" />
              {activeAlertsCount > 0 && (
                <span className="absolute -top-1 -right-1 bg-[#AD2831] text-white text-[10px] font-mono font-semibold w-4 h-4 rounded-full flex items-center justify-center border border-[#151517]">
                  {activeAlertsCount}
                </span>
              )}
            </button>

            {showAlertsDropdown && (
              <div className="absolute right-0 mt-2 w-80 bg-[#151517] border border-[#26262B] rounded-lg shadow-2xl z-50 p-4 font-sans text-xs">
                <div className="flex items-center justify-between border-b border-[#26262B] pb-2.5 mb-3">
                  <span className="font-bold text-white uppercase tracking-wide">
                    Live Alerts ({safeAlerts.length})
                  </span>
                  <button
                    onClick={() => setShowAlertsDropdown(false)}
                    className="text-gray-400 hover:text-white font-bold"
                  >
                    Close
                  </button>
                </div>

                <div className="max-h-60 overflow-y-auto space-y-2">
                  {safeAlerts.map((alert) => (
                    <div
                      key={alert.id}
                      className={`p-2.5 rounded-lg border ${
                        alert.severity === 'critical'
                          ? 'bg-[#AD2831]/20 border-[#AD2831]/60 text-[#fca5a5]'
                          : alert.severity === 'warning'
                          ? 'bg-[#D4A017]/15 border-[#D4A017]/60 text-[#fde047]'
                          : 'bg-[#3F8E4F]/15 border-[#3F8E4F]/60 text-[#86efac]'
                      }`}
                    >
                      <div className="flex justify-between items-center text-[10px] text-gray-400 mb-1">
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
