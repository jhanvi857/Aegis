import React from 'react';
import {
  AlertOctagon,
  AlertTriangle,
  Info,
  CheckCircle2,
  RefreshCw,
  X,
  Crosshair,
} from 'lucide-react';
import type { ValidationResponse, ValidationIssue } from '../../api/registry';

interface ValidationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  validation: ValidationResponse | null;
  isLoading: boolean;
  onRefresh: () => void;
  onFocusNode: (serviceId: string) => void;
  onFocusEdge?: (sourceId: string, targetId: string) => void;
}

export const ValidationDrawer: React.FC<ValidationDrawerProps> = ({
  isOpen,
  onClose,
  validation,
  isLoading,
  onRefresh,
  onFocusNode,
}) => {
  if (!isOpen) return null;

  const issues = validation?.issues || [];

  const getIssueIcon = (type: string) => {
    switch (type) {
      case 'error':
        return <AlertOctagon className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />;
      case 'warning':
        return <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />;
      default:
        return <Info className="w-4 h-4 text-sky-400 shrink-0 mt-0.5" />;
    }
  };

  return (
    <aside className="w-80 sm:w-96 bg-[#151517] border-l border-[#26262B] flex flex-col h-full font-sans shadow-2xl z-20 shrink-0">
      {/* Header */}
      <div className="p-4 border-b border-[#26262B] flex items-center justify-between">
        <div>
          <div className="flex items-center space-x-2">
            <h3 className="text-sm font-semibold text-white">Topology Health & Validation</h3>
            {validation && (
              <span
                className={`text-[10px] px-2 py-0.5 rounded-full font-bold uppercase ${
                  validation.valid
                    ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30'
                    : 'bg-rose-500/15 text-rose-400 border border-rose-500/30'
                }`}
              >
                {validation.valid ? 'Valid DAG' : 'Issues Found'}
              </span>
            )}
          </div>
          <p className="text-[11px] text-gray-400 mt-0.5">
            Detects cycles, self-loops, unmonitored dependencies & missing bindings
          </p>
        </div>
        <div className="flex items-center space-x-1">
          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="p-1.5 text-gray-400 hover:text-white hover:bg-gray-800 rounded transition-colors"
            title="Re-validate topology"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin text-purple-400' : ''}`} />
          </button>
          <button
            onClick={onClose}
            className="p-1.5 text-gray-400 hover:text-white hover:bg-gray-800 rounded transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Summary Bar */}
      {validation && (
        <div className="grid grid-cols-4 gap-1 p-3 bg-[#0B0B0C] border-b border-[#26262B] text-center text-xs">
          <div className="p-1.5 rounded bg-[#18181B] border border-[#26262B]">
            <div className="text-[10px] text-gray-400">Services</div>
            <div className="font-mono font-semibold text-white">{validation.summary.serviceCount}</div>
          </div>
          <div className="p-1.5 rounded bg-rose-950/20 border border-rose-900/40">
            <div className="text-[10px] text-rose-400 font-medium">Errors</div>
            <div className="font-mono font-semibold text-rose-300">{validation.summary.errorCount}</div>
          </div>
          <div className="p-1.5 rounded bg-amber-950/20 border border-amber-900/40">
            <div className="text-[10px] text-amber-400 font-medium">Warnings</div>
            <div className="font-mono font-semibold text-amber-300">{validation.summary.warningCount}</div>
          </div>
          <div className="p-1.5 rounded bg-purple-950/20 border border-purple-900/40">
            <div className="text-[10px] text-purple-400 font-medium">SCC Cycles</div>
            <div className="font-mono font-semibold text-purple-300">{validation.summary.sccCycles}</div>
          </div>
        </div>
      )}

      {/* Issues List */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2.5">
        {issues.length === 0 ? (
          <div className="py-12 text-center text-gray-400">
            <CheckCircle2 className="w-8 h-8 text-emerald-500 mx-auto mb-2 opacity-80" />
            <div className="text-xs font-semibold text-gray-300">Clean Topology Graph</div>
            <p className="text-[11px] text-gray-400 max-w-xs mx-auto mt-1">
              No orphan nodes, duplicate IDs, self-loops, or cycles detected. Ready for autonomous recovery planning.
            </p>
          </div>
        ) : (
          issues.map((issue: ValidationIssue, idx: number) => {
            const hasFocus = Boolean(issue.serviceId || issue.sourceId);
            return (
              <div
                key={idx}
                className={`p-3 rounded-lg border text-xs transition-all ${
                  issue.type === 'error'
                    ? 'bg-rose-950/20 border-rose-900/50 text-rose-200'
                    : issue.type === 'warning'
                    ? 'bg-amber-950/20 border-amber-900/50 text-amber-200'
                    : 'bg-sky-950/20 border-sky-900/50 text-sky-200'
                }`}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-start space-x-2 min-w-0">
                    {getIssueIcon(issue.type)}
                    <div className="min-w-0">
                      <div className="font-mono text-[10px] uppercase font-bold tracking-wider opacity-80">
                        {issue.code}
                      </div>
                      <div className="text-[12px] font-medium text-gray-200 mt-0.5 leading-snug">
                        {issue.message}
                      </div>
                    </div>
                  </div>

                  {hasFocus && (
                    <button
                      onClick={() => {
                        if (issue.serviceId) onFocusNode(issue.serviceId);
                        else if (issue.sourceId) onFocusNode(issue.sourceId);
                      }}
                      className="shrink-0 p-1 rounded bg-[#26262B] hover:bg-[#840032] text-gray-300 hover:text-white transition-colors flex items-center space-x-1"
                      title="Center and focus on canvas"
                    >
                      <Crosshair className="w-3 h-3" />
                      <span className="text-[10px]">Focus</span>
                    </button>
                  )}
                </div>

                {issue.details && issue.details.length > 0 && (
                  <div className="mt-2 pt-2 border-t border-gray-800/60 font-mono text-[10px] text-gray-400 space-y-0.5">
                    {issue.details.map((d, dIdx) => (
                      <div key={dIdx} className="truncate">• {d}</div>
                    ))}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </aside>
  );
};
