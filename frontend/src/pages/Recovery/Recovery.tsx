import React, { useState, useEffect } from 'react';
import { useTelemetryStore, getMLPrediction, calculateSystemRisk } from '../../store/useTelemetryStore';
import { recoveryApi } from '../../api/recovery';
import type { RecoveryAction } from '../../types/telemetry';
import {
  ShieldCheck,
  RotateCcw,
  PlusCircle,
  Database,
  Zap,
  History,
  Lock,
  Loader2,
  CheckCircle2,
  ShieldAlert,
  XCircle,
  AlertTriangle,
} from 'lucide-react';

export const Recovery: React.FC = () => {
  const { services, activeFaults, recoveryHistory, executeRecoveryAction, approveAction, rejectAction } =
    useTelemetryStore();
  const prediction = getMLPrediction(services, activeFaults);
  const risk = calculateSystemRisk(services, activeFaults);

  const [executingActionId, setExecutingActionId] = useState<string | null>(null);
  const [selectedTargetId, setSelectedTargetId] = useState<string>('');
  const [apiRecommendations, setApiRecommendations] = useState<RecoveryAction[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<RecoveryAction[]>([]);
  const [approvedActionIds, setApprovedActionIds] = useState<Set<string>>(new Set());
  const [loadingApprovalId, setLoadingApprovalId] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Default target service to root cause service or first non-healthy / first service
  const targetServiceId =
    selectedTargetId ||
    prediction.rootCauseServiceId ||
    (services.find((s) => s.status !== 'healthy')?.id || (services.length > 0 ? services[0].id : ''));
  const targetService = services.find((s) => s.id === targetServiceId);

  // Fetch live recommendations and pending approval items from API
  const refreshActions = async () => {
    try {
      const [recs, pending] = await Promise.all([
        recoveryApi.getRecommendations(),
        recoveryApi.getPending(),
      ]);
      if (Array.isArray(recs)) {
        setApiRecommendations(recs);
      }
      if (Array.isArray(pending)) {
        setPendingApprovals(pending);
      }
    } catch (err) {
      console.warn('[Recovery] Could not load dynamic recommendations from backend:', err);
    }
  };

  useEffect(() => {
    refreshActions();
    const interval = setInterval(refreshActions, 3000);
    return () => clearInterval(interval);
  }, [targetServiceId]);

  // Merge recommendations from API and prediction engine
  const activeRecommendations = (
    apiRecommendations.length > 0
      ? apiRecommendations
      : (prediction.recommendedActions || [])
  ).filter((act) => !selectedTargetId || act.targetServiceId === selectedTargetId);

  const handleGrantApproval = async (actionId: string, svcId: string) => {
    setLoadingApprovalId(actionId);
    try {
      await approveAction(actionId, svcId);
      setApprovedActionIds((prev) => new Set(prev).add(actionId));
      setStatusMessage({ type: 'success', text: `Approval granted for action ${actionId}. Ready for dispatch.` });
      await refreshActions();
    } catch (err) {
      setStatusMessage({ type: 'error', text: `Failed to record approval: ${err}` });
    } finally {
      setLoadingApprovalId(null);
    }
  };

  const handleRejectAction = async (actionId: string, svcId: string) => {
    setLoadingApprovalId(actionId);
    try {
      await rejectAction(actionId, svcId);
      setApprovedActionIds((prev) => {
        const next = new Set(prev);
        next.delete(actionId);
        return next;
      });
      setStatusMessage({ type: 'success', text: `Action proposal ${actionId} rejected by operator.` });
      await refreshActions();
    } catch (err) {
      setStatusMessage({ type: 'error', text: `Failed to reject proposal: ${err}` });
    } finally {
      setLoadingApprovalId(null);
    }
  };

  const handleExecute = async (
    actionType: string,
    serviceId: string,
    actionId: string,
    title: string,
    requiresApproval: boolean
  ) => {
    const isApproved = approvedActionIds.has(actionId);
    if (requiresApproval && !isApproved) {
      setStatusMessage({
        type: 'error',
        text: `Action "${title}" requires human approval before execution. Click "Grant Approval" above first.`,
      });
      return;
    }

    const finalServiceId = serviceId || targetServiceId;
    setExecutingActionId(actionId || title);
    setStatusMessage(null);

    try {
      await executeRecoveryAction(actionType, finalServiceId, actionId || title, isApproved);
      setStatusMessage({
        type: 'success',
        text: `Successfully executed ${title} on ${targetService?.name || finalServiceId}.`,
      });
      await refreshActions();
    } catch (err: unknown) {
      const errObj = err as { error?: string; message?: string };
      setStatusMessage({
        type: 'error',
        text: errObj?.error || errObj?.message || 'Execution blocked: operator approval required.',
      });
    } finally {
      setTimeout(() => {
        setExecutingActionId(null);
      }, 1500);
    }
  };

  // Helper icon selector based on action type
  const getActionIcon = (actionType: string) => {
    const norm = (actionType || '').toLowerCase();
    if (norm.includes('scale')) return <PlusCircle className="w-3.5 h-3.5 text-[#f07b9e]" />;
    if (norm.includes('pool')) return <Database className="w-3.5 h-3.5 text-[#e2588a]" />;
    if (norm.includes('flush')) return <Zap className="w-3.5 h-3.5 text-[#52b767]" />;
    if (norm.includes('reroute') || norm.includes('isolate')) return <AlertTriangle className="w-3.5 h-3.5 text-[#D4A017]" />;
    return <RotateCcw className="w-3.5 h-3.5 text-[#D4A017]" />;
  };

  const deduceActionType = (recId?: string, title?: string, family?: string): string => {
    const s = `${recId || ''} ${title || ''} ${family || ''}`.toLowerCase();
    if (s.includes('pool') || s.includes('database')) return 'increase_pool';
    if (s.includes('flush') || s.includes('cache')) return 'flush_cache';
    if (s.includes('reroute') || s.includes('latency')) return 'reroute_traffic';
    if (s.includes('isolate')) return 'isolate';
    if (s.includes('scale') || s.includes('cpu') || s.includes('queue')) return 'scale';
    return 'restart';
  };

  // Top recommended hero action
  const heroAction =
    activeRecommendations.length > 0
      ? activeRecommendations[0]
      : {
          id: prediction.recommendedActionId || 'act-auto',
          title: prediction.recommendedAction,
          description: prediction.rootCauseReason,
          targetServiceId: prediction.rootCauseServiceId,
          actionType: deduceActionType(
            prediction.recommendedActionId,
            prediction.recommendedAction,
            prediction.faultFamily
          ),
          confidence: prediction.confidence,
          status: 'idle' as const,
          riskLevel: risk > 70 ? ('HIGH' as const) : risk > 35 ? ('MEDIUM' as const) : ('LOW' as const),
          requiresApproval: risk > 35,
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

      {/* Notification Banner */}
      {statusMessage && (
        <div
          className={`p-3 rounded-lg border text-xs flex items-center justify-between ${
            statusMessage.type === 'success'
              ? 'bg-[#3F8E4F]/15 border-[#3F8E4F]/40 text-[#52b767]'
              : 'bg-[#840032]/20 border-[#AD2831]/50 text-[#f07b9e]'
          }`}
        >
          <span>{statusMessage.text}</span>
          <button onClick={() => setStatusMessage(null)} className="text-gray-400 hover:text-white ml-2">
            &times;
          </button>
        </div>
      )}

      {/* Recommended Action Hero Banner */}
      <div className="p-4 bg-[#840032]/20 border border-[#840032] rounded-lg space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2 text-[11px] font-semibold text-[#f07b9e] uppercase tracking-[0.07em]">
            <ShieldAlert className="w-4 h-4" />
            <span>
              AEGIS DECISION ENGINE: {prediction.faultFamily ? `${prediction.faultFamily.toUpperCase()} REMEDIATION` : 'REMEDIATION PROPOSAL'}
            </span>
          </div>

          <div className="flex items-center space-x-2">
            {prediction.faultFamily && (
              <span className="text-[10px] font-mono bg-[#151517] border border-[#840032] text-[#f07b9e] px-2 py-0.5 rounded">
                FAMILY: {prediction.faultFamily.toUpperCase()}
              </span>
            )}
            <span className="text-[10px] font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white px-2 py-0.5 rounded">
              CONFIDENCE: {prediction.confidence.toFixed(1)}%
            </span>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="text-[16px] font-semibold text-white mb-1">
              {heroAction.title}
            </h2>
            <p className="text-[12px] text-gray-300 font-normal leading-[18px]">
              Target workload: <span className="font-mono font-semibold text-white">{prediction.rootCauseServiceName || heroAction.targetServiceId}</span>. {heroAction.description || prediction.rootCauseReason}. Mitigates cascading risk from {risk}% down to nominal.
            </p>
          </div>

          <button
            onClick={() =>
              handleExecute(
                heroAction.actionType,
                heroAction.targetServiceId,
                heroAction.id,
                heroAction.title,
                Boolean(heroAction.requiresApproval)
              )
            }
            disabled={executingActionId === heroAction.id || executingActionId === heroAction.title}
            className={`px-4 py-2 rounded-md text-xs font-semibold transition-all flex items-center space-x-2 shadow-sm ${
              executingActionId === heroAction.id || executingActionId === heroAction.title
                ? 'bg-[#89023E] text-white border border-[#89023E]'
                : 'bg-[#840032] hover:bg-[#9b053d] text-white border border-[#840032]'
            }`}
          >
            {executingActionId === heroAction.id || executingActionId === heroAction.title ? (
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
      <div className="p-4 bg-[#151517] border border-[#D4A017]/60 rounded-lg space-y-3">
        <div className="flex items-center justify-between">
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
                  POLICY: MEDIUM & HIGH RISK REQUIRE APPROVAL
                </span>
              </div>
              <p className="text-[12px] text-gray-400 font-normal leading-[18px] mt-0.5">
                Destructive or topology-altering actions require operator sign-off before orchestrator dispatch.
              </p>
            </div>
          </div>
        </div>

        {/* Pending Approval Items List */}
        {pendingApprovals.length === 0 ? (
          <div className="p-3 bg-[#0B0B0C] border border-[#26262B] rounded-md text-xs text-gray-400 flex items-center justify-between">
            <span>No pending high-risk plans waiting for operator approval. Low-risk remediations execute autonomously.</span>
            <span className="text-[10px] font-mono text-[#52b767]">GATE CLEAR</span>
          </div>
        ) : (
          <div className="space-y-2">
            {pendingApprovals.map((plan) => {
              const isApproved = approvedActionIds.has(plan.id);
              return (
                <div
                  key={plan.id}
                  className="p-3 bg-[#0B0B0C] border border-[#D4A017]/40 rounded-md flex flex-wrap items-center justify-between gap-3 text-xs"
                >
                  <div className="space-y-0.5">
                    <div className="flex items-center space-x-2">
                      <span className="font-semibold text-white">{plan.title}</span>
                      <span className="text-[10px] font-mono bg-[#840032]/30 text-amber-300 px-1.5 py-0.2 rounded border border-[#D4A017]/50 uppercase">
                        {plan.riskLevel || 'HIGH'} RISK
                      </span>
                    </div>
                    <p className="text-[11px] text-gray-400">{plan.description}</p>
                  </div>

                  <div className="flex items-center space-x-2">
                    {isApproved ? (
                      <span className="flex items-center space-x-1 text-[#52b767] text-xs font-semibold">
                        <CheckCircle2 className="w-3.5 h-3.5" />
                        <span>APPROVAL RECORDED</span>
                      </span>
                    ) : (
                      <>
                        <button
                          onClick={() => handleRejectAction(plan.id, plan.targetServiceId)}
                          disabled={loadingApprovalId === plan.id}
                          className="px-2.5 py-1 rounded text-xs font-semibold border border-gray-600 hover:border-gray-400 text-gray-300 transition-all flex items-center space-x-1"
                        >
                          <XCircle className="w-3.5 h-3.5" />
                          <span>REJECT</span>
                        </button>
                        <button
                          onClick={() => handleGrantApproval(plan.id, plan.targetServiceId)}
                          disabled={loadingApprovalId === plan.id}
                          className="px-3 py-1 rounded text-xs font-semibold bg-[#840032]/40 hover:bg-[#840032]/70 text-amber-300 border border-[#D4A017] transition-all flex items-center space-x-1"
                        >
                          {loadingApprovalId === plan.id ? (
                            <Loader2 className="w-3.5 h-3.5 animate-spin" />
                          ) : (
                            <Lock className="w-3.5 h-3.5" />
                          )}
                          <span>GRANT OPERATOR APPROVAL</span>
                        </button>
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Dynamic Incident Remediation Actions (Replacing static cards) */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center space-x-2">
            <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
              INCIDENT REMEDIATION CANDIDATES
            </h3>
            <span className="text-[10px] text-gray-400">
              (Derived by Python Decision Engine from observed telemetry)
            </span>
          </div>

          <div className="flex items-center space-x-2 text-xs">
            <span className="text-gray-400">Target Workload:</span>
            <select
              value={selectedTargetId}
              onChange={(e) => setSelectedTargetId(e.target.value)}
              className="bg-[#0B0B0C] border border-[#26262B] text-gray-200 text-xs rounded px-2.5 py-1 focus:outline-none focus:border-[#e2588a]"
            >
              <option value="">All Anomalous Services</option>
              {services.map((svc) => (
                <option key={svc.id} value={svc.id}>
                  {svc.name} ({svc.id}) [{svc.status.toUpperCase()}]
                </option>
              ))}
            </select>
          </div>
        </div>

        {activeRecommendations.length === 0 ? (
          <div className="p-8 bg-[#151517] border border-[#26262B] rounded-lg text-center space-y-2">
            <CheckCircle2 className="w-8 h-8 text-[#52b767] mx-auto" />
            <h4 className="text-sm font-semibold text-white">Cluster Operating Nominally</h4>
            <p className="text-xs text-gray-400 max-w-md mx-auto">
              No active fault signatures or degraded thresholds detected on the selected workload. The decision engine will prescribe targeted candidate actions upon anomaly injection.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {activeRecommendations.map((act) => {
              const requiresApproval = Boolean(act.requiresApproval);
              const isApproved = approvedActionIds.has(act.id);
              const riskColor =
                act.riskLevel === 'HIGH'
                  ? 'border-[#AD2831]/60 text-[#f07b9e]'
                  : act.riskLevel === 'MEDIUM'
                  ? 'border-[#D4A017]/60 text-amber-300'
                  : 'border-[#3F8E4F]/60 text-[#52b767]';

              return (
                <div
                  key={act.id}
                  className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg flex flex-col justify-between space-y-3"
                >
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2">
                        {getActionIcon(act.actionType)}
                        <span className="font-semibold text-xs text-white">{act.title}</span>
                      </div>
                      <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded border uppercase ${riskColor}`}>
                        {act.riskLevel || 'LOW'}
                      </span>
                    </div>

                    <p className="text-[11px] text-gray-400 font-normal leading-[16px]">
                      {act.description}
                    </p>

                    <div className="flex items-center justify-between text-[10px] text-gray-400 pt-1 border-t border-[#26262B]">
                      <span>Workload: <span className="font-mono text-white">{act.targetServiceId}</span></span>
                      <span>
                        Policy:{' '}
                        <span className={requiresApproval ? 'text-amber-300 font-semibold' : 'text-[#52b767]'}>
                          {requiresApproval ? (isApproved ? 'Approved' : 'Approval Gate') : 'Autonomous'}
                        </span>
                      </span>
                    </div>
                  </div>

                  <div className="pt-2">
                    {requiresApproval && !isApproved ? (
                      <button
                        onClick={() => handleGrantApproval(act.id, act.targetServiceId)}
                        disabled={loadingApprovalId === act.id}
                        className="w-full py-1.5 rounded-md text-xs font-semibold bg-[#840032]/30 hover:bg-[#840032]/60 text-amber-300 border border-[#D4A017] transition-all flex items-center justify-center space-x-1"
                      >
                        <Lock className="w-3 h-3" />
                        <span>APPROVE FIRST</span>
                      </button>
                    ) : (
                      <button
                        onClick={() =>
                          handleExecute(
                            act.actionType,
                            act.targetServiceId,
                            act.id,
                            act.title,
                            requiresApproval
                          )
                        }
                        disabled={executingActionId === act.id}
                        className={`w-full py-1.5 rounded-md text-xs font-semibold border transition-all ${
                          executingActionId === act.id
                            ? 'bg-[#89023E] text-white border-[#89023E]'
                            : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-[#f07b9e] border border-[#840032]'
                        }`}
                      >
                        {executingActionId === act.id ? 'EXECUTING...' : 'DISPATCH ACTION'}
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Recovery History Audit Log */}
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
