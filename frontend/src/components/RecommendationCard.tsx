import React, { useState } from 'react';
import { ShieldAlert, CheckCircle2, ArrowRight, Loader2 } from 'lucide-react';

interface RecommendationCardProps {
  title: string;
  description: string;
  targetService: string;
  confidence: number;
  onExecute: () => void;
  isExecuting?: boolean;
  actionType?: string;
  riskLevel?: 'LOW' | 'MEDIUM' | 'HIGH';
  requiresApproval?: boolean;
  faultFamily?: string;
}

export const RecommendationCard: React.FC<RecommendationCardProps> = ({
  title,
  description,
  targetService,
  confidence,
  onExecute,
  isExecuting = false,
  actionType,
  riskLevel = 'LOW',
  requiresApproval = false,
  faultFamily,
}) => {
  const [done, setDone] = useState(false);
  const [localExecuting, setLocalExecuting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleAction = async () => {
    setLocalExecuting(true);
    setErrorMessage(null);
    try {
      await onExecute();
      setDone(true);
      setTimeout(() => setDone(false), 5000);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : 'Recovery request failed');
    } finally {
      setLocalExecuting(false);
    }
  };

  const currentlyExecuting = isExecuting || localExecuting;
  const isHighRisk = riskLevel === 'HIGH';
  const isMedRisk = riskLevel === 'MEDIUM';
  const needsApproval = requiresApproval || isHighRisk || isMedRisk;

  return (
    <div className="p-4 bg-[#151517] border border-[#840032] rounded-lg shadow-sm font-sans flex flex-col justify-between">
      <div>
        {/* Section Heading */}
        <div className="flex items-center justify-between border-b border-[#26262B] pb-2.5 mb-3">
          <div className="flex items-center space-x-2">
            <span className="text-[11px] font-semibold text-[#f07b9e] tracking-[0.07em] uppercase leading-[16px]">
              RECOVERY DECISION
            </span>
            {faultFamily && (
              <span className="text-[9px] font-mono bg-[#840032]/30 text-[#f07b9e] border border-[#840032] px-1.5 py-0.2 rounded uppercase">
                {faultFamily}
              </span>
            )}
          </div>
          <span
            className={`text-[10px] font-semibold tracking-[0.04em] uppercase px-2 py-0.5 rounded border leading-[16px] ${
              done
                ? 'bg-[#3F8E4F]/15 text-[#52b767] border-[#3F8E4F]/60'
                : currentlyExecuting
                ? 'bg-[#89023E]/25 text-[#f472b6] border-[#89023E]'
                : 'bg-[#840032]/25 text-[#f07b9e] border-[#840032]'
            }`}
          >
            {currentlyExecuting ? 'EXECUTING' : done ? 'DISPATCHED' : 'READY'}
          </span>
        </div>

        {/* Authority Block */}
        <div className="mb-3">
          <div className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
            AUTHORITY
          </div>
          <div className={`text-[18px] font-bold tracking-[-0.02em] leading-[24px] ${needsApproval ? 'text-amber-300' : 'text-white'}`}>
            {needsApproval ? 'OPERATOR APPROVAL REQUIRED' : 'AUTOMATIC'}
          </div>
          <div className="text-[11px] text-gray-400 font-normal leading-[16px] mt-0.5">
            {needsApproval
              ? `Safety gate: ${riskLevel} risk action requires human authorization before dispatch`
              : 'Policy allows autonomous recovery for this low-risk failure pattern'}
          </div>
        </div>

        {/* Recommended Action */}
        <div className="mb-3">
          <div className="flex items-center justify-between">
            <div className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              RECOMMENDED ACTION
            </div>
            {actionType && (
              <span className="text-[10px] font-mono text-gray-300 uppercase bg-[#0B0B0C] px-1.5 py-0.5 rounded border border-[#26262B]">
                {actionType}
              </span>
            )}
          </div>
          <div className="text-[14px] font-semibold text-gray-100 leading-[20px] mt-0.5">
            {title}
          </div>
          <div className="font-mono text-[14px] font-semibold text-gray-200 mt-1 bg-[#0B0B0C] p-2 rounded-md border border-[#26262B]">
            {targetService}
          </div>
          <p className="text-[11px] text-gray-400 font-normal leading-[16px] mt-1.5">{description}</p>
        </div>

        {/* Confidence & Risk Level */}
        <div className="grid grid-cols-2 gap-2 mb-4 bg-[#0B0B0C] p-2 rounded-md border border-[#26262B]">
          <div>
            <div className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              CONFIDENCE
            </div>
            <div className="font-mono text-[14px] font-semibold text-[#52b767] mt-0.5">
              {confidence}%
            </div>
          </div>
          <div>
            <div className="text-[10px] font-semibold tracking-[0.06em] text-gray-400 uppercase leading-[14px]">
              RISK LEVEL
            </div>
            <div
              className={`font-sans text-[11px] font-semibold tracking-[0.04em] uppercase mt-0.5 ${
                isHighRisk
                  ? 'text-[#f87171]'
                  : isMedRisk
                  ? 'text-amber-400'
                  : 'text-emerald-400'
              }`}
            >
              {riskLevel} {needsApproval ? '(APPROVAL GATE)' : '(AUTO-APPROVED)'}
            </div>
          </div>
        </div>
      </div>

      {errorMessage && (
        <div role="alert" className="mb-3 rounded border border-red-800 bg-red-950/40 px-3 py-2 text-xs text-red-200">
          {errorMessage}
        </div>
      )}

      <button
        onClick={handleAction}
        disabled={currentlyExecuting || done}
        className={`w-full py-2 px-3 rounded-md text-xs font-semibold font-sans border transition-all flex items-center justify-center space-x-2 ${
          done
            ? 'bg-[#3F8E4F]/20 text-[#52b767] border-[#3F8E4F]'
            : currentlyExecuting
            ? 'bg-[#89023E] text-white border-[#89023E]'
            : 'bg-[#840032] hover:bg-[#9b053d] text-white border-[#840032]'
        }`}
      >
        {done ? (
          <>
            <CheckCircle2 className="w-3.5 h-3.5 text-[#52b767]" />
            <span className="text-[10px] font-semibold uppercase tracking-[0.04em]">DISPATCHED (VERIFYING)</span>
          </>
        ) : currentlyExecuting ? (
          <>
            <Loader2 className="w-3.5 h-3.5 animate-spin text-white" />
            <span className="text-[10px] font-semibold uppercase tracking-[0.04em]">EXECUTING RECOVERY...</span>
          </>
        ) : (
          <>
            <ShieldAlert className="w-3.5 h-3.5" />
            <span className="text-[10px] font-semibold uppercase tracking-[0.04em]">
              {needsApproval ? 'APPROVE & EXECUTE PLAN' : 'EXECUTE PLAN NOW'}
            </span>
            <ArrowRight className="w-3 h-3" />
          </>
        )}
      </button>
    </div>
  );
};
