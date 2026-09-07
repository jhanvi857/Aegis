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

  const stateLabel = isCritical ? 'Critical' : isDegraded ? 'Warning' : 'Healthy';

  return (
    <div
      className={`px-3.5 py-3 bg-[#151517] border rounded-lg min-w-48 shadow-lg transition-all ${
        isCritical
          ? 'border-[#AD2831] ring-1 ring-[#AD2831]/70 bg-[#AD2831]/10'
          : isDegraded
          ? 'border-[#D4A017] ring-1 ring-[#D4A017]/60 bg-[#D4A017]/10'
          : 'border-[#26262B] hover:border-[#383840]'
      }`}
    >
      {/* Target input handles */}
      <Handle
        type="target"
        position={Position.Top}
        className="w-2.5 h-2.5 bg-[#151517] border-2 border-[#840032] !top-[-5px]"
      />

      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center space-x-2">
          <div
            className={`p-1.5 rounded-md border ${
              isCritical
                ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60'
                : isDegraded
                ? 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/60'
                : 'bg-[#0B0B0C] text-gray-400 border-[#26262B]'
            }`}
          >
            <Icon className="w-3 h-3" />
          </div>
          <div>
            <div className="font-mono text-[12px] font-semibold leading-tight text-white">{data.label}</div>
            <div className={`font-sans text-[11px] font-semibold leading-tight ${
              isCritical ? 'text-[#f87171]' : isDegraded ? 'text-[#e5b533]' : 'text-[#52b767]'
            }`}>
              {stateLabel}
            </div>
          </div>
        </div>

        <span
          className={`w-1.5 h-1.5 rounded-full shrink-0 ${
            isCritical
              ? 'bg-[#AD2831]'
              : isDegraded
              ? 'bg-[#D4A017]'
              : 'bg-[#3F8E4F]'
          }`}
        />
      </div>

      <div className="grid grid-cols-2 gap-1 text-[10px] text-gray-200 bg-[#0B0B0C] p-2 rounded-md border border-[#26262B]">
        <div className="flex items-center justify-between">
          <span className="font-sans text-[10px] text-gray-400">CPU</span>
          <span className={`font-mono text-[10px] font-normal ${data.cpu > 80 ? 'text-[#AD2831] font-semibold' : 'text-gray-300'}`}>
            {data.cpu}%
          </span>
        </div>

        <div className="flex items-center justify-between">
          <span className="font-sans text-[10px] text-gray-400">Lat</span>
          <span className={`font-mono text-[10px] font-normal ${data.latency > 150 ? 'text-[#AD2831] font-semibold' : 'text-gray-300'}`}>
            {data.latency}ms
          </span>
        </div>
      </div>

      {data.isRootCause && (
        <div className="mt-2 text-[10px] font-sans font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white border border-[#a80f49] px-2 py-0.5 rounded text-center">
          ROOT CAUSE
        </div>
      )}

      {/* Source output handles */}
      <Handle
        type="source"
        position={Position.Bottom}
        className="w-2.5 h-2.5 bg-[#151517] border-2 border-[#840032] !bottom-[-5px]"
      />
    </div>
  );
});
