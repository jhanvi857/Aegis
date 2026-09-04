import React from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import type { FaultType } from '../../types/telemetry';
import { Flame, ShieldAlert, Play, Square } from 'lucide-react';

export const Chaos: React.FC = () => {
  const { activeFaults, injectChaosFault, stopChaosFault, clearAllChaosFaults } = useTelemetryStore();

  const faultCatalog: { type: FaultType; label: string; defaultTarget: string; description: string }[] = [
    {
      type: 'slow_db',
      label: 'Slow Database',
      defaultTarget: 'database',
      description: 'Inject 350ms query delay & connection saturation into PostgreSQL DB.',
    },
    {
      type: 'high_cpu',
      label: 'High CPU',
      defaultTarget: 'payment',
      description: 'Saturate CPU cores to 98% utilization on payment microservice pods.',
    },
    {
      type: 'kill_redis',
      label: 'Kill Redis',
      defaultTarget: 'redis',
      description: 'Simulate immediate Redis cache cluster crash forcing 100% DB fallback.',
    },
    {
      type: 'kafka_lag',
      label: 'Kafka Lag',
      defaultTarget: 'kafka',
      description: 'Inject message queue consumer lag & partition offsets bottleneck.',
    },
    {
      type: 'packet_loss',
      label: 'Packet Loss',
      defaultTarget: 'orders',
      description: 'Simulate 25% dropped TCP packet loss on orders API network traffic.',
    },
    {
      type: 'memory_leak',
      label: 'Memory Leak',
      defaultTarget: 'notifications',
      description: 'Gradually leak heap RAM up to 96% on notification workers.',
    },
    {
      type: 'network_delay',
      label: 'Network Delay',
      defaultTarget: 'gateway',
      description: 'Inject cross-AZ inter-service latency jitter into ingress API Gateway.',
    },
  ];

  return (
    <div className="p-6 space-y-6 font-mono">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#1F2937] pb-4">
        <div>
          <div className="flex items-center space-x-2">
            <Flame className="w-5 h-5 text-red-500" />
            <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
              Chaos Engineering & Fault Injection Engine
            </h1>
          </div>
          <p className="text-xs text-gray-400">
            Inject synthetic microservice failure scenarios to test ML anomaly detection & system resilience
          </p>
        </div>

        {activeFaults.length > 0 && (
          <button
            onClick={clearAllChaosFaults}
            className="flex items-center space-x-1.5 px-4 py-2 bg-red-950 hover:bg-red-900 text-red-300 border border-red-800 rounded text-xs font-bold transition-colors"
          >
            <Square className="w-4 h-4" />
            <span>TERMINATE ALL ACTIVE FAULTS ({activeFaults.length})</span>
          </button>
        )}
      </div>

      {/* Active Fault Status Badges Banner */}
      {activeFaults.length > 0 && (
        <div className="p-4 bg-red-950/40 border border-red-800/80 rounded-lg space-y-3">
          <div className="flex items-center space-x-2 text-xs font-bold text-red-400 uppercase">
            <ShieldAlert className="w-4 h-4 animate-pulse" />
            <span>Active Injected Faults ({activeFaults.length})</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {activeFaults.map((fault) => (
              <div
                key={fault.id}
                className="p-3 bg-[#0B0F19] border border-red-800/60 rounded flex items-center justify-between text-xs"
              >
                <div>
                  <div className="flex items-center space-x-2">
                    <span className="font-bold text-red-300">{fault.title}</span>
                    <span className="text-[10px] bg-red-900 text-red-200 px-1.5 py-0.5 rounded font-mono">
                      RUNNING
                    </span>
                  </div>
                  <p className="text-[11px] text-gray-400">
                    Target: <span className="text-white">{fault.targetServiceName}</span> | Time Remaining:{' '}
                    <span className="text-amber-400 font-bold">{fault.remainingSeconds}s</span>
                  </p>
                </div>

                <button
                  onClick={() => stopChaosFault(fault.id)}
                  className="px-2.5 py-1 bg-red-900 hover:bg-red-800 text-white rounded text-[11px] font-bold border border-red-700"
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
        <h3 className="text-xs font-bold text-gray-300 uppercase tracking-wide">
          Select Fault Scenario to Inject
        </h3>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {faultCatalog.map((item) => {
            const isRunning = activeFaults.some((f) => f.faultType === item.type);

            return (
              <div
                key={item.type}
                className={`p-5 bg-[#111827] border rounded-lg font-mono flex flex-col justify-between transition-all ${
                  isRunning
                    ? 'border-red-600 ring-1 ring-red-600 bg-red-950/20'
                    : 'border-[#1F2937] hover:border-gray-600'
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="font-bold text-sm text-gray-100">{item.label}</h4>
                    <span className="text-[10px] bg-[#0B0F19] text-gray-400 border border-[#1F2937] px-2 py-0.5 rounded uppercase">
                      {item.type}
                    </span>
                  </div>
                  <p className="text-xs text-gray-400 mb-4">{item.description}</p>
                </div>

                <button
                  onClick={() => injectChaosFault(item.type, item.defaultTarget, 60)}
                  disabled={isRunning}
                  className={`w-full py-2 px-4 rounded text-xs font-bold font-mono border transition-all flex items-center justify-center space-x-2 ${
                    isRunning
                      ? 'bg-red-950 text-red-400 border-red-800 cursor-not-allowed'
                      : 'bg-red-900 hover:bg-red-800 text-white border-red-700'
                  }`}
                >
                  <Play className="w-3.5 h-3.5" />
                  <span>{isRunning ? 'FAULT INJECTED (RUNNING)' : `INJECT [ ${item.label.toUpperCase()} ]`}</span>
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};
