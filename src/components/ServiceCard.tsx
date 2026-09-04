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
    healthy: 'bg-emerald-950/80 text-emerald-300 border-emerald-700',
    degraded: 'bg-amber-950/80 text-amber-300 border-amber-700',
    critical: 'bg-rose-950/80 text-rose-300 border-rose-700 animate-pulse-subtle',
  };

  const statusLabel = {
    healthy: 'HEALTHY',
    degraded: 'DEGRADED',
    critical: 'CRITICAL',
  };

  return (
    <div
      onClick={() => onSelect(service.id)}
      className={`p-4 bg-[#0F172A] border rounded-xl font-mono cursor-pointer transition-all shadow-md hover:shadow-xl ${
        isSelected
          ? 'border-sky-400 ring-2 ring-sky-400/80'
          : isCritical
          ? 'border-rose-700/80 bg-rose-950/20'
          : isDegraded
          ? 'border-amber-700/80'
          : 'border-[#1E293B] hover:border-slate-500'
      }`}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div>
          <div className="flex items-center space-x-2">
            <span
              className={`w-2.5 h-2.5 rounded-full ${
                isCritical ? 'bg-rose-500' : isDegraded ? 'bg-amber-400' : 'bg-emerald-400'
              }`}
            />
            <h3 className="font-bold text-sm text-white">{service.name}</h3>
          </div>
          <span className="text-[10px] text-slate-400 uppercase font-medium">
            Type: {service.type} | Version: {service.version}
          </span>
        </div>

        <span
          className={`text-[10px] font-bold px-2 py-0.5 rounded border ${
            statusColorMap[service.status]
          }`}
        >
          {statusLabel[service.status]}
        </span>
      </div>

      {/* Primary Telemetry Metrics */}
      <div className="grid grid-cols-2 gap-2 my-3 text-xs">
        <div className="bg-[#07090E] p-2 rounded-lg border border-[#1E293B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-slate-400">
            <Cpu className="w-3.5 h-3.5 text-sky-400" />
            <span>CPU</span>
          </div>
          <span className={`font-bold ${service.cpu > 80 ? 'text-rose-400' : 'text-slate-200'}`}>
            {service.cpu}%
          </span>
        </div>

        <div className="bg-[#07090E] p-2 rounded-lg border border-[#1E293B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-slate-400">
            <Clock className="w-3.5 h-3.5 text-amber-400" />
            <span>Latency</span>
          </div>
          <span
            className={`font-bold ${service.latency > 150 ? 'text-rose-400' : 'text-slate-200'}`}
          >
            {service.latency}ms
          </span>
        </div>

        <div className="bg-[#07090E] p-2 rounded-lg border border-[#1E293B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-slate-400">
            <HardDrive className="w-3.5 h-3.5 text-purple-400" />
            <span>Memory</span>
          </div>
          <span className="font-bold text-slate-200">{service.memory}%</span>
        </div>

        <div className="bg-[#07090E] p-2 rounded-lg border border-[#1E293B] flex items-center justify-between">
          <div className="flex items-center space-x-1.5 text-slate-400">
            <Layers className="w-3.5 h-3.5 text-emerald-400" />
            <span>Replicas</span>
          </div>
          <span className="font-bold text-slate-200">
            {service.replicas}/{service.maxReplicas}
          </span>
        </div>
      </div>

      {/* Footer bar */}
      <div className="flex items-center justify-between pt-2 border-t border-[#1E293B] text-[11px] text-slate-400">
        <span>RPS: {service.rps}</span>
        <div className="flex items-center space-x-1 text-sky-400 font-bold hover:underline">
          <span>View Details</span>
          <ChevronRight className="w-3.5 h-3.5" />
        </div>
      </div>
    </div>
  );
};
