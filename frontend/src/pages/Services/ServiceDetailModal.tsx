import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { recoveryApi } from '../../api/recovery';
import { MetricChart } from '../../components/MetricChart';
import { LogStreamer } from '../../components/LogStreamer';
import { X, Cpu, Clock, HardDrive, Layers, RotateCcw, PlusCircle, ArrowRight, Database, Zap, Loader2, CheckCircle2 } from 'lucide-react';

interface ServiceDetailModalProps {
  serviceId: string;
  onClose: () => void;
}

export const ServiceDetailModal: React.FC<ServiceDetailModalProps> = ({
  serviceId,
  onClose,
}) => {
  const { services, logs, executeRecoveryAction, latestPrediction } = useTelemetryStore();
  const service = services.find((s) => s.id === serviceId);

  const [executing, setExecuting] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const handleAction = async (actionType: string, title: string) => {
    if (!service) return;
    setExecuting(actionType);
    setMessage(null);
    try {
      // Unify manual button trigger with the authoritative backend policy check
      const policy = await recoveryApi.checkPolicy(actionType, service.id);
      if (policy.cooldownRemainingSec > 0) {
        setMessage({
          type: 'error',
          text: `Safety Cooldown Active: ${service.name} was recently remediated. Wait ${policy.cooldownRemainingSec}s before retrying.`,
        });
        return;
      }

      await executeRecoveryAction(actionType, service.id, title, true);
      setMessage({
        type: 'success',
        text: `Dispatched ${title} [Policy: ${policy.riskLevel} (${Math.round(policy.riskScore * 100)}% risk, blast radius ${policy.blastRadiusSize} nodes)]. Verification window active.`,
      });
    } catch (err: unknown) {
      const errObj = err as { message?: string };
      setMessage({ type: 'error', text: errObj?.message || 'Failed to execute recovery action.' });
    } finally {
      setTimeout(() => setExecuting(null), 1200);
    }
  };

  if (!service) return null;

  const isCritical = service.status === 'critical';
  const isDegraded = service.status === 'degraded';

  const upstreamServices = services.filter((s) => s.dependencies.includes(service.id));
  const downstreamServices = services.filter((s) => service.dependencies.includes(s.id));

  // Check if Python Decision Engine prescribed an action for this specific workload
  const matchingRec =
    latestPrediction?.recommendedActions?.find((a) => a.targetServiceId === service.id) ||
    (latestPrediction?.rootCauseServiceId === service.id && latestPrediction?.recommendedActionId !== 'act-none'
      ? {
          title: latestPrediction.recommendedAction,
          description: latestPrediction.rootCauseReason,
          actionType: latestPrediction.recommendedActionId?.replace('act-', '') || 'scale',
          riskLevel: 'LOW' as const,
        }
      : null);

  return (
    <div className="fixed inset-0 bg-black/85 backdrop-blur-sm z-50 flex items-center justify-center p-4 font-sans">
      <div className="bg-[#151517] border border-[#26262B] rounded-[10px] max-w-4xl w-full max-h-[90vh] overflow-hidden shadow-2xl flex flex-col">
        {/* Header */}
        <div className="bg-[#0B0B0C] px-5 py-3.5 border-b border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <span
              className={`w-2 h-2 rounded-full ${
                isCritical
                  ? 'bg-[#AD2831]'
                  : isDegraded
                  ? 'bg-[#D4A017]'
                  : 'bg-[#3F8E4F]'
              }`}
            />
            <div>
              <h2 className="font-mono font-semibold text-white text-[16px] leading-[24px] flex items-center space-x-2">
                <span>{service.name}</span>
                <span className="text-xs text-gray-400 font-normal">
                  ({service.id})
                </span>
              </h2>
              <p className="text-[11px] text-gray-400 font-normal">
                Type: <span className="font-mono text-gray-300">{service.type.toUpperCase()}</span> | Ver: <span className="font-mono text-gray-300">{service.version}</span> | Uptime: <span className="font-mono text-gray-300">{service.uptime}</span>
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            <span
              className={`text-[10px] font-semibold px-2 py-0.5 rounded border uppercase tracking-[0.04em] ${
                isCritical
                  ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60'
                  : isDegraded
                  ? 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/60'
                  : 'bg-[#3F8E4F]/15 text-[#52b767] border-[#3F8E4F]/60'
              }`}
            >
              {service.status === 'degraded' ? 'WARNING' : service.status.toUpperCase()}
            </span>

            <button onClick={onClose} className="text-gray-400 hover:text-white">
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Modal Scrollable Body */}
        <div className="p-5 overflow-y-auto space-y-4">
          {/* Quick Metrics Bar */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
            <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Cpu className="w-3.5 h-3.5 text-gray-400" />
                <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">CPU LOAD</span>
              </div>
              <span className={`font-mono text-[16px] font-semibold ${service.cpu > 80 ? 'text-[#AD2831]' : 'text-gray-100'}`}>
                {Number(service.cpu || 0).toFixed(1)}%
              </span>
            </div>

            <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Clock className="w-3.5 h-3.5 text-[#D4A017]" />
                <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">LATENCY (P95)</span>
              </div>
              <span className={`font-mono text-[16px] font-semibold ${service.latency > 150 ? 'text-[#AD2831]' : 'text-gray-100'}`}>
                {Number(service.latency || 0).toFixed(0)}ms
              </span>
            </div>

            <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <HardDrive className="w-3.5 h-3.5 text-[#e2588a]" />
                <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">MEMORY</span>
              </div>
              <span className="font-mono text-[16px] font-semibold text-gray-100">
                {Number(service.memory || 0).toFixed(1)}%
              </span>
            </div>

            <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B]">
              <div className="flex items-center space-x-1.5 text-gray-400 mb-1">
                <Layers className="w-3.5 h-3.5 text-[#3F8E4F]" />
                <span className="text-[10px] font-semibold tracking-[0.06em] uppercase">POD REPLICAS</span>
              </div>
              <span className="font-mono text-[16px] font-semibold text-gray-100">
                {service.replicas} / {service.maxReplicas}
              </span>
            </div>
          </div>

          {/* Detailed Metric Charts */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <MetricChart
              title={`${service.name} CPU`}
              data={service.history}
              dataKey="cpu"
              lineColor="#AD2831"
              unit="%"
            />

            <MetricChart
              title={`${service.name} Latency`}
              data={service.history}
              dataKey="latency"
              lineColor="#D4A017"
              unit="ms"
            />

            <MetricChart
              title={`${service.name} Memory`}
              data={service.history}
              dataKey="memory"
              lineColor="#89023E"
              unit="%"
            />

            <MetricChart
              title={`${service.name} Queue Depth`}
              data={service.history}
              dataKey="queueDepth"
              lineColor="#3F8E4F"
              unit="items"
            />
          </div>

          {/* Dependency Topology Flow */}
          <div className="bg-[#0B0B0C] p-3.5 rounded-lg border border-[#26262B]">
            <h4 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] mb-2.5">
              DEPENDENCY LINKAGES
            </h4>

            <div className="flex flex-wrap items-center gap-2.5 text-xs">
              <div className="flex items-center space-x-1.5">
                <span className="text-[11px] text-gray-400">Callers:</span>
                {upstreamServices.length === 0 ? (
                  <span className="text-gray-500 italic text-[11px]">None (Ingress)</span>
                ) : (
                  upstreamServices.map((u) => (
                    <span
                      key={u.id}
                      className="px-2 py-0.5 bg-[#151517] border border-[#26262B] rounded font-mono text-[11px] text-gray-200"
                    >
                      {u.name}
                    </span>
                  ))
                )}
              </div>

              <ArrowRight className="w-3.5 h-3.5 text-gray-500 shrink-0" />

              <div className="px-2.5 py-0.5 bg-[#840032] text-white border border-[#840032] rounded font-mono text-[11px] font-semibold">
                {service.name}
              </div>

              <ArrowRight className="w-3.5 h-3.5 text-gray-500 shrink-0" />

              <div className="flex items-center space-x-1.5">
                <span className="text-[11px] text-gray-400">Targets:</span>
                {downstreamServices.length === 0 ? (
                  <span className="text-gray-500 italic text-[11px]">None (Leaf)</span>
                ) : (
                  downstreamServices.map((d) => (
                    <span
                      key={d.id}
                      className="px-2 py-0.5 bg-[#151517] border border-[#26262B] rounded font-mono text-[11px] text-gray-200"
                    >
                      {d.name}
                    </span>
                  ))
                )}
              </div>
            </div>
          </div>

          {/* Service Log Stream */}
          <div className="space-y-1.5">
            <h4 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em]">
              ISOLATED LOGS ({service.name})
            </h4>
            <LogStreamer logs={logs} serviceFilter={service.id} maxHeight="max-h-48" />
          </div>
        </div>

        {/* Action Controls Footer */}
        <div className="bg-[#0B0B0C] px-5 py-3 border-t border-[#26262B] flex flex-wrap items-center justify-between gap-3">
          <div>
            <span className="text-[11px] text-gray-300 font-semibold uppercase tracking-[0.05em] block">
              Workload Remediation Controls
            </span>
            <span className="text-[10px] text-gray-500 font-mono">
              Type: {service.type.toUpperCase()} | Status: {service.status.toUpperCase()}
            </span>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {matchingRec && (
              <button
                onClick={() => handleAction(matchingRec.actionType, matchingRec.title)}
                disabled={executing !== null}
                className="flex items-center space-x-1.5 px-3 py-1.5 bg-[#840032] hover:bg-[#9b053d] text-white border border-[#840032] rounded-md text-xs font-semibold transition-all shadow-sm ring-1 ring-[#e2588a]/40 disabled:opacity-50"
              >
                {executing === matchingRec.actionType ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin text-amber-300" />
                ) : (
                  <Zap className="w-3.5 h-3.5 text-amber-300" />
                )}
                <span className="uppercase tracking-[0.03em]">AI PRESCRIBED: {matchingRec.title}</span>
              </button>
            )}

            <button
              onClick={() => handleAction('restart', `Restarted ${service.name} Pods`)}
              disabled={executing !== null}
              className="flex items-center space-x-1 px-2.5 py-1.5 bg-[#151517] hover:bg-[#1f1f23] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors disabled:opacity-50"
            >
              {executing === 'restart' ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-[#e5b533]" />
              ) : (
                <RotateCcw className="w-3.5 h-3.5 text-[#e5b533]" />
              )}
              <span>RESTART</span>
            </button>

            <button
              onClick={() => handleAction('scale', `Scaled ${service.name} Replicas +2`)}
              disabled={executing !== null}
              className="flex items-center space-x-1 px-2.5 py-1.5 bg-[#151517] hover:bg-[#1f1f23] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors disabled:opacity-50"
            >
              {executing === 'scale' ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-[#f07b9e]" />
              ) : (
                <PlusCircle className="w-3.5 h-3.5 text-[#f07b9e]" />
              )}
              <span>SCALE (+2)</span>
            </button>

            <button
              onClick={() => handleAction('increase_pool', `Expand Pool for ${service.name}`)}
              disabled={executing !== null}
              className="flex items-center space-x-1 px-2.5 py-1.5 bg-[#151517] hover:bg-[#1f1f23] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors disabled:opacity-50"
            >
              {executing === 'increase_pool' ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-[#e2588a]" />
              ) : (
                <Database className="w-3.5 h-3.5 text-[#e2588a]" />
              )}
              <span>EXPAND POOL (+10)</span>
            </button>

            <button
              onClick={() => handleAction('flush_cache', `Flush Cache on ${service.name}`)}
              disabled={executing !== null}
              className="flex items-center space-x-1 px-2.5 py-1.5 bg-[#151517] hover:bg-[#1f1f23] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors disabled:opacity-50"
            >
              {executing === 'flush_cache' ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-[#52b767]" />
              ) : (
                <Zap className="w-3.5 h-3.5 text-[#52b767]" />
              )}
              <span>FLUSH CACHE</span>
            </button>

            <button
              onClick={() => handleAction('reroute_traffic', `Reroute Ingress Traffic`)}
              disabled={executing !== null}
              className="flex items-center space-x-1 px-2.5 py-1.5 bg-[#151517] hover:bg-[#1f1f23] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-colors disabled:opacity-50"
            >
              {executing === 'reroute_traffic' ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-amber-300" />
              ) : (
                <ArrowRight className="w-3.5 h-3.5 text-amber-300" />
              )}
              <span>REROUTE TRAFFIC</span>
            </button>
          </div>

          {/* Feedback Toast Banner */}
          {message && (
            <div
              className={`mt-2.5 p-2 rounded-md text-xs flex items-center space-x-1.5 font-sans border ${
                message.type === 'success'
                  ? 'bg-[#3F8E4F]/20 text-[#86efac] border-[#3F8E4F]/60'
                  : 'bg-[#AD2831]/20 text-[#fca5a5] border-[#AD2831]/60'
              }`}
            >
              {message.type === 'success' ? (
                <CheckCircle2 className="w-3.5 h-3.5 text-[#52b767] shrink-0" />
              ) : (
                <X className="w-3.5 h-3.5 text-[#f87171] shrink-0" />
              )}
              <span>{message.text}</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
