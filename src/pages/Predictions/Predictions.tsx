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
    <div className="p-6 space-y-6 font-mono">
      {/* Header */}
      <div className="border-b border-[#1F2937] pb-4">
        <div className="flex items-center space-x-2">
          <BrainCircuit className="w-5 h-5 text-purple-400" />
          <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
            AIOps Predictive Analytics & Root Cause Engine
          </h1>
        </div>
        <p className="text-xs text-gray-400">
          Machine learning failure forecasting model with real-time root cause isolation & blast radius mapping
        </p>
      </div>

      {/* Top 4 Prediction Gauges */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-gray-400 font-medium uppercase">
              Failure Probability
            </span>
            <div className="p-1.5 bg-purple-950 text-purple-400 border border-purple-800 rounded">
              <BrainCircuit className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span
              className={`text-3xl font-bold ${
                isCriticalProb
                  ? 'text-red-400'
                  : isDegradedProb
                  ? 'text-amber-400'
                  : 'text-emerald-400'
              }`}
            >
              {prediction.failureProbability}%
            </span>
            <span className="text-xs text-gray-500 font-bold">
              {isCriticalProb ? 'CRITICAL RISK' : isDegradedProb ? 'ELEVATED' : 'NOMINAL'}
            </span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-gray-400 font-medium uppercase">
              ML Model Confidence
            </span>
            <div className="p-1.5 bg-sky-950 text-sky-400 border border-sky-800 rounded">
              <CheckCircle2 className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-3xl font-bold text-sky-400">
              {prediction.confidence}%
            </span>
            <span className="text-xs text-gray-500">XGBoost Classifier</span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-gray-400 font-medium uppercase">
              Estimated Time to Failure
            </span>
            <div className="p-1.5 bg-amber-950 text-amber-400 border border-amber-800 rounded">
              <Clock className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-3xl font-bold text-amber-400">
              {prediction.estimatedFailureSec}s
            </span>
            <span className="text-xs text-gray-500">Prediction Horizon</span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-gray-400 font-medium uppercase">
              Isolated Root Cause
            </span>
            <div className="p-1.5 bg-red-950 text-red-400 border border-red-800 rounded">
              <ShieldAlert className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-xl font-bold text-red-400 truncate">
              {prediction.rootCauseServiceName}
            </span>
          </div>
        </div>
      </div>

      {/* Main Grid: Blast Radius Tree & Recommended Actions */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Blast Radius Section */}
        <div className="p-5 bg-[#111827] border border-[#1F2937] rounded-lg space-y-4">
          <div className="flex items-center space-x-2 border-b border-[#1F2937] pb-3">
            <Layers className="w-4 h-4 text-sky-400" />
            <h3 className="text-xs font-bold text-gray-200 uppercase tracking-wide">
              Failure Blast Radius & Cascade Hierarchy
            </h3>
          </div>

          {prediction.blastRadius.length === 0 ? (
            <div className="p-6 bg-[#0B0F19] rounded border border-[#1F2937] text-center text-gray-500 text-xs">
              No cascading blast radius detected. All downstream nodes nominal.
            </div>
          ) : (
            <div className="space-y-3">
              <p className="text-xs text-gray-400">
                Primary failure origin propagating upstream through dependency chain:
              </p>

              <div className="flex flex-wrap items-center gap-2 p-4 bg-[#0B0F19] rounded border border-[#1F2937]">
                {prediction.blastRadius.map((srvId, idx) => {
                  const srv = services.find((s) => s.id === srvId);
                  const isOrigin = idx === 0;

                  return (
                    <React.Fragment key={srvId}>
                      <div
                        className={`px-3 py-2 rounded text-xs font-bold flex items-center space-x-2 border ${
                          isOrigin
                            ? 'bg-red-950 text-red-300 border-red-800'
                            : 'bg-amber-950/60 text-amber-300 border-amber-800/80'
                        }`}
                      >
                        <span>{srv ? srv.name : srvId}</span>
                        {isOrigin && (
                          <span className="text-[9px] bg-red-800 text-white px-1.5 py-0.5 rounded font-mono">
                            ORIGIN
                          </span>
                        )}
                      </div>

                      {idx < prediction.blastRadius.length - 1 && (
                        <ArrowRight className="w-4 h-4 text-gray-500 shrink-0" />
                      )}
                    </React.Fragment>
                  );
                })}
              </div>

              <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937] text-xs text-gray-300 space-y-1">
                <span className="text-gray-500">Root Cause Diagnostics:</span>
                <p className="font-semibold text-red-400">
                  {prediction.rootCauseReason}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Prescriptive Remediation Actions */}
        <div className="p-5 bg-[#111827] border border-[#1F2937] rounded-lg space-y-4">
          <div className="flex items-center space-x-2 border-b border-[#1F2937] pb-3">
            <Zap className="w-4 h-4 text-cyan-400" />
            <h3 className="text-xs font-bold text-gray-200 uppercase tracking-wide">
              Prescriptive Automated Remediation
            </h3>
          </div>

          <RecommendationCard
            title={prediction.recommendedAction}
            description="Executes kubernetes pod scaling and database connection pool expansion to alleviate bottleneck."
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
