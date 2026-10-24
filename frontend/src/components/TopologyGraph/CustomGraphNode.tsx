import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import {
  Server,
  Database,
  HardDrive,
  Layers,
  Activity,
  AlertTriangle,
  Eye,
  Sparkles,
  Zap,
  EyeOff,
  Shield,
} from 'lucide-react';
import type { MonitoringMode } from '../../api/registry';

export interface CustomNodeData extends Record<string, unknown> {
  label: string;
  serviceId: string;
  type: 'gateway' | 'service' | 'database' | 'cache' | 'queue' | string;
  monitoringMode?: MonitoringMode;
  status?: string;
  tier?: string;
  namespace?: string;
  cpu?: number;
  latency?: number;
  rps?: number;
  replicas?: number;
  hasRecoveryBinding?: boolean;
  isUnconnected?: boolean;
  isUnmonitored?: boolean;
  isRootCause?: boolean;
  isDimmed?: boolean;
  isHighlighted?: boolean;
  lastTelemetryTime?: string;
}

export const CustomGraphNode = memo(({ data, selected }: { data: CustomNodeData; selected?: boolean }) => {
  const isUnconnected = data.isUnconnected || data.status === 'unconnected';
  const isUnmonitored = data.isUnmonitored || data.monitoringMode === 'ignore';
  const isCritical = data.status === 'critical' || data.status === 'degraded_critical';
  const isWarning = data.status === 'degraded' || data.status === 'no_data' || data.status === 'stale';

  const typeIconMap: Record<string, any> = {
    gateway: Activity,
    service: Server,
    database: Database,
    cache: HardDrive,
    queue: Layers,
  };
  const Icon = typeIconMap[data.type] || Server;

  const modeIconMap: Record<string, { icon: any; color: string; bg: string; label: string }> = {
    observe: { icon: Eye, color: 'text-blue-400', bg: 'bg-blue-500/15 border-blue-500/40', label: 'Observe' },
    recommend: { icon: Sparkles, color: 'text-amber-400', bg: 'bg-amber-500/15 border-amber-500/40', label: 'Recommend' },
    auto_remediate: { icon: Zap, color: 'text-emerald-400', bg: 'bg-emerald-500/15 border-emerald-500/40', label: 'Auto-Remediate' },
    ignore: { icon: EyeOff, color: 'text-gray-400', bg: 'bg-gray-500/15 border-gray-500/40', label: 'Ignore' },
  };
  const modeConfig = modeIconMap[data.monitoringMode || 'observe'] || modeIconMap.observe;
  const ModeIcon = modeConfig.icon;

  return (
    <div
      className={`relative px-3.5 py-3 rounded-lg min-w-52 shadow-xl transition-all duration-150 select-none ${
        data.isDimmed ? 'opacity-25 grayscale' : 'opacity-100'
      } ${
        isUnmonitored
          ? 'bg-[#151517]/90 border-2 border-dashed border-gray-600 bg-[repeating-linear-gradient(45deg,#1f1f23,#1f1f23_10px,#151517_10px,#151517_20px)]'
          : isUnconnected
          ? 'bg-[#18181b] border-2 border-dashed border-amber-500/60'
          : isCritical
          ? 'bg-[#151517] border-2 border-[#AD2831] ring-2 ring-[#AD2831]/50'
          : isWarning
          ? 'bg-[#151517] border-2 border-[#D4A017] ring-1 ring-[#D4A017]/40'
          : 'bg-[#151517] border border-[#2E2E35] hover:border-[#840032]/80'
      } ${selected ? '!border-[#840032] !ring-2 !ring-[#840032]' : ''}`}
    >
      {/* Target input handles on all sides for flexible routing */}
      <Handle type="target" position={Position.Top} id="top" className="w-2.5 h-2.5 bg-[#840032] border-2 border-[#151517] !top-[-6px]" />
      <Handle type="target" position={Position.Left} id="left" className="w-2.5 h-2.5 bg-[#840032] border-2 border-[#151517] !left-[-6px]" />

      {/* Header bar: Icon, Label, Mode badge */}
      <div className="flex items-center justify-between gap-2 mb-2">
        <div className="flex items-center space-x-2 min-w-0">
          <div
            className={`p-1.5 rounded-md border shrink-0 ${
              isUnconnected
                ? 'bg-amber-950/40 text-amber-400 border-amber-700/50'
                : isUnmonitored
                ? 'bg-gray-800 text-gray-400 border-gray-700'
                : isCritical
                ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60'
                : isWarning
                ? 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/60'
                : 'bg-[#0B0B0C] text-gray-300 border-[#26262B]'
            }`}
          >
            {isUnconnected ? <AlertTriangle className="w-3.5 h-3.5" /> : <Icon className="w-3.5 h-3.5" />}
          </div>
          <div className="min-w-0">
            <div className="font-mono text-[12px] font-semibold leading-tight text-white truncate max-w-28" title={data.label || data.serviceId}>
              {data.label || data.serviceId}
            </div>
            <div className="text-[10px] text-gray-400 font-mono leading-tight truncate">
              {data.namespace || 'default'}
            </div>
          </div>
        </div>

        {/* Mode pill */}
        <div className={`flex items-center space-x-1 px-1.5 py-0.5 rounded text-[9px] font-semibold border shrink-0 ${modeConfig.bg} ${modeConfig.color}`} title={`Mode: ${modeConfig.label}`}>
          <ModeIcon className="w-2.5 h-2.5" />
          <span>{modeConfig.label}</span>
        </div>
      </div>

      {/* Status banner if unconnected or unmonitored */}
      {isUnconnected ? (
        <div className="mb-2 px-2 py-1 bg-amber-500/10 border border-amber-500/30 rounded text-[10px] text-amber-300 flex items-center space-x-1">
          <AlertTriangle className="w-3 h-3 shrink-0" />
          <span className="font-medium">Unconnected — No Telemetry</span>
        </div>
      ) : isUnmonitored ? (
        <div className="mb-2 px-2 py-1 bg-gray-800/80 border border-gray-700 rounded text-[10px] text-gray-300 flex items-center space-x-1">
          <EyeOff className="w-3 h-3 shrink-0" />
          <span className="font-medium">Unmonitored Dependency</span>
        </div>
      ) : (
        /* Real-time Telemetry Metrics */
        <div className="grid grid-cols-2 gap-1 text-[10px] text-gray-200 bg-[#0B0B0C] p-1.5 rounded-md border border-[#26262B] mb-1.5">
          <div className="flex items-center justify-between min-w-0 px-1">
            <span className="font-sans text-[10px] text-gray-400">CPU</span>
            <span className={`font-mono text-[10px] ${data.cpu && data.cpu > 80 ? 'text-[#AD2831] font-semibold' : 'text-gray-300'}`}>
              {data.cpu !== undefined ? `${Number(data.cpu).toFixed(0)}%` : '--'}
            </span>
          </div>
          <div className="flex items-center justify-between min-w-0 px-1">
            <span className="font-sans text-[10px] text-gray-400">Lat</span>
            <span className={`font-mono text-[10px] ${data.latency && data.latency > 150 ? 'text-[#AD2831] font-semibold' : 'text-gray-300'}`}>
              {data.latency !== undefined ? `${Number(data.latency).toFixed(0)}ms` : '--'}
            </span>
          </div>
        </div>
      )}

      {/* Footer tags: Tier & Recovery Binding Indicator */}
      <div className="flex items-center justify-between text-[9px] text-gray-400 pt-1 border-t border-[#26262B]">
        <span className="font-mono uppercase tracking-wider text-gray-400">{data.tier || 'tier-2'}</span>
        {data.hasRecoveryBinding ? (
          <span className="flex items-center space-x-1 text-emerald-400 font-medium" title="Recovery binding active">
            <Shield className="w-2.5 h-2.5" />
            <span>Remediable</span>
          </span>
        ) : (
          <span className="text-gray-400 font-medium" title="No recovery binding (observe-only)">
            Observe-only
          </span>
        )}
      </div>

      {data.isRootCause && (
        <div className="mt-2 text-[10px] font-sans font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white border border-[#a80f49] px-2 py-0.5 rounded text-center shadow">
          ROOT CAUSE
        </div>
      )}

      {/* Source output handles */}
      <Handle type="source" position={Position.Bottom} id="bottom" className="w-2.5 h-2.5 bg-[#840032] border-2 border-[#151517] !bottom-[-6px]" />
      <Handle type="source" position={Position.Right} id="right" className="w-2.5 h-2.5 bg-[#840032] border-2 border-[#151517] !right-[-6px]" />
    </div>
  );
});
