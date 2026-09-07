import React from 'react';
import { useTelemetryStore, getMLPrediction } from '../../store/useTelemetryStore';
import { RecommendationCard } from '../../components/RecommendationCard';
import { BrainCircuit, ShieldAlert, Clock, ArrowRight, Layers, CheckCircle2, Zap } from 'lucide-react';

export const Predictions: React.FC = () => {
  const { services, activeFaults, executeRecoveryAction } = useTelemetryStore();
  const prediction = getMLPrediction(services, activeFaults);

  const isCriticalProb = prediction.failureProbability > 70;
  const isDegradedProb = prediction.failureProbability > 30 && prediction.failureProbability <= 70;

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="border-b border-[#26262B] pb-4">
        <div className="flex items-center space-x-2">
          <BrainCircuit className="w-5 h-5 text-[#e2588a]" />
          <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
            Predictive Analytics & Root Cause Engine
          </h1>
        </div>
        <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
          Temporal Graph Neural Network (TGNN) failure forecasting, root cause isolation, and propagation cascade
        </p>
      </div>

      {/* Top 4 Prediction Gauges */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              FAILURE PROBABILITY
            </span>
            <div className="p-1.5 bg-[#840032] text-white border border-[#a80f49] rounded-md">
              <BrainCircuit className="w-3.5 h-3.5" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span
              className={`text-[28px] font-semibold tracking-[-0.03em] leading-[32px] font-mono ${
                isCriticalProb
                  ? 'text-[#AD2831]'
                  : isDegradedProb
                  ? 'text-[#D4A017]'
                  : 'text-[#3F8E4F]'
              }`}
            >
              {prediction.failureProbability}%
            </span>
            <span className="text-[10px] font-semibold tracking-[0.04em] uppercase text-gray-400">
              {isCriticalProb ? 'CRITICAL' : isDegradedProb ? 'WARNING' : 'HEALTHY'}
            </span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              TGNN CONFIDENCE
            </span>
            <div className="p-1.5 bg-[#3F8E4F]/20 text-[#52b767] border border-[#3F8E4F]/50 rounded-md">
              <CheckCircle2 className="w-3.5 h-3.5" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-[28px] font-semibold tracking-[-0.03em] leading-[32px] font-mono text-[#3F8E4F]">
              {prediction.confidence}%
            </span>
            <span className="text-[10px] font-semibold tracking-[0.04em] uppercase text-gray-400">CLASSIFIER</span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              ESTIMATED TIME TO FAILURE
            </span>
            <div className="p-1.5 bg-[#D4A017]/20 text-[#e5b533] border border-[#D4A017]/50 rounded-md">
              <Clock className="w-3.5 h-3.5" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-[28px] font-semibold tracking-[-0.03em] leading-[32px] font-mono text-[#D4A017]">
              {prediction.estimatedFailureSec}s
            </span>
            <span className="text-[10px] font-semibold tracking-[0.04em] uppercase text-gray-400">HORIZON</span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              ROOT CAUSE ENTITY
            </span>
            <div className="p-1.5 bg-[#AD2831]/20 text-[#f87171] border border-[#AD2831]/60 rounded-md">
              <ShieldAlert className="w-3.5 h-3.5" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="font-mono text-[16px] font-semibold leading-[24px] text-[#AD2831] truncate">
              {prediction.rootCauseServiceName}
            </span>
          </div>
        </div>
      </div>

      {/* Main Grid: Blast Radius Tree & Recommended Actions */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Blast Radius Section */}
        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg space-y-3">
          <div className="flex items-center space-x-2 border-b border-[#26262B] pb-2.5">
            <Layers className="w-4 h-4 text-[#e2588a]" />
            <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
              PROPAGATION PATH & BLAST RADIUS
            </h3>
          </div>

          {prediction.blastRadius.length === 0 ? (
            <div className="p-6 bg-[#0B0B0C] rounded-lg border border-[#26262B] text-center text-gray-500 text-xs">
              No cascading blast radius detected. All downstream nodes nominal.
            </div>
          ) : (
            <div className="space-y-3">
              <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
                Primary failure origin propagating upstream through dependency chain:
              </p>

              <div className="flex flex-wrap items-center gap-2 p-3 bg-[#0B0B0C] rounded-lg border border-[#26262B]">
                {prediction.blastRadius.map((srvId, idx) => {
                  const srv = services.find((s) => s.id === srvId);
                  const isOrigin = idx === 0;

                  return (
                    <React.Fragment key={srvId}>
                      <div
                        className={`px-2.5 py-1.5 rounded-md text-xs font-semibold flex items-center space-x-2 border ${
                          isOrigin
                            ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]'
                            : 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/70'
                        }`}
                      >
                        <span className="font-mono text-[12px]">{srv ? srv.name : srvId}</span>
                        {isOrigin && (
                          <span className="text-[9px] bg-[#AD2831] text-white px-1.5 py-0.5 rounded font-sans font-semibold uppercase">
                            ORIGIN
                          </span>
                        )}
                      </div>

                      {idx < prediction.blastRadius.length - 1 && (
                        <ArrowRight className="w-3.5 h-3.5 text-gray-500 shrink-0" />
                      )}
                    </React.Fragment>
                  );
                })}
              </div>

              <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B] text-xs text-gray-300 space-y-1">
                <span className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase">
                  ROOT CAUSE DIAGNOSTICS:
                </span>
                <p className="text-[12px] font-semibold text-[#AD2831]">
                  {prediction.rootCauseReason}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Prescriptive Remediation Actions */}
        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg space-y-3">
          <div className="flex items-center space-x-2 border-b border-[#26262B] pb-2.5">
            <Zap className="w-4 h-4 text-[#e2588a]" />
            <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
              RECOVERY DECISION & EXECUTION
            </h3>
          </div>

          <RecommendationCard
            title={prediction.recommendedAction}
            description="Executes container scaling and database pool expansion to relieve upstream bottleneck."
            targetService={prediction.rootCauseServiceName}
            confidence={prediction.confidence}
            onExecute={() => {
              executeRecoveryAction(
                'scale',
                prediction.rootCauseServiceId,
                prediction.recommendedAction
              );
            }}
          />
        </div>
      </div>
    </div>
  );
};
