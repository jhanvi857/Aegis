import React from 'react';
import type { Microservice } from '../types/telemetry';
import { Cpu, Clock, HardDrive, Layers, ChevronRight } from 'lucide-react';

interface ServiceCardProps {
  service: Microservice;
  onSelect: (serviceId: string) => void;
  isSelected?: boolean;
}

export const ServiceCard: React.FC<ServiceCardProps> = ({
  service,
  onSelect,
  isSelected = false,
}) => {
  const isCritical = service.status === 'critical';
  const isDegraded = service.status === 'degraded';

  const statusColorMap = {
    healthy: 'bg-[#3F8E4F]/15 text-[#52b767] border-[#3F8E4F]/50',
    degraded: 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/50',
    critical: 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60 animate-pulse-subtle',
  };

  const statusLabel = {
    healthy: 'HEALTHY',
    degraded: 'WARNING',
    critical: 'CRITICAL',
  };

  return (
    <div
      onClick={() => onSelect(service.id)}
      className={`p-4 bg-[#151517] border rounded-lg font-sans cursor-pointer transition-all shadow-sm ${
        isSelected
          ? 'border-[#840032] ring-1 ring-[#840032] bg-[#840032]/10'
          : isCritical
          ? 'border-[#AD2831]/70 bg-[#AD2831]/10'
          : isDegraded
          ? 'border-[#D4A017]/70 bg-[#D4A017]/10'
          : 'border-[#26262B] hover:border-[#383840]'
      }`}
    >
      {/* Header */}
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="flex items-center space-x-2">
            <span
              className={`w-1.5 h-1.5 rounded-full ${
                isCritical ? 'bg-[#AD2831]' : isDegraded ? 'bg-[#D4A017]' : 'bg-[#3F8E4F]'
              }`}
            />
            <h3 className="font-mono text-[16px] font-semibold leading-[24px] text-white">{service.name}</h3>
          </div>
          <span className="text-[10px] text-gray-400 uppercase font-semibold tracking-[0.06em]">
            Type: <span className="font-mono text-gray-300">{service.type}</span> | Ver: <span className="font-mono text-gray-300">{service.version}</span>
          </span>
        </div>

        <span
          className={`text-[10px] font-semibold tracking-[0.04em] uppercase leading-[16px] px-2 py-0.5 rounded border font-sans ${
            statusColorMap[service.status]
          }`}
        >
          {statusLabel[service.status]}
        </span>
      </div>

      {/* Primary Telemetry Metrics */}
      <div className="grid grid-cols-2 gap-2 my-3 text-xs">
        <div className="bg-[#0B0B0C] p-2 rounded-md border border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-gray-400">
            <Cpu className="w-3.5 h-3.5 text-gray-400" />
            <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">CPU</span>
          </div>
          <span className={`font-mono text-xs font-semibold ${service.cpu > 80 ? 'text-[#AD2831]' : 'text-gray-200'}`}>
            {service.cpu}%
          </span>
        </div>

        <div className="bg-[#0B0B0C] p-2 rounded-md border border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-gray-400">
            <Clock className="w-3.5 h-3.5 text-[#D4A017]" />
            <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">Latency</span>
          </div>
          <span
            className={`font-mono text-xs font-semibold ${service.latency > 150 ? 'text-[#AD2831]' : 'text-gray-200'}`}
          >
            {service.latency}ms
          </span>
        </div>

        <div className="bg-[#0B0B0C] p-2 rounded-md border border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-gray-400">
            <HardDrive className="w-3.5 h-3.5 text-[#e2588a]" />
            <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">Memory</span>
          </div>
          <span className="font-mono text-xs font-semibold text-gray-200">{service.memory}%</span>
        </div>

        <div className="bg-[#0B0B0C] p-2 rounded-md border border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-gray-400">
            <Layers className="w-3.5 h-3.5 text-[#3F8E4F]" />
            <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">Replicas</span>
          </div>
          <span className="font-mono text-xs font-semibold text-gray-200">
            {service.replicas}/{service.maxReplicas}
          </span>
        </div>
      </div>

      {/* Footer bar */}
      <div className="flex items-center justify-between pt-2 border-t border-[#26262B] text-[11px] text-gray-400">
        <span className="font-sans">Throughput: <span className="font-mono font-medium text-gray-300">{service.rps} req/s</span></span>
        <div className="flex items-center space-x-1 text-[#e2588a] font-semibold hover:underline">
          <span>Details</span>
          <ChevronRight className="w-3.5 h-3.5" />
        </div>
      </div>
    </div>
  );
};
