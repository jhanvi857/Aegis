import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import type { FaultType } from '../../types/telemetry';
import { Flame, ShieldAlert, Play, Square, Server } from 'lucide-react';

export const Chaos: React.FC = () => {
  const { activeFaults, services, injectChaosFault, stopChaosFault, clearAllChaosFaults } = useTelemetryStore();
  const [selectedTargetId, setSelectedTargetId] = useState<string>('');

  // Default target service
  const effectiveTargetId = selectedTargetId || (services.find((s) => s.type !== 'gateway')?.id || services[0]?.id || 'node-a');

  const faultCatalog: { type: FaultType; label: string; description: string }[] = [
    {
      type: 'latency',
      label: 'Network Latency',
      description: 'Inject 950ms artificial request latency and network jitter into the service pipeline.',
    },
    {
      type: 'cpu_stress',
      label: 'CPU Core Saturation',
      description: 'Run computational spin-loops saturating CPU cores up to 98% utilization.',
    },
    {
      type: 'kill_service',
      label: 'Service Crash (SIGKILL)',
      description: 'Simulate sudden container or process termination forcing cascade failovers.',
    },
    {
      type: 'memory_leak',
      label: 'Heap Memory Leak',
      description: 'Continuously allocate heap memory up to critical pod out-of-memory limits.',
    },
    {
      type: 'packet_loss',
      label: 'Packet Loss',
      description: 'Drop 25% of incoming and outgoing TCP network packets.',
    },
    {
      type: 'mq_lag',
      label: 'Message Queue Lag',
      description: 'Inject message queue consumer lag and unconsumed backlog accumulation.',
    },
    {
      type: 'cache_down',
      label: 'Cache Outage',
      description: 'Simulate cache unavailability forcing high-latency database fallbacks.',
    },
    {
      type: 'db_lock',
      label: 'Database Lock',
      description: 'Simulate exclusive lock contention blocking downstream transaction queries.',
    },
    {
      type: 'slow_query',
      label: 'Slow DB Query',
      description: 'Emulate high-latency unindexed query execution across data sinks.',
    },
    {
      type: 'thread_exhaustion',
      label: 'Thread Exhaustion',
      description: 'Saturate worker thread pools to reject incoming concurrent requests.',
    },
  ];

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#26262B] pb-4">
        <div>
          <div className="flex items-center space-x-2">
            <Flame className="w-5 h-5 text-[#AD2831]" />
            <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
              Chaos Engineering & Fault Injection
            </h1>
          </div>
          <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
            Trigger real-time fault injectors against live microservices to test TGNN graph anomaly prediction and self-healing response
          </p>
        </div>

        {activeFaults.length > 0 && (
          <button
            onClick={clearAllChaosFaults}
            className="flex items-center space-x-1.5 px-3.5 py-1.5 bg-[#AD2831]/20 hover:bg-[#AD2831]/30 text-[#f87171] border border-[#AD2831] rounded-md text-xs font-semibold transition-colors"
          >
            <Square className="w-3.5 h-3.5" />
            <span>TERMINATE ALL ACTIVE FAULTS ({activeFaults.length})</span>
          </button>
        )}
      </div>

      {/* Target Node Selection Bar */}
      <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="p-2 bg-[#840032]/20 border border-[#840032] rounded-md text-white">
            <Server className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-white">Target Service for Injection</h4>
            <p className="text-xs text-gray-400">Choose which topology node will receive the injected failure</p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <label className="text-xs text-gray-400">Target Node:</label>
          <select
            value={effectiveTargetId}
            onChange={(e) => setSelectedTargetId(e.target.value)}
            className="bg-[#0B0B0C] text-white border border-[#26262B] rounded px-3 py-1.5 text-xs font-mono focus:border-[#840032] focus:outline-none"
          >
            {services.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name} ({s.id}) — [{s.status.toUpperCase()}]
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Active Fault Status Badges Banner */}
      {activeFaults.length > 0 && (
        <div className="p-4 bg-[#AD2831]/15 border border-[#AD2831]/60 rounded-lg space-y-3">
          <div className="flex items-center space-x-2 text-[11px] font-semibold text-[#f87171] uppercase tracking-[0.07em]">
            <ShieldAlert className="w-4 h-4" />
            <span>ACTIVE FAULTS ({activeFaults.length})</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {activeFaults.map((fault) => (
              <div
                key={fault.id}
                className="p-3 bg-[#0B0B0C] border border-[#AD2831]/60 rounded-md flex items-center justify-between text-xs"
              >
                <div>
                  <div className="flex items-center space-x-2">
                    <span className="font-semibold text-[#fca5a5]">{fault.title}</span>
                    <span className="text-[10px] bg-[#AD2831] text-white px-1.5 py-0.5 rounded font-sans font-semibold tracking-[0.04em] uppercase">
                      ACTIVE
                    </span>
                  </div>
                  <p className="text-[11px] text-gray-400 mt-0.5">
                    Target: <span className="font-mono text-white font-medium">{fault.targetServiceName}</span> | Remaining:{' '}
                    <span className="font-mono text-[#D4A017] font-semibold">{fault.remainingSeconds}s</span>
                  </p>
                </div>

                <button
                  onClick={() => stopChaosFault(fault.id)}
                  className="px-2.5 py-1 bg-[#AD2831] hover:bg-[#901e25] text-white rounded text-[10px] font-semibold tracking-[0.04em] uppercase border border-[#AD2831]"
                >
                  STOP
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Fault Injection Catalog Grid */}
      <div className="space-y-3">
        <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
          REAL BACKEND FAULT SCENARIO CATALOG
        </h3>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {faultCatalog.map((item) => {
            const isRunning = activeFaults.some(
              (f) => f.faultType === item.type && f.targetServiceId === effectiveTargetId
            );

            return (
              <div
                key={item.type}
                className={`p-4 bg-[#151517] border rounded-lg flex flex-col justify-between transition-all ${
                  isRunning
                    ? 'border-[#AD2831] ring-1 ring-[#AD2831] bg-[#AD2831]/10'
                    : 'border-[#26262B] hover:border-[#383840]'
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="font-semibold text-[14px] text-gray-100">{item.label}</h4>
                    <span className="text-[10px] font-mono bg-[#0B0B0C] text-gray-400 border border-[#26262B] px-1.5 py-0.5 rounded uppercase">
                      {item.type}
                    </span>
                  </div>
                  <p className="text-[12px] font-normal leading-[18px] text-gray-400 mb-4">{item.description}</p>
                </div>

                <button
                  onClick={() => injectChaosFault(item.type, effectiveTargetId, 60)}
                  disabled={isRunning}
                  className={`w-full py-2 px-3 rounded-md text-xs font-semibold border transition-all flex items-center justify-center space-x-2 ${
                    isRunning
                      ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831] cursor-not-allowed'
                      : 'bg-[#840032] hover:bg-[#9b053d] text-white border-[#840032]'
                  }`}
                >
                  <Play className="w-3.5 h-3.5" />
                  <span>{isRunning ? 'FAULT RUNNING' : `INJECT ON ${effectiveTargetId.toUpperCase()}`}</span>
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};
