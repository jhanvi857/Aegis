import React, { useState } from 'react';
import { useTelemetryStore, getMLPrediction, calculateSystemRisk } from '../store/useTelemetryStore';
import { FileText, Copy, X, CheckCircle2 } from 'lucide-react';

interface IncidentReportModalProps {
  onClose: () => void;
}

export const IncidentReportModal: React.FC<IncidentReportModalProps> = ({ onClose }) => {
  const { services, activeFaults, recoveryHistory } = useTelemetryStore();
  const [copied, setCopied] = useState(false);

  const risk = calculateSystemRisk(services, activeFaults);
  const prediction = getMLPrediction(services, activeFaults);

  const activeFault = activeFaults[0];
  const lastRecovery = recoveryHistory[0];

  const reportDate = new Date().toISOString().split('T')[0];
  const reportTime = new Date().toLocaleTimeString();

  const reportMarkdown = `# INCIDENT POST-MORTEM REPORT
Generated: ${reportDate} ${reportTime}
Environment: Production Cluster (us-east-k8s-01)
Incident Status: ${activeFault ? 'ACTIVE INCIDENT' : 'RESOLVED / STABLE'}

## 1. INCIDENT SUMMARY
* Primary Root Cause: ${prediction.rootCauseServiceName} (${prediction.rootCauseServiceId})
* Severity: ${risk >= 80 ? 'P1 CRITICAL' : risk >= 40 ? 'P2 MAJOR' : 'P3 MINOR'}
* System Risk Score: ${risk}%
* Failure Probability Lead Indicator: ${prediction.failureProbability}% (Confidence: ${prediction.confidence}%)

## 2. ROOT CAUSE ISOLATION
${activeFault ? `Fault Injector Active: ${activeFault.title} on ${activeFault.targetServiceName}.` : 'System telemetry indicates prior connection pool degradation.'}
Diagnostics: ${prediction.rootCauseReason}.

## 3. AFFECTED SERVICES (BLAST RADIUS)
${services
  .filter((s) => s.status !== 'healthy')
  .map((s) => `- ${s.name} (${s.id}): Status ${s.status.toUpperCase()}, Latency ${s.latency}ms, CPU ${s.cpu}%`)
  .join('\n') || '- None. All services operating within nominal parameters.'}

## 4. AUTOMATED REMEDIATION & RECOVERY
${lastRecovery ? `- Action Taken: ${lastRecovery.actionTitle}\n- Target: ${lastRecovery.targetService}\n- Execution Time: ${lastRecovery.duration}\n- Risk Before: ${lastRecovery.riskBefore}% -> Risk After: ${lastRecovery.riskAfter}%` : '- Prescriptive Action Recommended: ' + prediction.recommendedAction}

## 5. LESSONS LEARNED & ACTION ITEMS
- [ ] Auto-scaling threshold adjusted for ${prediction.rootCauseServiceName}.
- [ ] Redis Sentinel failover health check interval reduced to 2 seconds.
- [ ] ML failure prediction rule set updated with high accuracy weight.
`;

  const handleCopy = () => {
    navigator.clipboard.writeText(reportMarkdown);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 font-mono">
      <div className="bg-[#0F172A] border border-[#1E293B] rounded-2xl max-w-2xl w-full overflow-hidden shadow-2xl flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="bg-[#07090E] px-6 py-4 border-b border-[#1E293B] flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <FileText className="w-5 h-5 text-sky-400" />
            <h2 className="font-bold text-white text-sm uppercase tracking-wider">
              Incident Post-Mortem Report Generator
            </h2>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-white">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 overflow-y-auto space-y-4 text-xs">
          <div className="bg-[#07090E] border border-[#1E293B] p-4 rounded-xl text-slate-200 whitespace-pre-wrap font-mono leading-relaxed max-h-96 overflow-y-auto">
            {reportMarkdown}
          </div>
        </div>

        {/* Footer */}
        <div className="bg-[#07090E] px-6 py-4 border-t border-[#1E293B] flex items-center justify-between">
          <span className="text-[11px] text-slate-400">
            Formatted as GitHub Markdown Post-Mortem Standard
          </span>

          <div className="flex items-center space-x-3">
            <button
              onClick={handleCopy}
              className="flex items-center space-x-1.5 px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-xs font-bold border border-sky-400 transition-colors shadow-md"
            >
              {copied ? (
                <>
                  <CheckCircle2 className="w-4 h-4 text-emerald-300" />
                  <span>REPORT COPIED</span>
                </>
              ) : (
                <>
                  <Copy className="w-4 h-4" />
                  <span>COPY MARKDOWN</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
