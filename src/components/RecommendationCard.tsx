import React, { useState } from 'react';
import { ShieldCheck, Zap, CheckCircle2, ArrowRight } from 'lucide-react';

interface RecommendationCardProps {
  title: string;
  description: string;
  targetService: string;
  confidence: number;
  onExecute: () => void;
  isExecuting?: boolean;
}

export const RecommendationCard: React.FC<RecommendationCardProps> = ({
  title,
  description,
  targetService,
  confidence,
  onExecute,
  isExecuting = false,
}) => {
  const [done, setDone] = useState(false);

  const handleAction = () => {
    onExecute();
    setDone(true);
    setTimeout(() => setDone(false), 3000);
  };

  return (
    <div className="p-5 bg-[#0F172A] border border-cyan-700/80 rounded-xl shadow-lg font-mono flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center space-x-2">
            <div className="p-1.5 bg-cyan-950 text-cyan-400 border border-cyan-700 rounded-lg">
              <Zap className="w-4 h-4" />
            </div>
            <span className="text-xs font-bold text-cyan-400 uppercase tracking-wide">
              Prescriptive Remediation
            </span>
          </div>

          <span className="text-[10px] bg-cyan-950 text-cyan-300 border border-cyan-700 px-2 py-0.5 rounded font-bold">
            CONFIDENCE: {confidence}%
          </span>
        </div>

        <h3 className="font-bold text-sm text-white mb-1.5">{title}</h3>
        <p className="text-xs text-slate-400 mb-4">{description}</p>

        <div className="text-[11px] text-slate-300 bg-[#07090E] p-2.5 rounded-lg border border-[#1E293B] mb-4">
          <span className="text-slate-500 font-medium">Target Workload: </span>
          <span className="text-white font-bold">{targetService}</span>
        </div>
      </div>

      <button
        onClick={handleAction}
        disabled={isExecuting || done}
        className={`w-full py-2.5 px-4 rounded-lg text-xs font-bold font-mono border transition-all flex items-center justify-center space-x-2 shadow-md ${
          done
            ? 'bg-emerald-950 text-emerald-300 border-emerald-700'
            : 'bg-cyan-700 hover:bg-cyan-600 text-white border-cyan-500'
        }`}
      >
        {done ? (
          <>
            <CheckCircle2 className="w-4 h-4 text-emerald-400" />
            <span>RECOVERY ACTION APPLIED</span>
          </>
        ) : (
          <>
            <ShieldCheck className="w-4 h-4" />
            <span>EXECUTE AUTOMATED RECOVERY</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </>
        )}
      </button>
    </div>
  );
};
