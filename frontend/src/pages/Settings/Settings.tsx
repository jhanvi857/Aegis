import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { Settings as SettingsIcon, RotateCcw, CheckCircle2 } from 'lucide-react';

export const Settings: React.FC = () => {
  const { resetDemoState } = useTelemetryStore();
  const [wsUrl, setWsUrl] = useState(import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws');
  const [apiUrl, setApiUrl] = useState(import.meta.env.VITE_API_BASE_URL || 'http://localhost:8000/api');
  const [saved, setSaved] = useState(false);

  const handleSave = () => {
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="p-6 space-y-6 font-sans max-w-4xl">
      {/* Header */}
      <div className="border-b border-[#26262B] pb-4">
        <div className="flex items-center space-x-2">
          <SettingsIcon className="w-5 h-5 text-[#840032]" />
          <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
            Console Configuration & Integration Settings
          </h1>
        </div>
        <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
          Configure real-time telemetry streaming parameters, WebSocket gateways, and environment endpoints
        </p>
      </div>

      {/* Settings Panel */}
      <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg space-y-4">
        <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px] border-b border-[#26262B] pb-2">
          BACKEND GATEWAY CONNECTION SETUP
        </h3>

        <div className="space-y-4 text-xs">
          <div>
            <label className="block text-gray-300 text-[11px] font-semibold uppercase tracking-[0.06em] mb-1.5">
              REST API Base Endpoint URL:
            </label>
            <input
              type="text"
              value={apiUrl}
              onChange={(e) => setApiUrl(e.target.value)}
              className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md p-2 font-mono text-xs text-gray-200 focus:outline-none focus:border-[#840032]"
            />
            <p className="text-[11px] text-gray-400 mt-1 font-normal">
              Used for fetching historical metrics, service details, ML predictions, and executing recovery actions.
            </p>
          </div>

          <div>
            <label className="block text-gray-300 text-[11px] font-semibold uppercase tracking-[0.06em] mb-1.5">
              WebSocket Telemetry Gateway URL:
            </label>
            <input
              type="text"
              value={wsUrl}
              onChange={(e) => setWsUrl(e.target.value)}
              className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md p-2 font-mono text-xs text-gray-200 focus:outline-none focus:border-[#840032]"
            />
            <p className="text-[11px] text-gray-400 mt-1 font-normal">
              Streams 1s telemetry metrics ticks directly into Zustand state store.
            </p>
          </div>

          <div className="pt-2 border-t border-[#26262B] flex items-center justify-between">
            <div>
              <span className="font-semibold text-gray-200 text-xs">Simulation Engine Fallback:</span>
              <p className="text-[11px] text-gray-400 font-normal">
                When offline, the client automatically defaults to built-in 1s simulation tick generator.
              </p>
            </div>
            <span className="px-2 py-0.5 bg-[#3F8E4F]/10 text-[#3F8E4F] border border-[#3F8E4F]/30 rounded text-[10px] font-semibold tracking-[0.04em] uppercase">
              ENABLED (ACTIVE)
            </span>
          </div>

          {/* Human Approval Authority Boundary Config */}
          <div className="pt-2 border-t border-[#26262B] flex items-center justify-between">
            <div>
              <span className="font-semibold text-gray-200 text-xs flex items-center gap-2">
                Human Approval Gate (Authority Boundary):
                <span className="px-1.5 py-0.5 text-[9px] font-semibold tracking-[0.04em] uppercase rounded bg-amber-500/10 text-[#D4A017] border border-[#D4A017]/40">
                  HIGH-RISK ACTIONS
                </span>
              </span>
              <p className="text-[11px] text-gray-400 font-normal">
                Requires manual operator sign-off in the Recovery console before executing cascading cluster reboots or pool terminations.
              </p>
            </div>
            <span className="px-2 py-0.5 bg-[#840032]/20 text-[#D4A017] border border-[#840032] rounded text-[10px] font-semibold tracking-[0.04em] uppercase">
              ENFORCED
            </span>
          </div>
        </div>

        <div className="pt-3 border-t border-[#26262B] flex items-center justify-between">
          <button
            onClick={resetDemoState}
            className="flex items-center space-x-1.5 px-3.5 py-1.5 bg-[#0B0B0C] hover:bg-[#1a1a1d] text-gray-300 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors"
          >
            <RotateCcw className="w-3.5 h-3.5 text-[#3F8E4F]" />
            <span>RESET DEMO STATE</span>
          </button>

          <button
            onClick={handleSave}
            className="flex items-center space-x-1.5 px-4 py-1.5 bg-[#840032] hover:bg-[#9c023d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors shadow-sm"
          >
            {saved ? (
              <>
                <CheckCircle2 className="w-3.5 h-3.5 text-[#3F8E4F]" />
                <span>SAVED</span>
              </>
            ) : (
              <span>SAVE CONFIGURATION</span>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
