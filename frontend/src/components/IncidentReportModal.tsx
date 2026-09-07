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
Environment: Production Cluster (aegis-mesh-01)
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
- [ ] Failover health check interval reduced to 2 seconds for affected tier.
- [ ] ML failure prediction rule set updated with high accuracy weight.
`;

  const handleCopy = () => {
    navigator.clipboard.writeText(reportMarkdown);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 bg-black/85 backdrop-blur-sm z-50 flex items-center justify-center p-4 font-sans">
      <div className="bg-[#151517] border border-[#26262B] rounded-[10px] max-w-2xl w-full overflow-hidden shadow-2xl flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="bg-[#0B0B0C] px-5 py-3.5 border-b border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <FileText className="w-4 h-4 text-[#e2588a]" />
            <h2 className="font-semibold text-white text-[14px] uppercase tracking-wide">
              Incident Post-Mortem Report
            </h2>
          </div>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content */}
        <div className="p-4 overflow-y-auto space-y-3 text-xs">
          <div className="bg-[#0B0B0C] border border-[#26262B] p-3.5 rounded-lg text-gray-200 whitespace-pre-wrap font-mono text-[11px] leading-relaxed max-h-96 overflow-y-auto">
            {reportMarkdown}
          </div>
        </div>

        {/* Footer */}
        <div className="bg-[#0B0B0C] px-5 py-3 border-t border-[#26262B] flex items-center justify-between">
          <span className="text-[11px] text-gray-400 font-normal">
            Aegis Incident Standard Format
          </span>

          <div className="flex items-center space-x-3">
            <button
              onClick={handleCopy}
              className="flex items-center space-x-1.5 px-3.5 py-1.5 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors shadow-sm"
            >
              {copied ? (
                <>
                  <CheckCircle2 className="w-3.5 h-3.5 text-[#52b767]" />
                  <span>COPIED</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5" />
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
