import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Server, Database, HardDrive, Layers, Activity } from 'lucide-react';
import type { HealthStatus } from '../types/telemetry';

export interface TopologyNodeData {
  label: string;
  serviceId: string;
  type: 'gateway' | 'service' | 'database' | 'cache' | 'queue';
  status: HealthStatus;
  cpu: number;
  latency: number;
  rps: number;
  replicas: number;
  isRootCause?: boolean;
}

export const TopologyNode = memo(({ data }: { data: TopologyNodeData }) => {
  const isCritical = data.status === 'critical';
  const isDegraded = data.status === 'degraded';

  const typeIconMap = {
    gateway: Activity,
    service: Server,
    database: Database,
    cache: HardDrive,
    queue: Layers,
  };

  const Icon = typeIconMap[data.type] || Server;

  return (
    <div
      className={`px-4 py-3 bg-[#0F172A] border rounded-xl font-mono min-w-48 shadow-2xl transition-all ${
        isCritical
          ? 'border-rose-600 ring-2 ring-rose-500/80 bg-rose-950/30 animate-pulse-subtle'
          : isDegraded
          ? 'border-amber-600 ring-1 ring-amber-500/60 bg-amber-950/30'
          : 'border-[#1E293B] hover:border-slate-500'
      }`}
    >
      {/* Target input handles */}
      <Handle
        type="target"
        position={Position.Top}
        className="w-3.5 h-3.5 bg-[#0F172A] border-2 border-sky-400 !top-[-7px]"
      />

      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center space-x-2">
          <div
            className={`p-1.5 rounded-lg border ${
              isCritical
                ? 'bg-rose-950 text-rose-400 border-rose-700'
                : isDegraded
                ? 'bg-amber-950 text-amber-400 border-amber-700'
                : 'bg-sky-950 text-sky-400 border-sky-700'
            }`}
          >
            <Icon className="w-3.5 h-3.5" />
          </div>
          <span className="font-bold text-xs text-white">{data.label}</span>
        </div>

        <span
          className={`w-2.5 h-2.5 rounded-full ${
            isCritical
              ? 'bg-rose-500'
              : isDegraded
              ? 'bg-amber-400'
              : 'bg-emerald-400'
          }`}
        />
      </div>

      <div className="grid grid-cols-2 gap-1 text-[10px] text-slate-200 bg-[#07090E] p-2 rounded-lg border border-[#1E293B]">
        <div className="flex items-center justify-between">
          <span className="text-slate-400">CPU:</span>
          <span className={`font-bold ${data.cpu > 80 ? 'text-rose-400' : 'text-slate-100'}`}>
            {data.cpu}%
          </span>
        </div>

        <div className="flex items-center justify-between">
          <span className="text-slate-400">Lat:</span>
          <span className={`font-bold ${data.latency > 150 ? 'text-rose-400' : 'text-slate-100'}`}>
            {data.latency}ms
          </span>
        </div>
      </div>

      {data.isRootCause && (
        <div className="mt-2 text-[9px] bg-rose-950 text-rose-300 border border-rose-700 px-2 py-0.5 rounded font-bold text-center uppercase tracking-wider">
          ROOT CAUSE ISOLATED
        </div>
      )}

      {/* Source output handles */}
      <Handle
        type="source"
        position={Position.Bottom}
        className="w-3.5 h-3.5 bg-[#0F172A] border-2 border-sky-400 !bottom-[-7px]"
      />
    </div>
  );
});
