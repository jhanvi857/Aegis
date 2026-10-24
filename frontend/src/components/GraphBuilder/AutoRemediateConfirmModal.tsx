import React, { useState } from 'react';
import { ShieldAlert, AlertTriangle, X, Zap } from 'lucide-react';
import type { TopologyServiceItem } from '../../api/registry';

interface AutoRemediateConfirmModalProps {
  isOpen: boolean;
  onClose: () => void;
  service: TopologyServiceItem | null;
  blastRadius: string[];
  onConfirm: () => Promise<void>;
}

export const AutoRemediateConfirmModal: React.FC<AutoRemediateConfirmModalProps> = ({
  isOpen,
  onClose,
  service,
  blastRadius,
  onConfirm,
}) => {
  const [acknowledged, setAcknowledged] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  if (!isOpen || !service) return null;

  const allowedActions = service.recoveryBinding?.allowedActions || [];

  const handleConfirm = async () => {
    if (!acknowledged) return;
    setSubmitting(true);
    try {
      await onConfirm();
      onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-xs p-4 font-sans animate-in fade-in duration-200">
      <div className="bg-[#151517] border border-[#AD2831]/60 rounded-xl shadow-2xl max-w-lg w-full overflow-hidden text-gray-200">
        {/* Header */}
        <div className="p-5 bg-gradient-to-r from-[#AD2831]/20 to-transparent border-b border-[#AD2831]/40 flex items-start justify-between">
          <div className="flex items-center space-x-3">
            <div className="p-2.5 rounded-lg bg-[#AD2831]/25 text-rose-400 border border-[#AD2831]/60">
              <ShieldAlert className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h2 className="text-base font-semibold text-white">Enable Autonomous Remediation</h2>
                <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-[#AD2831] text-white">
                  ADMIN ONLY
                </span>
              </div>
              <p className="text-xs text-rose-300/80 mt-0.5">
                Grant Aegis autonomous execution permissions on live cluster workloads
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body content */}
        <div className="p-5 space-y-4 text-xs">
          {/* Target Service Info */}
          <div className="p-3 rounded-lg bg-[#0B0B0C] border border-[#26262B] space-y-1.5">
            <div className="flex justify-between">
              <span className="text-gray-400">Target Service:</span>
              <span className="font-mono font-semibold text-white">{service.name} ({service.id})</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Runtime Binding:</span>
              <span className="font-mono text-emerald-400">
                {service.recoveryBinding?.runtime} → {service.recoveryBinding?.target}
              </span>
            </div>
          </div>

          {/* Allowed Actions */}
          <div>
            <div className="font-semibold text-gray-300 mb-1.5">Permitted Autonomous Actions:</div>
            <div className="flex flex-wrap gap-1.5">
              {allowedActions.length > 0 ? (
                allowedActions.map((action) => (
                  <span
                    key={action}
                    className="px-2 py-1 rounded bg-[#26262B] border border-gray-700 font-mono text-[11px] text-gray-200 flex items-center space-x-1"
                  >
                    <Zap className="w-3 h-3 text-emerald-400" />
                    <span>{action}</span>
                  </span>
                ))
              ) : (
                <span className="text-amber-400 italic">No allowed actions defined! Configure recovery binding first.</span>
              )}
            </div>
          </div>

          {/* Blast Radius Warning */}
          <div className="p-3 rounded-lg bg-amber-950/20 border border-amber-800/40 space-y-1.5">
            <div className="flex items-center space-x-1.5 text-amber-300 font-semibold">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>Downstream Blast Radius:</span>
            </div>
            <p className="text-gray-300 text-[11px] leading-relaxed">
              If autonomous remediation triggers a restart or traffic reroute on <span className="font-mono text-white">{service.id}</span>, the following dependent microservices may experience temporary transit delays:
            </p>
            <div className="flex flex-wrap gap-1 pt-1">
              {blastRadius.length > 0 ? (
                blastRadius.map((dep) => (
                  <span key={dep} className="px-1.5 py-0.5 rounded bg-black/40 border border-amber-900/50 font-mono text-[10px] text-amber-200">
                    {dep}
                  </span>
                ))
              ) : (
                <span className="text-gray-400 italic">No direct downstream dependents</span>
              )}
            </div>
          </div>

          {/* Acknowledgment Checkbox */}
          <label className="flex items-start space-x-2.5 p-3 rounded-lg bg-[#18181B] border border-[#2E2E35] cursor-pointer hover:border-gray-500 transition-colors">
            <input
              type="checkbox"
              checked={acknowledged}
              onChange={(e) => setAcknowledged(e.target.checked)}
              className="mt-0.5 rounded border-gray-700 text-[#840032] focus:ring-[#840032]"
            />
            <span className="text-[11px] text-gray-300 leading-normal select-none">
              I certify that I am an authorized Administrator and grant Aegis permission to autonomously execute recovery actions on this workload within configured cooldown gates.
            </span>
          </label>
        </div>

        {/* Footer */}
        <div className="p-4 bg-[#0B0B0C] border-t border-[#26262B] flex items-center justify-end space-x-3">
          <button
            type="button"
            onClick={onClose}
            className="px-3.5 py-1.5 rounded-md text-xs text-gray-400 hover:text-white transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!acknowledged || submitting || allowedActions.length === 0}
            onClick={handleConfirm}
            className="flex items-center space-x-1.5 px-4 py-1.5 rounded-md text-xs font-semibold bg-[#AD2831] hover:bg-[#840032] text-white disabled:opacity-40 disabled:cursor-not-allowed shadow transition-colors"
          >
            <Zap className="w-3.5 h-3.5" />
            <span>{submitting ? 'Applying...' : 'Authorize Auto-Remediate'}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
