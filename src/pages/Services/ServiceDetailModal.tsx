import React from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { MetricChart } from '../../components/MetricChart';
import { LogStreamer } from '../../components/LogStreamer';
import { X, Cpu, Clock, HardDrive, Layers, RotateCcw, PlusCircle, ArrowRight } from 'lucide-react';

interface ServiceDetailModalProps {
  serviceId: string;
  onClose: () => void;
}

export const ServiceDetailModal: React.FC<ServiceDetailModalProps> = ({
  serviceId,
  onClose,
}) => {
  const { services, logs, executeRecoveryAction } = useTelemetryStore();
  const service = services.find((s) => s.id === serviceId);

  if (!service) return null;

  const isCritical = service.status === 'critical';
  const isDegraded = service.status === 'degraded';

  const upstreamServices = services.filter((s) => s.dependencies.includes(service.id));
  const downstreamServices = services.filter((s) => service.dependencies.includes(s.id));

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 font-mono">
      <div className="bg-[#111827] border border-[#1F2937] rounded-lg max-w-4xl w-full max-h-[90vh] overflow-hidden shadow-2xl flex flex-col font-mono">
        {/* Header */}
        <div className="bg-[#0B0F19] px-6 py-4 border-b border-[#1F2937] flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <span
              className={`w-3 h-3 rounded-full ${
                isCritical
                  ? 'bg-red-500'
                  : isDegraded
                  ? 'bg-amber-500'
                  : 'bg-emerald-500'
              }`}
            />
            <div>
              <h2 className="font-bold text-gray-100 text-base flex items-center space-x-2">
                <span>{service.name}</span>
                <span className="text-xs text-gray-400 font-normal">
                  ({service.id})
                </span>
              </h2>
              <p className="text-[11px] text-gray-400">
                Type: {service.type.toUpperCase()} | Version: {service.version} | Uptime: {service.uptime}
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            <span
              className={`text-xs font-bold px-2.5 py-1 rounded border uppercase ${
                isCritical
                  ? 'bg-red-950 text-red-400 border-red-800'
                  : isDegraded
                  ? 'bg-amber-950 text-amber-400 border-amber-800'
                  : 'bg-emerald-950 text-emerald-400 border-emerald-800'
              }`}
            >
              {service.status}
            </span>

            <button onClick={onClose} className="text-gray-400 hover:text-white">
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Modal Scrollable Body */}
        <div className="p-6 overflow-y-auto space-y-6">
          {/* Quick Metrics Bar */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
            <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Cpu className="w-3.5 h-3.5 text-sky-400" />
                <span>CPU Core Load</span>
              </div>
              <span className={`font-bold text-base ${service.cpu > 80 ? 'text-red-400' : 'text-gray-100'}`}>
                {service.cpu}%
              </span>
            </div>

            <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Clock className="w-3.5 h-3.5 text-amber-400" />
                <span>Latency (p95)</span>
              </div>
              <span className={`font-bold text-base ${service.latency > 150 ? 'text-red-400' : 'text-gray-100'}`}>
                {service.latency}ms
              </span>
            </div>

            <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <HardDrive className="w-3.5 h-3.5 text-purple-400" />
                <span>Memory</span>
              </div>
              <span className="font-bold text-base text-gray-100">
                {service.memory}%
              </span>
            </div>

            <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Layers className="w-3.5 h-3.5 text-emerald-400" />
                <span>Pod Replicas</span>
              </div>
              <span className="font-bold text-base text-gray-100">
                {service.replicas} / {service.maxReplicas}
              </span>
            </div>
          </div>

          {/* Detailed Metric Charts */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <MetricChart
              title={`${service.name} CPU Utilization`}
              data={service.history}
              dataKey="cpu"
              lineColor="#0284C7"
              unit="%"
            />

            <MetricChart
              title={`${service.name} Latency`}
              data={service.history}
              dataKey="latency"
              lineColor="#D97706"
              unit="ms"
            />

            <MetricChart
              title={`${service.name} Memory Footprint`}
              data={service.history}
              dataKey="memory"
              lineColor="#7C3AED"
              unit="%"
            />

            <MetricChart
              title={`${service.name} Queue Depth`}
              data={service.history}
              dataKey="queueDepth"
              lineColor="#0891B2"
              unit="items"
            />
          </div>

          {/* Dependency Topology Flow */}
          <div className="bg-[#0B0F19] p-4 rounded border border-[#1F2937]">
            <h4 className="text-xs font-bold text-gray-300 uppercase tracking-wide mb-3">
              Upstream and Downstream Dependencies
            </h4>

            <div className="flex flex-wrap items-center gap-3 text-xs">
              <div className="flex items-center space-x-2">
                <span className="text-gray-500">Callers (Upstream):</span>
                {upstreamServices.length === 0 ? (
                  <span className="text-gray-400 italic">None (Ingress)</span>
                ) : (
                  upstreamServices.map((u) => (
                    <span
                      key={u.id}
                      className="px-2 py-0.5 bg-[#111827] border border-[#1F2937] rounded text-sky-400"
                    >
                      {u.name}
                    </span>
                  ))
                )}
              </div>

              <ArrowRight className="w-4 h-4 text-gray-500 shrink-0" />

              <div className="px-3 py-1 bg-sky-950 text-sky-300 border border-sky-800 rounded font-bold">
                {service.name}
              </div>

              <ArrowRight className="w-4 h-4 text-gray-500 shrink-0" />

              <div className="flex items-center space-x-2">
                <span className="text-gray-500">Targets (Downstream):</span>
                {downstreamServices.length === 0 ? (
                  <span className="text-gray-400 italic">None (Leaf Storage)</span>
                ) : (
                  downstreamServices.map((d) => (
                    <span
                      key={d.id}
                      className="px-2 py-0.5 bg-[#111827] border border-[#1F2937] rounded text-purple-400"
                    >
                      {d.name}
                    </span>
                  ))
                )}
              </div>
            </div>
          </div>

          {/* Service Log Stream */}
          <div className="space-y-2">
            <h4 className="text-xs font-bold text-gray-300 uppercase tracking-wide">
              Isolated Logs for {service.name}
            </h4>
            <LogStreamer logs={logs} serviceFilter={service.id} maxHeight="max-h-48" />
          </div>
        </div>

        {/* Action Controls Footer */}
        <div className="bg-[#0B0F19] px-6 py-4 border-t border-[#1F2937] flex flex-wrap items-center justify-between gap-3">
          <span className="text-xs text-gray-400">
            Microservice Control Operations
          </span>

          <div className="flex items-center space-x-3">
            <button
              onClick={() => {
                executeRecoveryAction('restart', service.id, `Restarted ${service.name} Pods`);
              }}
              className="flex items-center space-x-1.5 px-3 py-1.5 bg-amber-950 hover:bg-amber-900 text-amber-300 border border-amber-800 rounded text-xs font-bold transition-colors"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              <span>RESTART PODS</span>
            </button>

            <button
              onClick={() => {
                executeRecoveryAction('scale', service.id, `Scaled ${service.name} Replicas +3`);
              }}
              className="flex items-center space-x-1.5 px-3 py-1.5 bg-sky-900 hover:bg-sky-800 text-white border border-sky-700 rounded text-xs font-bold transition-colors"
            >
              <PlusCircle className="w-3.5 h-3.5" />
              <span>SCALE REPLICAS (+3)</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
