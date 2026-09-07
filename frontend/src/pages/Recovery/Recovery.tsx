import React, { useState } from 'react';
import { useTelemetryStore, getMLPrediction, calculateSystemRisk } from '../../store/useTelemetryStore';
import { ShieldCheck, RotateCcw, PlusCircle, Database, Zap, History, Lock, Loader2, CheckCircle2, ShieldAlert } from 'lucide-react';

export const Recovery: React.FC = () => {
  const { services, activeFaults, recoveryHistory, executeRecoveryAction } = useTelemetryStore();
  const prediction = getMLPrediction(services, activeFaults);
  const risk = calculateSystemRisk(services, activeFaults);

  const [executingActionId, setExecutingActionId] = useState<string | null>(null);
  const [approvedHumanGate, setApprovedHumanGate] = useState<boolean>(false);
  const [selectedTargetId, setSelectedTargetId] = useState<string>('');

  // Default target service to root cause service or first service
  const targetServiceId = selectedTargetId || prediction.rootCauseServiceId || (services.length > 0 ? services[0].id : '');
  const targetService = services.find((s) => s.id === targetServiceId);

  const handleExecute = (actionType: 'scale' | 'restart' | 'flush_cache' | 'pool_expand', serviceId: string, title: string) => {
    const finalServiceId = serviceId || targetServiceId;
    setExecutingActionId(title);
    executeRecoveryAction(actionType, finalServiceId, title);
    setTimeout(() => {
      setExecutingActionId(null);
    }, 2000);
  };

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="border-b border-[#26262B] pb-4">
        <div className="flex items-center space-x-2">
          <ShieldCheck className="w-5 h-5 text-[#e2588a]" />
          <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
            Automated Incident Response & Recovery
          </h1>
        </div>
        <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
          Dependency-ordered autonomous recovery execution with authority boundary gating and SLA verification
        </p>
      </div>

      {/* Recommended Action Hero Banner (Aegis Intervention -> Executing) */}
      <div className="p-4 bg-[#840032]/20 border border-[#840032] rounded-lg space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2 text-[11px] font-semibold text-[#f07b9e] uppercase tracking-[0.07em]">
            <ShieldAlert className="w-4 h-4" />
            <span>AEGIS INTERVENTION: REMEDIATION PROPOSAL</span>
          </div>

          <span className="text-[10px] font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white px-2 py-0.5 rounded">
            CONFIDENCE: {prediction.confidence}%
          </span>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="text-[16px] font-semibold text-white mb-1">
              {prediction.recommendedAction}
            </h2>
            <p className="text-[12px] text-gray-300 font-normal leading-[18px]">
              Target workload: <span className="font-mono font-semibold text-white">{prediction.rootCauseServiceName}</span>. Mitigates cascading failure risk from {risk}% down to nominal ~10%.
            </p>
          </div>

          <button
            onClick={() => handleExecute('scale', prediction.rootCauseServiceId, prediction.recommendedAction)}
            disabled={executingActionId === prediction.recommendedAction}
            className={`px-4 py-2 rounded-md text-xs font-semibold transition-all flex items-center space-x-2 shadow-sm ${
              executingActionId === prediction.recommendedAction
                ? 'bg-[#89023E] text-white border border-[#89023E]'
                : 'bg-[#840032] hover:bg-[#9b053d] text-white border border-[#840032]'
            }`}
          >
            {executingActionId === prediction.recommendedAction ? (
              <>
                <Loader2 className="w-3.5 h-3.5 animate-spin text-white" />
                <span className="text-[10px] uppercase tracking-[0.04em]">EXECUTING INTERVENTION...</span>
              </>
            ) : (
              <>
                <Zap className="w-3.5 h-3.5" />
                <span className="text-[10px] uppercase tracking-[0.04em]">EXECUTE AEGIS INTERVENTION</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Authority Boundary / Human Approval Gate */}
      <div className="p-4 bg-[#151517] border border-[#D4A017]/60 rounded-lg flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="p-2 bg-[#840032]/30 text-[#D4A017] border border-[#D4A017]/70 rounded-md">
            <Lock className="w-4 h-4 text-[#D4A017]" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <span className="font-semibold text-[11px] uppercase tracking-[0.07em] text-amber-300">
                AUTHORITY BOUNDARY GATE
              </span>
              <span className="text-[10px] bg-[#840032]/30 text-[#D4A017] border border-[#840032] px-1.5 py-0.5 rounded font-semibold tracking-[0.04em] uppercase">
                HUMAN APPROVAL REQUIRED
              </span>
            </div>
            <p className="text-[12px] text-gray-400 font-normal leading-[18px] mt-0.5">
              High-risk destructive operations require human operator sign-off before orchestrator dispatch.
            </p>
          </div>
        </div>

        <button
          onClick={() => setApprovedHumanGate(!approvedHumanGate)}
          className={`px-3.5 py-1.5 rounded-md text-xs font-semibold border transition-all flex items-center space-x-1.5 ${
            approvedHumanGate
              ? 'bg-[#3F8E4F]/20 text-[#52b767] border-[#3F8E4F]'
              : 'bg-[#840032]/30 text-amber-300 border-[#D4A017] hover:bg-[#840032]/50'
          }`}
        >
          {approvedHumanGate ? (
            <>
              <CheckCircle2 className="w-3.5 h-3.5 text-[#3F8E4F]" />
              <span className="text-[10px] uppercase tracking-[0.04em]">APPROVAL GRANTED</span>
            </>
          ) : (
            <>
              <Lock className="w-3.5 h-3.5 text-[#D4A017]" />
              <span className="text-[10px] uppercase tracking-[0.04em]">GRANT OPERATOR APPROVAL</span>
            </>
          )}
        </button>
      </div>

      {/* Manual Actions Grid */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
            OPERATOR REMEDIATION ACTIONS
          </h3>
          <div className="flex items-center space-x-2 text-xs">
            <span className="text-gray-400">Target Workload:</span>
            <select
              value={selectedTargetId}
              onChange={(e) => setSelectedTargetId(e.target.value)}
              className="bg-[#0B0B0C] border border-[#26262B] text-gray-200 text-xs rounded px-2.5 py-1 focus:outline-none focus:border-[#e2588a]"
            >
              {services.map((svc) => (
                <option key={svc.id} value={svc.id}>
                  {svc.name} ({svc.id}) [{svc.status.toUpperCase()}]
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-[#D4A017] mb-1.5">
                <RotateCcw className="w-3.5 h-3.5" />
                <span className="font-semibold text-xs">Restart Service</span>
              </div>
              <p className="text-[11px] text-gray-400 font-normal leading-[16px] mb-3">
                Graceful rolling restart of container instances on {targetService ? targetService.name : 'target node'}.
              </p>
            </div>
            <button
              onClick={() => handleExecute('restart', selectedTargetId, `Restart ${targetService?.name || selectedTargetId}`)}
              disabled={executingActionId === `Restart ${targetService?.name || selectedTargetId}`}
              className={`w-full py-1.5 rounded-md text-xs font-semibold border transition-all ${
                executingActionId === `Restart ${targetService?.name || selectedTargetId}`
                  ? 'bg-[#89023E] text-white border-[#89023E]'
                  : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-[#e5b533] border border-[#D4A017]/60'
              }`}
            >
              {executingActionId === `Restart ${targetService?.name || selectedTargetId}` ? 'EXECUTING...' : 'RESTART PODS'}
            </button>
          </div>

          <div className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-[#f07b9e] mb-1.5">
                <PlusCircle className="w-3.5 h-3.5" />
                <span className="font-semibold text-xs">Scale Replicas</span>
              </div>
              <p className="text-[11px] text-gray-400 font-normal leading-[16px] mb-3">
                Increase replica capacity by +2 pods on {targetService ? targetService.name : 'target node'}.
              </p>
            </div>
            <button
              onClick={() => handleExecute('scale', selectedTargetId, `Scale ${targetService?.name || selectedTargetId} Replicas`)}
              disabled={executingActionId === `Scale ${targetService?.name || selectedTargetId} Replicas`}
              className={`w-full py-1.5 rounded-md text-xs font-semibold border transition-all ${
                executingActionId === `Scale ${targetService?.name || selectedTargetId} Replicas`
                  ? 'bg-[#89023E] text-white border-[#89023E]'
                  : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-[#f07b9e] border border-[#840032]'
              }`}
            >
              {executingActionId === `Scale ${targetService?.name || selectedTargetId} Replicas` ? 'EXECUTING...' : 'SCALE REPLICAS'}
            </button>
          </div>

          <div className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-[#e2588a] mb-1.5">
                <Database className="w-3.5 h-3.5" />
                <span className="font-semibold text-xs">Expand Worker Pool</span>
              </div>
              <p className="text-[11px] text-gray-400 font-normal leading-[16px] mb-3">
                Expand connection limits and worker thread capacity on {targetService ? targetService.name : 'target node'}.
              </p>
            </div>
            <button
              onClick={() => handleExecute('pool_expand', selectedTargetId, `Expand Pool for ${targetService?.name || selectedTargetId}`)}
              disabled={executingActionId === `Expand Pool for ${targetService?.name || selectedTargetId}`}
              className={`w-full py-1.5 rounded-md text-xs font-semibold border transition-all ${
                executingActionId === `Expand Pool for ${targetService?.name || selectedTargetId}`
                  ? 'bg-[#89023E] text-white border-[#89023E]'
                  : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-[#e2588a] border border-[#840032]'
              }`}
            >
              {executingActionId === `Expand Pool for ${targetService?.name || selectedTargetId}` ? 'EXECUTING...' : 'EXPAND POOL'}
            </button>
          </div>

          <div className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg flex flex-col justify-between">
            <div>
              <div className="flex items-center space-x-2 text-[#3F8E4F] mb-1.5">
                <Zap className="w-3.5 h-3.5" />
                <span className="font-semibold text-xs">Flush Buffer & Cache</span>
              </div>
              <p className="text-[11px] text-gray-400 font-normal leading-[16px] mb-3">
                Purge stale memory buffers and cache allocations on {targetService ? targetService.name : 'target node'}.
              </p>
            </div>
            <button
              onClick={() => handleExecute('flush_cache', selectedTargetId, `Flush Cache on ${targetService?.name || selectedTargetId}`)}
              disabled={executingActionId === `Flush Cache on ${targetService?.name || selectedTargetId}`}
              className={`w-full py-1.5 rounded-md text-xs font-semibold border transition-all ${
                executingActionId === `Flush Cache on ${targetService?.name || selectedTargetId}`
                  ? 'bg-[#89023E] text-white border-[#89023E]'
                  : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-[#52b767] border border-[#3F8E4F]/60'
              }`}
            >
              {executingActionId === `Flush Cache on ${targetService?.name || selectedTargetId}` ? 'EXECUTING...' : 'FLUSH CACHE'}
            </button>
          </div>
        </div>
      </div>

      {/* Recovery History Audit Log (Recovered -> Healthy) */}
      <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg space-y-3">
        <div className="flex items-center space-x-2 border-b border-[#26262B] pb-2.5">
          <History className="w-4 h-4 text-gray-400" />
          <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
            REMEDIATION EXECUTION HISTORY & AUDIT LOG
          </h3>
        </div>

        <div className="space-y-1.5">
          {recoveryHistory.length === 0 ? (
            <div className="p-4 text-center text-gray-500 text-xs">
              No recovery actions executed during current session.
            </div>
          ) : (
            recoveryHistory.map((item) => (
              <div
                key={item.id}
                className="p-2.5 bg-[#0B0B0C] border border-[#26262B] rounded-md flex items-center justify-between text-xs"
              >
                <div className="flex items-center space-x-3">
                  <span className="font-mono text-gray-500 text-[10px]">{item.timestamp}</span>
                  <span className="font-semibold text-gray-100">{item.actionTitle}</span>
                  <span className="font-mono text-[10px] text-gray-400">({item.targetService})</span>
                </div>

                <div className="flex items-center space-x-3">
                  <span className="text-gray-400 text-[11px]">
                    Risk: <span className="font-mono text-[#AD2831]">{item.riskBefore}%</span> {'->'}{' '}
                    <span className="font-mono text-[#3F8E4F]">{item.riskAfter}%</span>
                  </span>
                  <span className="text-[10px] bg-[#3F8E4F]/15 text-[#52b767] border border-[#3F8E4F]/50 px-2 py-0.5 rounded font-semibold tracking-[0.04em] uppercase">
                    RECOVERED ({item.duration})
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
