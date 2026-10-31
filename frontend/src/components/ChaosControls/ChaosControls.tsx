import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import type { FaultType } from '../../types/telemetry';
import {
  Flame,
  Play,
  Square,
  Zap,
  Activity,
  BrainCircuit,
  AlertOctagon,
} from 'lucide-react';

interface ChaosControlsProps {
  compact?: boolean;
  onFaultInjected?: (faultType: FaultType, targetId: string) => void;
  className?: string;
}

export const ChaosControls: React.FC<ChaosControlsProps> = ({
  compact: _compact = false,
  onFaultInjected,
  className = '',
}) => {
  const { services, activeFaults, injectChaosFault, stopChaosFault, clearAllChaosFaults } = useTelemetryStore();
  const [selectedFault, setSelectedFault] = useState<FaultType>('cpu_stress');
  const [selectedTarget, setSelectedTarget] = useState<string>('');
  const [durationSec, setDurationSec] = useState<number>(30);
  const [isInjecting, setIsInjecting] = useState<boolean>(false);

  const effectiveTarget = selectedTarget || (services.find((s) => s.type !== 'gateway')?.id || services[0]?.id || '');

  const faultCatalog: { type: FaultType; label: string; description: string }[] = [
    { type: 'cpu_stress', label: 'CPU Saturation', description: 'Saturates CPU cores to 95%+' },
    { type: 'latency', label: 'Network Latency', description: 'Injects 950ms network delay & jitter' },
    { type: 'kill_service', label: 'Service SIGKILL', description: 'Simulates sudden container termination' },
    { type: 'memory_leak', label: 'Heap Memory Leak', description: 'Rapidly exhausts container RAM limits' },
    { type: 'packet_loss', label: 'Packet Drop (25%)', description: 'Drops 25% of TCP network packets' },
    { type: 'mq_lag', label: 'Queue Backlog', description: 'Artificially delays consumer processing' },
    { type: 'cache_down', label: 'Cache Outage', description: 'Flushes cache & denies read hits' },
    { type: 'db_lock', label: 'Database Lock', description: 'Simulates transaction lock contention' },
  ];

  const handleInject = async () => {
    setIsInjecting(true);
    try {
      await injectChaosFault(selectedFault, effectiveTarget, durationSec);
      if (onFaultInjected) {
        onFaultInjected(selectedFault, effectiveTarget);
      }
    } finally {
      setIsInjecting(false);
    }
  };

  const handleQuickPreset = async (presetType: FaultType, targetId: string, dur: number = 30) => {
    setIsInjecting(true);
    try {
      await injectChaosFault(presetType, targetId, dur);
      if (onFaultInjected) {
        onFaultInjected(presetType, targetId);
      }
    } finally {
      setIsInjecting(false);
    }
  };

  return (
    <div className={`bg-[#0F0F11] border border-[#26262B] rounded-xl overflow-hidden flex flex-col font-sans ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-[#151517] border-b border-[#26262B]">
        <div className="flex items-center space-x-2.5">
          <div className="p-1.5 bg-[#AD2831]/20 border border-[#AD2831] rounded-md text-[#f87171]">
            <Flame className="w-4 h-4" />
          </div>
          <div>
            <span className="text-xs font-semibold text-white tracking-wide">
              CHAOS INJECTION CONTROLS
            </span>
            <span className="text-[11px] text-gray-400 ml-2 font-mono">
              {activeFaults.length} active
            </span>
          </div>
        </div>

        {activeFaults.length > 0 && (
          <button
            onClick={clearAllChaosFaults}
            className="flex items-center space-x-1.5 px-2.5 py-1 bg-[#AD2831]/20 hover:bg-[#AD2831]/30 text-[#f87171] border border-[#AD2831] rounded text-[11px] font-semibold transition-colors"
          >
            <Square className="w-3 h-3" />
            <span>HALT ALL ({activeFaults.length})</span>
          </button>
        )}
      </div>

      <div className="p-4 space-y-4">
        {/* 1-Click Quick Preset Scenarios */}
        <div>
          <span className="text-[10px] font-semibold tracking-wider text-gray-400 uppercase">
            Quick Scenarios
          </span>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mt-2">
            <button
              onClick={() => handleQuickPreset('cpu_stress', effectiveTarget, 30)}
              disabled={isInjecting}
              className="flex items-center space-x-2 p-2 bg-[#151517] hover:bg-[#1C1C20] border border-[#26262B] hover:border-[#AD2831]/60 rounded-lg text-left transition-all group"
            >
              <Zap className="w-3.5 h-3.5 text-[#f87171] shrink-0" />
              <div className="min-w-0">
                <div className="text-[11px] font-semibold text-white group-hover:text-[#f87171] truncate">
                  CPU Saturation
                </div>
                <div className="text-[10px] text-gray-400 font-mono truncate">selected service (30s)</div>
              </div>
            </button>

            <button
              onClick={() => handleQuickPreset('latency', effectiveTarget, 30)}
              disabled={isInjecting}
              className="flex items-center space-x-2 p-2 bg-[#151517] hover:bg-[#1C1C20] border border-[#26262B] hover:border-[#D4A017]/60 rounded-lg text-left transition-all group"
            >
              <Activity className="w-3.5 h-3.5 text-[#e5b533] shrink-0" />
              <div className="min-w-0">
                <div className="text-[11px] font-semibold text-white group-hover:text-[#e5b533] truncate">
                  Latency Spike
                </div>
                <div className="text-[10px] text-gray-400 font-mono truncate">selected service (30s)</div>
              </div>
            </button>

            <button
              onClick={() => handleQuickPreset('kill_service', effectiveTarget, 30)}
              disabled={isInjecting}
              className="flex items-center space-x-2 p-2 bg-[#151517] hover:bg-[#1C1C20] border border-[#26262B] hover:border-red-500/60 rounded-lg text-left transition-all group"
            >
              <AlertOctagon className="w-3.5 h-3.5 text-red-400 shrink-0" />
              <div className="min-w-0">
                <div className="text-[11px] font-semibold text-white group-hover:text-red-400 truncate">
                  SIGKILL Crash
                </div>
                <div className="text-[10px] text-gray-400 font-mono truncate">selected service (30s)</div>
              </div>
            </button>

            <button
              onClick={() => handleQuickPreset('memory_leak', effectiveTarget, 30)}
              disabled={isInjecting}
              className="flex items-center space-x-2 p-2 bg-[#151517] hover:bg-[#1C1C20] border border-[#26262B] hover:border-[#840032]/80 rounded-lg text-left transition-all group"
            >
              <BrainCircuit className="w-3.5 h-3.5 text-[#f07b9e] shrink-0" />
              <div className="min-w-0">
                <div className="text-[11px] font-semibold text-white group-hover:text-[#f07b9e] truncate">
                  Memory Leak
                </div>
                <div className="text-[10px] text-gray-400 font-mono truncate">selected service (30s)</div>
              </div>
            </button>
          </div>
        </div>

        {/* Custom Fault Form */}
        <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <div>
              <label className="text-[10px] font-medium text-gray-400 block mb-1">FAULT INJECTOR</label>
              <select
                value={selectedFault}
                onChange={(e) => setSelectedFault(e.target.value as FaultType)}
                className="w-full bg-[#0B0B0C] text-gray-200 border border-[#26262B] rounded px-2.5 py-1.5 text-xs focus:outline-none focus:border-[#AD2831]"
              >
                {faultCatalog.map((f) => (
                  <option key={f.type} value={f.type}>
                    {f.label}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-[10px] font-medium text-gray-400 block mb-1">TARGET SERVICE</label>
              <select
                value={effectiveTarget}
                onChange={(e) => setSelectedTarget(e.target.value)}
                className="w-full bg-[#0B0B0C] text-gray-200 border border-[#26262B] rounded px-2.5 py-1.5 text-xs focus:outline-none focus:border-[#AD2831]"
              >
                {services.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name} ({s.id}) [{s.status.toUpperCase()}]
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-[10px] font-medium text-gray-400 block mb-1">DURATION</label>
              <select
                value={durationSec}
                onChange={(e) => setDurationSec(Number(e.target.value))}
                className="w-full bg-[#0B0B0C] text-gray-200 border border-[#26262B] rounded px-2.5 py-1.5 text-xs focus:outline-none focus:border-[#AD2831]"
              >
                <option value={15}>15 Seconds</option>
                <option value={30}>30 Seconds</option>
                <option value={60}>60 Seconds</option>
                <option value={120}>120 Seconds</option>
              </select>
            </div>
          </div>

          <div className="flex items-center justify-between pt-1">
            <span className="text-[11px] text-gray-400">
              {faultCatalog.find((f) => f.type === selectedFault)?.description}
            </span>

            <button
              onClick={handleInject}
              disabled={isInjecting}
              className="flex items-center space-x-1.5 px-4 py-1.5 bg-[#AD2831] hover:bg-[#c9323c] text-white rounded text-xs font-semibold shadow transition-colors shrink-0"
            >
              <Play className="w-3.5 h-3.5 fill-white" />
              <span>{isInjecting ? 'Injecting...' : 'INJECT FAULT'}</span>
            </button>
          </div>
        </div>

        {/* Active Faults List */}
        {activeFaults.length > 0 && (
          <div className="space-y-2">
            <span className="text-[10px] font-semibold tracking-wider text-gray-400 uppercase">
              In-Flight Chaos Episodes ({activeFaults.length})
            </span>
            <div className="space-y-1.5 max-h-36 overflow-y-auto">
              {activeFaults.map((fault) => (
                <div
                  key={fault.id}
                  className="flex items-center justify-between p-2.5 bg-[#AD2831]/10 border border-[#AD2831]/40 rounded-lg text-xs"
                >
                  <div className="flex items-center space-x-2.5">
                    <span className="w-2 h-2 rounded-full bg-[#AD2831] animate-ping" />
                    <span className="font-semibold text-white capitalize">
                      {fault.faultType.replace('_', ' ')}
                    </span>
                    <span className="text-gray-400 font-mono">→ {fault.targetServiceName || fault.targetServiceId}</span>
                  </div>

                  <div className="flex items-center space-x-3">
                    <span className="text-[11px] text-[#f87171] font-mono">
                      {fault.remainingSeconds}s remaining
                    </span>
                    <button
                      onClick={() => stopChaosFault(fault.id)}
                      className="px-2 py-0.5 bg-[#AD2831] hover:bg-[#c9323c] text-white rounded text-[10px] font-semibold"
                    >
                      STOP
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
