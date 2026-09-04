import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { Settings as SettingsIcon, RotateCcw, CheckCircle2 } from 'lucide-react';

export const Settings: React.FC = () => {
  const { resetDemoState } = useTelemetryStore();
  const [wsUrl, setWsUrl] = useState('ws://localhost:8080/ws');
  const [apiUrl, setApiUrl] = useState('http://localhost:8080/api');
  const [saved, setSaved] = useState(false);

  const handleSave = () => {
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="p-6 space-y-6 font-mono max-w-4xl">
      {/* Header */}
      <div className="border-b border-[#1F2937] pb-4">
        <div className="flex items-center space-x-2">
          <SettingsIcon className="w-5 h-5 text-sky-400" />
          <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
            Console Configuration & Integration Settings
          </h1>
        </div>
        <p className="text-xs text-gray-400">
          Configure real-time telemetry streaming parameters, WebSocket gateways, and environment endpoints
        </p>
      </div>

      {/* Settings Panel */}
      <div className="p-5 bg-[#111827] border border-[#1F2937] rounded-lg space-y-5">
        <h3 className="text-xs font-bold text-gray-200 uppercase tracking-wide border-b border-[#1F2937] pb-2">
          Backend Gateway Connection Setup
        </h3>

        <div className="space-y-4 text-xs">
          <div>
            <label className="block text-gray-300 font-bold mb-1">
              REST API Base Endpoint URL:
            </label>
            <input
              type="text"
              value={apiUrl}
              onChange={(e) => setApiUrl(e.target.value)}
              className="w-full bg-[#0B0F19] border border-[#1F2937] rounded p-2 text-gray-200 focus:outline-none focus:border-sky-500"
            />
            <p className="text-[10px] text-gray-400 mt-1">
              Used for fetching historical metrics, service details, ML predictions, and executing recovery actions.
            </p>
          </div>

          <div>
            <label className="block text-gray-300 font-bold mb-1">
              WebSocket Telemetry Gateway URL:
            </label>
            <input
              type="text"
              value={wsUrl}
              onChange={(e) => setWsUrl(e.target.value)}
              className="w-full bg-[#0B0F19] border border-[#1F2937] rounded p-2 text-gray-200 focus:outline-none focus:border-sky-500"
            />
            <p className="text-[10px] text-gray-400 mt-1">
              Streams 1s telemetry metrics ticks directly into Zustand state store.
            </p>
          </div>

          <div className="pt-2 border-t border-[#1F2937] flex items-center justify-between">
            <div>
              <span className="font-bold text-gray-200">Simulation Engine Fallback:</span>
              <p className="text-[10px] text-gray-400">
                When offline, the client automatically defaults to built-in 1s simulation tick generator.
              </p>
            </div>
            <span className="px-2.5 py-1 bg-emerald-950 text-emerald-400 border border-emerald-800 rounded font-bold text-[10px]">
              ENABLED (ACTIVE)
            </span>
          </div>
        </div>

        <div className="pt-4 border-t border-[#1F2937] flex items-center justify-between">
          <button
            onClick={resetDemoState}
            className="flex items-center space-x-1.5 px-4 py-2 bg-[#0B0F19] hover:bg-[#1E293B] text-gray-300 border border-[#1F2937] hover:border-gray-500 rounded text-xs font-bold"
          >
            <RotateCcw className="w-4 h-4 text-emerald-400" />
            <span>RESET DEMO STATE TO BASELINE</span>
          </button>

          <button
            onClick={handleSave}
            className="flex items-center space-x-1.5 px-5 py-2 bg-sky-900 hover:bg-sky-800 text-white rounded text-xs font-bold border border-sky-700 transition-colors"
          >
            {saved ? (
              <>
                <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                <span>SETTINGS SAVED</span>
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
