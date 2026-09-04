import React from 'react';
import { useTelemetryStore, getMLPrediction, calculateSystemRisk } from '../../store/useTelemetryStore';
import { ShieldCheck, RotateCcw, PlusCircle, Database, Zap, History } from 'lucide-react';

export const Recovery: React.FC = () => {
  const { services, activeFaults, recoveryHistory, executeRecoveryAction } = useTelemetryStore();
  const prediction = getMLPrediction(services, activeFaults);
  const risk = calculateSystemRisk(services, activeFaults);

  return (
    <div className="p-6 space-y-6 font-mono">
      {/* Header */}
      <div className="border-b border-[#1F2937] pb-4">
        <div className="flex items-center space-x-2">
          <ShieldCheck className="w-5 h-5 text-cyan-400" />
          <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
            Automated Incident Response & Recovery Console
          </h1>
        </div>
        <p className="text-xs text-gray-400">
          Execute ML-suggested remediation actions or manual operator overrides with automated SLA recovery verification
        </p>
      </div>

      {/* Recommended Action Hero Banner */}
      <div className="p-5 bg-cyan-950/30 border border-cyan-800/80 rounded-lg space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2 text-xs font-bold text-cyan-400 uppercase">
            <Zap className="w-4 h-4" />
            <span>Current Recommended Remediation</span>
          </div>

          <span className="text-xs font-bold bg-cyan-950 text-cyan-300 border border-cyan-700 px-2.5 py-0.5 rounded">
            CONFIDENCE: {prediction.confidence}%
          </span>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="text-base font-bold text-gray-100 mb-1">
              {prediction.recommendedAction}
            </h2>
            <p className="text-xs text-gray-300">
              Target microservice: <span className="font-bold text-white">{prediction.rootCauseServiceName}</span>. Mitigates cascading failure risk from {risk}% down to nominal ~10%.
            </p>
          </div>

          <button
            onClick={() => {
              executeRecoveryAction('scale', prediction.rootCauseServiceId, prediction.recommendedAction);
            }}
            className="px-5 py-2 bg-cyan-900 hover:bg-cyan-800 text-white rounded text-xs font-bold border border-cyan-700 transition-all flex items-center space-x-2 shadow"
          >
            <ShieldCheck className="w-4 h-4" />
            <span>EXECUTE RECOMMENDED RECOVERY</span>
          </button>
        </div>
      </div>

      {/* Manual Actions Grid */}
      <div className="space-y-3">
        <h3 className="text-xs font-bold text-gray-300 uppercase tracking-wide">
          Manual Operator Action Shortcuts
        </h3>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="p-4 bg-[#111827] border border-[#1F2937] hover:border-gray-600 rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-amber-400 mb-2">
                <RotateCcw className="w-4 h-4" />
                <span className="font-bold text-xs">Restart Payment</span>
              </div>
              <p className="text-xs text-gray-400 mb-3">
                Graceful rolling restart of all Payment pod instances.
              </p>
            </div>
            <button
              onClick={() => executeRecoveryAction('restart', 'payment', 'Restarted Payment Pods')}
              className="w-full py-1.5 bg-[#0B0F19] hover:bg-[#1E293B] text-amber-300 border border-amber-800/80 rounded text-xs font-bold"
            >
              EXECUTE RESTART
            </button>
          </div>

          <div className="p-4 bg-[#111827] border border-[#1F2937] hover:border-gray-600 rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-sky-400 mb-2">
                <PlusCircle className="w-4 h-4" />
                <span className="font-bold text-xs">Scale Payment x2</span>
              </div>
              <p className="text-xs text-gray-400 mb-3">
                Double replica capacity from current count to handle traffic spike.
              </p>
            </div>
            <button
              onClick={() => executeRecoveryAction('scale', 'payment', 'Scaled Payment Replicas x2')}
              className="w-full py-1.5 bg-[#0B0F19] hover:bg-[#1E293B] text-sky-300 border border-sky-800/80 rounded text-xs font-bold"
            >
              SCALE WORKLOAD
            </button>
          </div>

          <div className="p-4 bg-[#111827] border border-[#1F2937] hover:border-gray-600 rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-purple-400 mb-2">
                <Database className="w-4 h-4" />
                <span className="font-bold text-xs">Increase DB Pool</span>
              </div>
              <p className="text-xs text-gray-400 mb-3">
                Expand PostgreSQL max_connections setting to 200 workers.
              </p>
            </div>
            <button
              onClick={() => executeRecoveryAction('pool_expand', 'database', 'Expanded DB Connection Pool')}
              className="w-full py-1.5 bg-[#0B0F19] hover:bg-[#1E293B] text-purple-300 border border-purple-800/80 rounded text-xs font-bold"
            >
              EXPAND POOL
            </button>
          </div>

          <div className="p-4 bg-[#111827] border border-[#1F2937] hover:border-gray-600 rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-emerald-400 mb-2">
                <Zap className="w-4 h-4" />
                <span className="font-bold text-xs">Flush Redis Cache</span>
              </div>
              <p className="text-xs text-gray-400 mb-3">
                Purge stale cache keys and flush corrupted memory buffers.
              </p>
            </div>
            <button
              onClick={() => executeRecoveryAction('flush_cache', 'redis', 'Flushed Redis Cache')}
              className="w-full py-1.5 bg-[#0B0F19] hover:bg-[#1E293B] text-emerald-300 border border-emerald-800/80 rounded text-xs font-bold"
            >
              FLUSH CACHE
            </button>
          </div>
        </div>
      </div>

      {/* Recovery History Audit Log */}
      <div className="p-5 bg-[#111827] border border-[#1F2937] rounded-lg space-y-3">
        <div className="flex items-center space-x-2 border-b border-[#1F2937] pb-3">
          <History className="w-4 h-4 text-gray-400" />
          <h3 className="text-xs font-bold text-gray-200 uppercase tracking-wide">
            Remediation Execution History & Audit Log
          </h3>
        </div>

        <div className="space-y-2">
          {recoveryHistory.length === 0 ? (
            <div className="p-4 text-center text-gray-500 text-xs">
              No recovery actions executed during current session.
            </div>
          ) : (
            recoveryHistory.map((item) => (
              <div
                key={item.id}
                className="p-3 bg-[#0B0F19] border border-[#1F2937] rounded flex items-center justify-between text-xs"
              >
                <div className="flex items-center space-x-3">
                  <span className="text-gray-500">{item.timestamp}</span>
                  <span className="font-bold text-gray-100">{item.actionTitle}</span>
                  <span className="text-[10px] text-gray-400">({item.targetService})</span>
                </div>

                <div className="flex items-center space-x-3">
                  <span className="text-gray-400 text-[11px]">
                    Risk: <span className="text-red-400">{item.riskBefore}%</span> {'->'}{' '}
                    <span className="text-emerald-400">{item.riskAfter}%</span>
                  </span>
                  <span className="text-[10px] bg-emerald-950 text-emerald-400 border border-emerald-800 px-2 py-0.5 rounded font-bold uppercase">
                    {item.status} ({item.duration})
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
