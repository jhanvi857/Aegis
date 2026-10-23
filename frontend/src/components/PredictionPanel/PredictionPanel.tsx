import React, { useState } from 'react';
import { useTelemetryStore, getMLPrediction } from '../../store/useTelemetryStore';
import {
  BrainCircuit,
  ArrowRight,
  Zap,
} from 'lucide-react';

interface PredictionPanelProps {
  compact?: boolean;
  onExecutePlan?: (actionType: string, serviceId: string) => void;
  className?: string;
}

export const PredictionPanel: React.FC<PredictionPanelProps> = ({
  compact: _compact = false,
  onExecutePlan,
  className = '',
}) => {
  const { services, activeFaults, executeRecoveryAction } = useTelemetryStore();
  const prediction = getMLPrediction(services, activeFaults);

  const [isExecuting, setIsExecuting] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  const isCriticalProb = prediction.failureProbability > 70;
  const isDegradedProb = prediction.failureProbability > 30 && prediction.failureProbability <= 70;

  const rootService = services.find((s) => s.id === prediction.rootCauseServiceId);

  const handleRemediate = async () => {
    if (!prediction.rootCauseServiceId || prediction.rootCauseServiceId === 'None') return;

    // Extract action type from recommendation string or default
    let actionType = 'restart';
    const recLower = (prediction.recommendedAction || '').toLowerCase();
    if (recLower.includes('scale')) actionType = 'scale';
    else if (recLower.includes('flush') || recLower.includes('cache')) actionType = 'flush_cache';
    else if (recLower.includes('pool')) actionType = 'increase_pool';
    else if (recLower.includes('reroute')) actionType = 'reroute_traffic';
    else if (recLower.includes('restart')) actionType = 'restart';

    setIsExecuting(true);
    setFeedback(null);
    try {
      if (onExecutePlan) {
        onExecutePlan(actionType, prediction.rootCauseServiceId);
      } else {
        await executeRecoveryAction(
          actionType,
          prediction.rootCauseServiceId,
          prediction.recommendedActionId || `act-${actionType}-${prediction.rootCauseServiceId}`,
          true
        );
      }
      setFeedback('Remediation dispatched! Entering verification window...');
      setTimeout(() => setFeedback(null), 4000);
    } catch (err: any) {
      setFeedback(`Execution halted: ${err?.message || err}`);
    } finally {
      setIsExecuting(false);
    }
  };

  return (
    <div className={`bg-[#0F0F11] border border-[#26262B] rounded-xl overflow-hidden flex flex-col font-sans ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-[#151517] border-b border-[#26262B]">
        <div className="flex items-center space-x-2.5">
          <div className="p-1.5 bg-[#840032]/20 border border-[#840032] rounded-md text-[#e2588a]">
            <BrainCircuit className="w-4 h-4" />
          </div>
          <div>
            <span className="text-xs font-semibold text-white tracking-wide">
              TGNN PREDICTION & ROOT CAUSE ENGINE
            </span>
            <span className="text-[11px] text-gray-400 ml-2 font-mono">
              Horizon: {prediction.estimatedFailureSec}s
            </span>
          </div>
        </div>

        <span
          className={`text-[10px] font-mono font-semibold px-2 py-0.5 rounded border uppercase ${
            isCriticalProb
              ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60'
              : isDegradedProb
              ? 'bg-[#D4A017]/20 text-[#e5b533] border-[#D4A017]/60'
              : 'bg-[#3F8E4F]/20 text-[#52b767] border-[#3F8E4F]/60'
          }`}
        >
          {isCriticalProb ? 'CRITICAL RISK' : isDegradedProb ? 'ELEVATED' : 'NOMINAL'}
        </span>
      </div>

      <div className="p-4 space-y-4">
        {/* Top 3 Metric Gauges */}
        <div className="grid grid-cols-3 gap-2">
          {/* Failure Prob */}
          <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg">
            <div className="text-[10px] font-medium text-gray-400 uppercase tracking-wider mb-1">
              Failure Prob
            </div>
            <div className="flex items-baseline space-x-1.5">
              <span
                className={`text-2xl font-bold font-mono ${
                  isCriticalProb
                    ? 'text-[#AD2831]'
                    : isDegradedProb
                    ? 'text-[#D4A017]'
                    : 'text-[#3F8E4F]'
                }`}
              >
                {prediction.failureProbability}%
              </span>
            </div>
          </div>

          {/* Model Confidence */}
          <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg">
            <div className="text-[10px] font-medium text-gray-400 uppercase tracking-wider mb-1">
              Confidence
            </div>
            <div className="text-2xl font-bold font-mono text-[#3F8E4F]">
              {prediction.confidence}%
            </div>
          </div>

          {/* Time to Failure */}
          <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg">
            <div className="text-[10px] font-medium text-gray-400 uppercase tracking-wider mb-1">
              Horizon
            </div>
            <div className="text-2xl font-bold font-mono text-[#D4A017]">
              {prediction.estimatedFailureSec}s
            </div>
          </div>
        </div>

        {/* Root Cause Entity Details */}
        <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold tracking-wider text-gray-400 uppercase">
              ISOLATED ROOT CAUSE
            </span>
            {rootService && (
              <span
                className={`text-[10px] font-mono px-2 py-0.5 rounded uppercase font-semibold ${
                  rootService.status === 'critical'
                    ? 'bg-[#AD2831]/20 text-[#f87171] border border-[#AD2831]'
                    : 'bg-[#D4A017]/20 text-[#e5b533] border border-[#D4A017]'
                }`}
              >
                {rootService.status}
              </span>
            )}
          </div>

          <div className="flex items-center space-x-2">
            <div className="font-mono text-sm font-bold text-white">
              {prediction.rootCauseServiceName || prediction.rootCauseServiceId || 'None'}
            </div>
            {prediction.rootCauseServiceId && (
              <span className="text-xs text-gray-400 font-mono">
                ({prediction.rootCauseServiceId})
              </span>
            )}
          </div>

          <p className="text-xs text-gray-300 leading-relaxed">
            {prediction.rootCauseReason}
          </p>

          {/* Root Node Telemetry Evidence */}
          {rootService && (
            <div className="grid grid-cols-4 gap-1.5 pt-2 border-t border-[#26262B] text-[11px]">
              <div className="p-1.5 bg-[#0B0B0C] rounded border border-[#26262B]">
                <div className="text-gray-400 text-[10px]">CPU</div>
                <div className={`font-mono font-semibold ${rootService.cpu > 80 ? 'text-[#f87171]' : 'text-gray-200'}`}>
                  {Number(rootService.cpu || 0).toFixed(0)}%
                </div>
              </div>

              <div className="p-1.5 bg-[#0B0B0C] rounded border border-[#26262B]">
                <div className="text-gray-400 text-[10px]">Memory</div>
                <div className={`font-mono font-semibold ${rootService.memory > 80 ? 'text-[#f87171]' : 'text-gray-200'}`}>
                  {Number(rootService.memory || 0).toFixed(0)}%
                </div>
              </div>

              <div className="p-1.5 bg-[#0B0B0C] rounded border border-[#26262B]">
                <div className="text-gray-400 text-[10px]">Latency</div>
                <div className={`font-mono font-semibold ${rootService.latency > 150 ? 'text-[#f87171]' : 'text-gray-200'}`}>
                  {Number(rootService.latency || 0).toFixed(0)}ms
                </div>
              </div>

              <div className="p-1.5 bg-[#0B0B0C] rounded border border-[#26262B]">
                <div className="text-gray-400 text-[10px]">Error Rate</div>
                <div className={`font-mono font-semibold ${rootService.errorRate > 0.05 ? 'text-[#f87171]' : 'text-gray-200'}`}>
                  {(Number(rootService.errorRate || 0) * 100).toFixed(1)}%
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Cascade Propagation Breadcrumbs */}
        {prediction.blastRadius && prediction.blastRadius.length > 0 && (
          <div className="p-3 bg-[#151517] border border-[#26262B] rounded-lg space-y-2">
            <span className="text-[10px] font-semibold tracking-wider text-gray-400 uppercase">
              PROPAGATION CASCADE PATH ({prediction.blastRadius.length} AFFECTED NODES)
            </span>
            <div className="flex flex-wrap items-center gap-1.5 pt-1">
              {prediction.blastRadius.map((nodeId, idx) => (
                <React.Fragment key={nodeId}>
                  <span className="px-2 py-0.5 bg-purple-950/60 border border-purple-800 text-purple-200 rounded font-mono text-[11px] font-semibold">
                    {nodeId}
                  </span>
                  {idx < prediction.blastRadius.length - 1 && (
                    <ArrowRight className="w-3 h-3 text-purple-400 shrink-0" />
                  )}
                </React.Fragment>
              ))}
            </div>
          </div>
        )}

        {/* Decision Engine Recommended Remediation */}
        <div className="p-3 bg-[#840032]/10 border border-[#840032]/50 rounded-lg space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold tracking-wider text-[#f07b9e] uppercase">
              DECISION ENGINE REMEDIATION
            </span>
            <span className="text-[10px] font-mono bg-[#840032] text-white px-2 py-0.5 rounded font-semibold">
              RULEBOOK MITIGATION
            </span>
          </div>

          <div className="text-xs font-semibold text-white">
            {prediction.recommendedAction}
          </div>

          {feedback && (
            <div className="text-[11px] text-emerald-400 font-mono bg-emerald-950/40 border border-emerald-800/60 p-2 rounded">
              {feedback}
            </div>
          )}

          {prediction.rootCauseServiceId && prediction.rootCauseServiceId !== 'None' && (
            <div className="pt-2">
              <button
                onClick={handleRemediate}
                disabled={isExecuting}
                className="w-full flex items-center justify-center space-x-2 py-2 px-4 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors shadow-sm"
              >
                <Zap className="w-3.5 h-3.5 fill-white" />
                <span>{isExecuting ? 'DISPATCHING REMEDIATION...' : 'EXECUTE RECOMMENDED REMEDIATION'}</span>
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
