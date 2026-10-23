import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { recoveryApi } from '../../api/recovery';
import {
  ShieldCheck,
  RotateCcw,
  History,
  Loader2,
  Play,
  Pause,
} from 'lucide-react';

interface RecoveryLogProps {
  maxItems?: number;
  className?: string;
}

export const RecoveryLog: React.FC<RecoveryLogProps> = ({
  maxItems = 10,
  className = '',
}) => {
  const { recoveryHistory, autonomousStatus, setAutonomousStatus, tickTelemetry } = useTelemetryStore();
  const [rollingBackId, setRollingBackId] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  const isAutonomousEnabled = autonomousStatus?.enabled ?? true;

  const handleToggleAutonomous = async () => {
    try {
      await setAutonomousStatus(!isAutonomousEnabled);
      setFeedback(`Autonomous recovery ${!isAutonomousEnabled ? 'ENABLED' : 'PAUSED'}.`);
      setTimeout(() => setFeedback(null), 3000);
    } catch (err: any) {
      setFeedback(`Failed to update mode: ${err?.message || err}`);
    }
  };

  const handleRollback = async (historyId: string) => {
    setRollingBackId(historyId);
    setFeedback(null);
    try {
      await recoveryApi.rollbackAction(historyId);
      setFeedback(`Rollback successful for execution ${historyId}. Telemetry reverting.`);
      await tickTelemetry();
      setTimeout(() => setFeedback(null), 4000);
    } catch (err: any) {
      setFeedback(`Rollback failed: ${err?.message || err}`);
    } finally {
      setRollingBackId(null);
    }
  };

  const displayedHistory = recoveryHistory.slice(0, maxItems);

  return (
    <div className={`bg-[#0F0F11] border border-[#26262B] rounded-xl overflow-hidden flex flex-col font-sans ${className}`}>
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between px-4 py-2.5 bg-[#151517] border-b border-[#26262B] gap-2">
        <div className="flex items-center space-x-2.5">
          <div className="p-1.5 bg-[#3F8E4F]/20 border border-[#3F8E4F] rounded-md text-[#52b767]">
            <History className="w-4 h-4" />
          </div>
          <div>
            <span className="text-xs font-semibold text-white tracking-wide">
              REMEDIAL ACTIONS & RECOVERY LOG
            </span>
            <span className="text-[11px] text-gray-400 ml-2 font-mono">
              {recoveryHistory.length} total executions
            </span>
          </div>
        </div>

        {/* Autonomous Mode Toggle & Status */}
        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1.5 text-[11px]">
            <span className={`w-2 h-2 rounded-full ${isAutonomousEnabled ? 'bg-emerald-500 animate-pulse' : 'bg-amber-500'}`} />
            <span className={`font-semibold ${isAutonomousEnabled ? 'text-emerald-400' : 'text-amber-400'}`}>
              {isAutonomousEnabled ? 'AUTONOMOUS ACTIVE' : 'AUTONOMOUS PAUSED'}
            </span>
          </div>

          <button
            onClick={handleToggleAutonomous}
            className={`flex items-center space-x-1 px-2.5 py-1 rounded text-[10px] font-semibold border transition-colors ${
              isAutonomousEnabled
                ? 'bg-[#151517] hover:bg-[#202024] text-gray-300 border-[#26262B]'
                : 'bg-emerald-950/60 hover:bg-emerald-900/80 text-emerald-300 border-emerald-700'
            }`}
          >
            {isAutonomousEnabled ? (
              <>
                <Pause className="w-3 h-3 text-amber-400" />
                <span>PAUSE</span>
              </>
            ) : (
              <>
                <Play className="w-3 h-3 text-emerald-400 fill-emerald-400" />
                <span>RESUME</span>
              </>
            )}
          </button>
        </div>
      </div>

      {feedback && (
        <div className="px-4 py-2 bg-[#151517] border-b border-[#26262B] text-xs font-mono text-emerald-400">
          {feedback}
        </div>
      )}

      {/* History Log List */}
      <div className="p-4 space-y-2.5 overflow-y-auto max-h-[420px]">
        {displayedHistory.length === 0 ? (
          <div className="text-center py-8 text-gray-400 space-y-2">
            <ShieldCheck className="w-8 h-8 text-gray-400 mx-auto" />
            <div className="text-xs font-medium text-gray-300">No remediations recorded yet</div>
            <div className="text-[11px] text-gray-400">
              Inject a fault or trigger autonomous recovery to observe real-time remediation
            </div>
          </div>
        ) : (
          displayedHistory.map((item) => {
            const isVerified = item.status === 'Verified' || item.status === 'Completed';
            const isIneffective = item.status === 'Ineffective' || item.status === 'Failed';
            const isRolledBack = item.status === 'RolledBack';
            const isVerifying = item.status === 'Verifying';

            return (
              <div
                key={item.id}
                className="p-3 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg transition-all space-y-2"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center space-x-2">
                    <span className="font-semibold text-white text-xs">{item.actionTitle}</span>
                    <span className="text-[10px] font-mono bg-[#0B0B0C] text-gray-300 px-2 py-0.5 rounded border border-[#26262B]">
                      {item.targetService}
                    </span>
                  </div>

                  {/* Status Badge */}
                  <div className="flex items-center space-x-2">
                    <span
                      className={`text-[10px] font-mono px-2 py-0.5 rounded font-semibold uppercase flex items-center space-x-1 ${
                        isVerified
                          ? 'bg-emerald-950/60 text-emerald-400 border border-emerald-800'
                          : isIneffective
                          ? 'bg-red-950/60 text-red-400 border border-red-800'
                          : isRolledBack
                          ? 'bg-purple-950/60 text-purple-400 border border-purple-800'
                          : 'bg-amber-950/60 text-amber-400 border border-amber-800'
                      }`}
                    >
                      {isVerifying && <Loader2 className="w-2.5 h-2.5 animate-spin mr-1" />}
                      <span>{item.status}</span>
                    </span>

                    {/* Rollback button for completed actions */}
                    {(isVerified || isIneffective) && (
                      <button
                        onClick={() => handleRollback(item.id)}
                        disabled={rollingBackId === item.id}
                        className="flex items-center space-x-1 px-2 py-0.5 bg-[#0B0B0C] hover:bg-[#1a1a1e] text-gray-300 border border-[#26262B] hover:border-purple-600 rounded text-[10px] font-semibold transition-colors"
                      >
                        <RotateCcw className="w-2.5 h-2.5 text-purple-400" />
                        <span>{rollingBackId === item.id ? 'Reverting...' : 'Rollback'}</span>
                      </button>
                    )}
                  </div>
                </div>

                {/* Telemetry Impact & Duration Metrics */}
                <div className="flex items-center justify-between text-[11px] text-gray-400 pt-1 border-t border-[#26262B]/80 font-mono">
                  <div className="flex items-center space-x-3">
                    <span>
                      Risk Delta: <span className="text-red-400 font-semibold">{item.riskBefore}%</span> →{' '}
                      <span className="text-emerald-400 font-semibold">{item.riskAfter}%</span>
                    </span>
                    <span>·</span>
                    <span>Elapsed: {item.duration}</span>
                  </div>

                  <span className="text-gray-400">{item.timestamp}</span>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
