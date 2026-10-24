import React, { useState, useEffect } from 'react';
import {
  Server,
  Activity,
  CheckCircle2,
  AlertTriangle,
  ArrowRight,
  ArrowLeft,
  X,
  Zap,
  Eye,
  Sparkles,
  EyeOff,
  Copy,
  Check,
} from 'lucide-react';
import {
  registryApi,
  type TopologyServiceItem,
  type TelemetrySource,
  type RecoveryBinding,
  type MonitoringMode,
  type TestConnectionResponse,
} from '../../api/registry';
import { useAuthStore } from '../../store/useAuthStore';

interface ServiceWizardModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSaved: () => Promise<void>;
  editService?: TopologyServiceItem | null;
}

export const ServiceWizardModal: React.FC<ServiceWizardModalProps> = ({
  isOpen,
  onClose,
  onSaved,
  editService,
}) => {
  const { user } = useAuthStore();
  const isAdmin = user?.role === 'admin';

  const [step, setStep] = useState<number>(1);
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  // Form State
  const [id, setId] = useState<string>('');
  const [name, setName] = useState<string>('');
  const [namespace, setNamespace] = useState<string>('default');
  const [tier, setTier] = useState<string>('tier-2');
  const [owner, setOwner] = useState<string>('');
  const [description, setDescription] = useState<string>('');
  const [tagsStr, setTagsStr] = useState<string>('');
  const [type, setType] = useState<string>('service');

  // Telemetry Source State
  const [sourceType, setSourceType] = useState<'prometheus' | 'kubernetes' | 'agent_push'>('prometheus');
  const [baseUrl, setBaseUrl] = useState<string>('http://prometheus.default.svc.cluster.local:9090');
  const [job, setJob] = useState<string>('');
  const [selector, setSelector] = useState<string>('');
  const [cpuQuery, setCpuQuery] = useState<string>('sum(rate(container_cpu_usage_seconds_total{container="{service}"}[5m]))');
  const [memoryQuery, setMemoryQuery] = useState<string>('sum(container_memory_working_set_bytes{container="{service}"})');
  const [latencyQuery, setLatencyQuery] = useState<string>('histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{service="{service}"}[5m])) by (le))');
  const [errorRateQuery, setErrorRateQuery] = useState<string>('sum(rate(http_requests_total{service="{service}",status=~"5.."}[5m])) / sum(rate(http_requests_total{service="{service}"}[5m]))');
  const [queueDepthQuery, setQueueDepthQuery] = useState<string>('');
  const [authToken, setAuthToken] = useState<string>('');
  const [authUsername, setAuthUsername] = useState<string>('');
  const [authPassword, setAuthPassword] = useState<string>('');

  // Kubernetes source
  const [clusterId, setClusterId] = useState<string>('cluster-primary');
  const [k8sNamespace, setK8sNamespace] = useState<string>('default');
  const [workloadKind, setWorkloadKind] = useState<string>('Deployment');
  const [workloadName, setWorkloadName] = useState<string>('');

  // Agent Push
  const [agentKey, setAgentKey] = useState<string>('');
  const [snippetCopied, setSnippetCopied] = useState<boolean>(false);

  // Test Connection State
  const [testResult, setTestResult] = useState<TestConnectionResponse | null>(null);
  const [testing, setTesting] = useState<boolean>(false);

  // Recovery Binding
  const [hasRecovery, setHasRecovery] = useState<boolean>(false);
  const [recoveryRuntime, setRecoveryRuntime] = useState<'kubernetes' | 'docker'>('kubernetes');
  const [recoveryTarget, setRecoveryTarget] = useState<string>('');
  const [allowedActions, setAllowedActions] = useState<string[]>(['scale', 'restart']);

  // Monitoring Mode
  const [monitoringMode, setMonitoringMode] = useState<MonitoringMode>('observe');

  useEffect(() => {
    if (!isOpen) return;
    if (editService) {
      setId(editService.id);
      setName(editService.name);
      setNamespace(editService.namespace || 'default');
      setTier(editService.tier || 'tier-2');
      setOwner(editService.owner || '');
      setDescription(editService.description || '');
      setTagsStr((editService.tags || []).join(', '));
      setType(editService.type || 'service');
      setMonitoringMode(editService.monitoringMode || 'observe');

      if (editService.telemetrySource) {
        setSourceType((editService.telemetrySource.type as any) || 'prometheus');
        setBaseUrl(editService.telemetrySource.baseUrl || '');
        setJob(editService.telemetrySource.job || '');
        setSelector(editService.telemetrySource.selector || '');
        setCpuQuery(editService.telemetrySource.cpuQuery || '');
        setMemoryQuery(editService.telemetrySource.memoryQuery || '');
        setLatencyQuery(editService.telemetrySource.latencyQuery || '');
        setErrorRateQuery(editService.telemetrySource.errorRateQuery || '');
        setQueueDepthQuery(editService.telemetrySource.queueDepthQuery || '');
        setClusterId(editService.telemetrySource.clusterId || 'cluster-primary');
        setK8sNamespace(editService.telemetrySource.namespace || 'default');
        setWorkloadKind(editService.telemetrySource.workloadKind || 'Deployment');
        setWorkloadName(editService.telemetrySource.workloadName || '');
        setAgentKey(editService.telemetrySource.apiKey || '');
      }

      if (editService.recoveryBinding) {
        setHasRecovery(true);
        setRecoveryRuntime((editService.recoveryBinding.runtime as any) || 'kubernetes');
        setRecoveryTarget(editService.recoveryBinding.target);
        setAllowedActions(editService.recoveryBinding.allowedActions || []);
      } else {
        setHasRecovery(false);
      }
    } else {
      // Reset for new service
      setId('');
      setName('');
      setNamespace('default');
      setTier('tier-2');
      setOwner('');
      setDescription('');
      setTagsStr('');
      setType('service');
      setSourceType('prometheus');
      setMonitoringMode('observe');
      setHasRecovery(false);
      setTestResult(null);
      setAgentKey(`aegis-push-${Math.random().toString(36).substring(2, 10)}`);
    }
    setStep(1);
    setErrorMsg(null);
  }, [isOpen, editService]);

  if (!isOpen) return null;

  const buildTelemetrySource = (): TelemetrySource => {
    if (sourceType === 'prometheus') {
      return {
        type: 'prometheus',
        baseUrl: baseUrl.trim(),
        job: job.trim(),
        selector: selector.trim(),
        cpuQuery: cpuQuery.trim(),
        memoryQuery: memoryQuery.trim(),
        latencyQuery: latencyQuery.trim(),
        errorRateQuery: errorRateQuery.trim(),
        queueDepthQuery: queueDepthQuery.trim(),
        authToken: authToken.trim() || undefined,
        authUsername: authUsername.trim() || undefined,
        authPassword: authPassword.trim() || undefined,
      };
    } else if (sourceType === 'kubernetes') {
      return {
        type: 'kubernetes',
        clusterId: clusterId.trim(),
        namespace: k8sNamespace.trim(),
        workloadKind: workloadKind.trim(),
        workloadName: workloadName.trim() || id.trim(),
      };
    } else {
      return {
        type: 'agent_push',
        serviceId: id.trim(),
        apiKey: agentKey || `aegis-key-${id.trim()}`,
      };
    }
  };

  const handleRunTestConnection = async () => {
    setTesting(true);
    setTestResult(null);
    setErrorMsg(null);
    try {
      const src = buildTelemetrySource();
      const res = await registryApi.testConnection(editService ? editService.id : 'test', src);
      setTestResult(res);
      if (!res.success && res.errors && res.errors.length > 0) {
        setErrorMsg(`Test connection failed: ${res.errors.join('; ')}`);
      }
    } catch (err: any) {
      setTestResult({
        success: false,
        errors: [err.message || 'Connection test failed'],
      });
      setErrorMsg(err.message || 'Connection test failed');
    } finally {
      setTesting(false);
    }
  };

  const handleToggleAction = (action: string) => {
    if (allowedActions.includes(action)) {
      setAllowedActions(allowedActions.filter((a) => a !== action));
    } else {
      setAllowedActions([...allowedActions, action]);
    }
  };

  const handleSave = async () => {
    setErrorMsg(null);
    setSubmitting(true);

    try {
      const tags = tagsStr.split(',').map((t) => t.trim()).filter(Boolean);
      const telemetrySource = buildTelemetrySource();
      const recoveryBinding: RecoveryBinding | undefined = hasRecovery
        ? {
            runtime: recoveryRuntime,
            target: recoveryTarget.trim() || `${namespace}/${id}`,
            allowedActions,
          }
        : undefined;

      const payload: TopologyServiceItem = {
        id: id.trim(),
        name: name.trim(),
        type,
        namespace: namespace.trim(),
        clusterId: clusterId.trim() || 'cluster-primary',
        tier,
        owner: owner.trim(),
        description: description.trim(),
        tags,
        monitoringMode,
        telemetrySource,
        recoveryBinding,
      };

      if (editService) {
        await registryApi.updateService(editService.id, payload);
      } else {
        await registryApi.createService(payload);
      }

      await onSaved();
      onClose();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to save service');
    } finally {
      setSubmitting(false);
    }
  };

  const installSnippet = `# Deploy Aegis Telemetry Agent via Docker
docker run -d \\
  --name aegis-agent-${id || 'service'} \\
  -e AEGIS_SERVICE_ID="${id || 'service-id'}" \\
  -e AEGIS_API_KEY="${agentKey}" \\
  -e AEGIS_GATEWAY_URL="http://aegis-gateway:8000/api/telemetry/spans" \\
  ghcr.io/aegis/agent:latest`;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-xs p-4 font-sans animate-in fade-in duration-200">
      <div className="bg-[#151517] border border-[#26262B] rounded-xl shadow-2xl max-w-2xl w-full flex flex-col max-h-[90vh] overflow-hidden text-gray-200">
        {/* Header */}
        <div className="p-4 border-b border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-2.5">
            <div className="p-2 rounded-lg bg-[#840032]/25 text-[#e2588a] border border-[#840032]">
              <Server className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-white">
                {editService ? `Configure Service: ${editService.name}` : 'Add Service to Mesh Catalog'}
              </h2>
              <p className="text-[11px] text-gray-400">
                Define telemetry ingestion, autonomous recovery boundaries, and monitoring modes
              </p>
            </div>
          </div>
          <button onClick={onClose} className="p-1 rounded text-gray-400 hover:text-white hover:bg-gray-800">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Wizard Steps Stepper */}
        <div className="px-6 py-2.5 bg-[#0B0B0C] border-b border-[#26262B] flex items-center justify-between text-xs">
          {[
            { num: 1, label: 'Basics' },
            { num: 2, label: 'Telemetry Source' },
            { num: 3, label: 'Recovery Binding' },
            { num: 4, label: 'Mode' },
            { num: 5, label: 'Review' },
          ].map((s) => (
            <div
              key={s.num}
              onClick={() => {
                if (s.num < step || (id && name)) setStep(s.num);
              }}
              className={`flex items-center space-x-1.5 cursor-pointer select-none ${
                step === s.num
                  ? 'text-[#e2588a] font-semibold'
                  : step > s.num
                  ? 'text-emerald-400 font-medium'
                  : 'text-gray-400'
              }`}
            >
              <div
                className={`w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-bold ${
                  step === s.num
                    ? 'bg-[#840032] text-white'
                    : step > s.num
                    ? 'bg-emerald-600 text-white'
                    : 'bg-[#26262B] text-gray-400'
                }`}
              >
                {step > s.num ? <Check className="w-3 h-3 text-white" /> : s.num}
              </div>
              <span>{s.label}</span>
            </div>
          ))}
        </div>

        {/* Wizard Content Body */}
        <div className="flex-1 overflow-y-auto p-6 text-xs space-y-4">
          {errorMsg && (
            <div className="p-3 bg-rose-950/20 border border-rose-900/50 rounded-lg text-rose-300 flex items-center space-x-2">
              <AlertTriangle className="w-4 h-4 shrink-0 text-rose-400" />
              <span>{errorMsg}</span>
            </div>
          )}

          {/* STEP 1: BASICS */}
          {step === 1 && (
            <div className="space-y-3.5">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Service ID (kebab-case) *</label>
                  <input
                    type="text"
                    disabled={Boolean(editService)}
                    value={id}
                    onChange={(e) => setId(e.target.value.toLowerCase().replace(/[^a-z0-9-_]/g, ''))}
                    placeholder="order-processor"
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032] disabled:opacity-50 font-mono"
                  />
                </div>
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Display Name *</label>
                  <input
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Order Processing Service"
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  />
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Namespace</label>
                  <input
                    type="text"
                    value={namespace}
                    onChange={(e) => setNamespace(e.target.value)}
                    placeholder="default"
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  />
                </div>
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Tier</label>
                  <select
                    value={tier}
                    onChange={(e) => setTier(e.target.value)}
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  >
                    <option value="tier-1">Tier 1 — Mission Critical</option>
                    <option value="tier-2">Tier 2 — Standard Service</option>
                    <option value="tier-3">Tier 3 — Background / Async</option>
                  </select>
                </div>
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Workload Type</label>
                  <select
                    value={type}
                    onChange={(e) => setType(e.target.value)}
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  >
                    <option value="service">Microservice</option>
                    <option value="gateway">API Gateway</option>
                    <option value="database">Database</option>
                    <option value="cache">Cache</option>
                    <option value="queue">Message Queue</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Owner / Team Email</label>
                  <input
                    type="text"
                    value={owner}
                    onChange={(e) => setOwner(e.target.value)}
                    placeholder="payments-team@company.internal"
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  />
                </div>
                <div>
                  <label className="block text-gray-300 font-semibold mb-1">Tags (comma separated)</label>
                  <input
                    type="text"
                    value={tagsStr}
                    onChange={(e) => setTagsStr(e.target.value)}
                    placeholder="payment, pci, grpc"
                    className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                  />
                </div>
              </div>

              <div>
                <label className="block text-gray-300 font-semibold mb-1">Description</label>
                <textarea
                  rows={2}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Handles checkout transactions and authorization calls"
                  className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                />
              </div>
            </div>
          )}

          {/* STEP 2: TELEMETRY SOURCE */}
          {step === 2 && (
            <div className="space-y-4">
              {/* Telemetry type selector */}
              <div>
                <label className="block text-gray-300 font-semibold mb-1.5">Telemetry Ingestion Engine</label>
                <div className="grid grid-cols-3 gap-2">
                  {[
                    { id: 'prometheus', label: 'Prometheus Pull', desc: 'Queries Prometheus / PromQL' },
                    { id: 'kubernetes', label: 'Kubernetes Workload', desc: 'Auto-scrapes cluster metrics' },
                    { id: 'agent_push', label: 'Agent Push (OTel)', desc: 'Sidecar pushes traces/metrics' },
                  ].map((t) => (
                    <button
                      key={t.id}
                      type="button"
                      onClick={() => {
                        setSourceType(t.id as any);
                        setTestResult(null);
                      }}
                      className={`p-3 rounded-lg border text-left transition-all ${
                        sourceType === t.id
                          ? 'bg-[#840032]/20 border-[#840032] text-white ring-1 ring-[#840032]'
                          : 'bg-[#0B0B0C] border-[#26262B] text-gray-400 hover:text-gray-200'
                      }`}
                    >
                      <div className="font-semibold text-xs text-gray-200">{t.label}</div>
                      <div className="text-[10px] text-gray-400 mt-0.5">{t.desc}</div>
                    </button>
                  ))}
                </div>
              </div>

              {/* Source-specific configuration */}
              {sourceType === 'prometheus' && (
                <div className="space-y-3 p-3.5 rounded-lg bg-[#0B0B0C] border border-[#26262B]">
                  <div>
                    <label className="block text-gray-300 font-semibold mb-1">Prometheus Base URL *</label>
                    <input
                      type="text"
                      value={baseUrl}
                      onChange={(e) => setBaseUrl(e.target.value)}
                      placeholder="http://prometheus:9090"
                      className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                    />
                    <p className="text-[10px] text-gray-400 mt-0.5">
                      Security Notice: SSRF protected. Internal metadata and loopbacks are blocked unless admin bypass is enabled.
                    </p>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Job / Selector</label>
                      <input
                        type="text"
                        value={job}
                        onChange={(e) => setJob(e.target.value)}
                        placeholder='job="order-service"'
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Bearer Auth Token (Optional)</label>
                      <input
                        type="password"
                        value={authToken}
                        onChange={(e) => setAuthToken(e.target.value)}
                        placeholder="••••••••••••"
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Basic Auth User (Optional)</label>
                      <input
                        type="text"
                        value={authUsername}
                        onChange={(e) => setAuthUsername(e.target.value)}
                        placeholder="admin"
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Basic Auth Pass (Optional)</label>
                      <input
                        type="password"
                        value={authPassword}
                        onChange={(e) => setAuthPassword(e.target.value)}
                        placeholder="••••••••••••"
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-2 text-[11px]">
                    <div>
                      <label className="block text-gray-400 mb-0.5">CPU PromQL</label>
                      <input
                        type="text"
                        value={cpuQuery}
                        onChange={(e) => setCpuQuery(e.target.value)}
                        className="w-full bg-[#151517] border border-[#26262B] rounded px-2 py-1 text-[10px] text-gray-300 font-mono"
                      />
                    </div>
                    <div>
                      <label className="block text-gray-400 mb-0.5">Latency PromQL</label>
                      <input
                        type="text"
                        value={latencyQuery}
                        onChange={(e) => setLatencyQuery(e.target.value)}
                        className="w-full bg-[#151517] border border-[#26262B] rounded px-2 py-1 text-[10px] text-gray-300 font-mono"
                      />
                    </div>
                  </div>
                </div>
              )}

              {sourceType === 'kubernetes' && (
                <div className="space-y-3 p-3.5 rounded-lg bg-[#0B0B0C] border border-[#26262B]">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Kubernetes Namespace *</label>
                      <input
                        type="text"
                        value={k8sNamespace}
                        onChange={(e) => setK8sNamespace(e.target.value)}
                        placeholder="default"
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Workload Kind</label>
                      <select
                        value={workloadKind}
                        onChange={(e) => setWorkloadKind(e.target.value)}
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                      >
                        <option value="Deployment">Deployment</option>
                        <option value="StatefulSet">StatefulSet</option>
                        <option value="DaemonSet">DaemonSet</option>
                      </select>
                    </div>
                  </div>
                  <div>
                    <label className="block text-gray-300 font-semibold mb-1">Workload Name *</label>
                    <input
                      type="text"
                      value={workloadName}
                      onChange={(e) => setWorkloadName(e.target.value)}
                      placeholder={id || 'my-deployment'}
                      className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                    />
                  </div>
                </div>
              )}

              {sourceType === 'agent_push' && (
                <div className="space-y-3 p-3.5 rounded-lg bg-[#0B0B0C] border border-[#26262B]">
                  <div>
                    <label className="block text-gray-300 font-semibold mb-1">Issued Ingestion API Key</label>
                    <div className="flex items-center space-x-2">
                      <input
                        type="text"
                        readOnly
                        value={agentKey}
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-emerald-400 font-mono select-all"
                      />
                    </div>
                  </div>
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <label className="text-gray-300 font-semibold">Copyable Install Snippet</label>
                      <button
                        type="button"
                        onClick={() => {
                          navigator.clipboard.writeText(installSnippet);
                          setSnippetCopied(true);
                          setTimeout(() => setSnippetCopied(false), 2000);
                        }}
                        className="text-[10px] text-gray-400 hover:text-white flex items-center space-x-1"
                      >
                        {snippetCopied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                        <span>{snippetCopied ? 'Copied' : 'Copy Snippet'}</span>
                      </button>
                    </div>
                    <pre className="p-2.5 rounded bg-black/60 border border-[#26262B] font-mono text-[10px] text-gray-300 overflow-x-auto">
                      {installSnippet}
                    </pre>
                  </div>
                </div>
              )}

              {/* MANDATORY CONNECTION TEST */}
              <div className="p-3.5 rounded-lg bg-[#18181B] border border-[#2E2E35] space-y-2.5">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="font-semibold text-white">Telemetry Source Validation</div>
                    <div className="text-[10px] text-gray-400">
                      Verifies metric availability, sample rate, and SSRF restrictions before saving.
                    </div>
                  </div>
                  <button
                    type="button"
                    disabled={testing}
                    onClick={handleRunTestConnection}
                    className="px-3.5 py-1.5 rounded-md bg-[#840032] hover:bg-[#a80f49] text-white font-semibold flex items-center space-x-1.5 disabled:opacity-50 transition-colors shadow"
                  >
                    <Activity className={`w-3.5 h-3.5 ${testing ? 'animate-spin' : ''}`} />
                    <span>{testing ? 'Testing...' : 'Test Connection'}</span>
                  </button>
                </div>

                {testResult && (
                  <div
                    className={`p-3 rounded-md text-xs border ${
                      testResult.success
                        ? 'bg-emerald-950/20 border-emerald-900/60 text-emerald-200'
                        : 'bg-rose-950/20 border-rose-900/60 text-rose-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2 font-semibold">
                      {testResult.success ? (
                        <>
                          <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                          <span>Connection Successful & Verified</span>
                        </>
                      ) : (
                        <>
                          <AlertTriangle className="w-4 h-4 text-rose-400" />
                          <span>Connection Failed</span>
                        </>
                      )}
                    </div>
                    {testResult.metricsFound && testResult.metricsFound.length > 0 && (
                      <div className="mt-1.5 flex items-center space-x-1.5 text-[11px] text-gray-300">
                        <span className="text-gray-400">Metrics Discovered:</span>
                        {testResult.metricsFound.map((m) => (
                          <span key={m} className="px-1.5 py-0.2 rounded bg-black/40 font-mono text-[10px] text-emerald-300 border border-emerald-900/40">
                            {m}
                          </span>
                        ))}
                      </div>
                    )}
                    {testResult.errors && testResult.errors.length > 0 && (
                      <div className="mt-1.5 text-[11px] text-rose-300 space-y-0.5">
                        {testResult.errors.map((e, idx) => (
                          <div key={idx}>• {e}</div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* STEP 3: RECOVERY BINDING */}
          {step === 3 && (
            <div className="space-y-4">
              <div className="p-3 bg-[#0B0B0C] border border-[#26262B] rounded-lg text-gray-400">
                <span className="font-semibold text-white">Optional Recovery Binding:</span> Connect this service to a container or Kubernetes orchestrator so Aegis can recommend or autonomously execute self-healing actions.
                Services without a recovery binding remain strictly <span className="text-amber-400 font-medium">Observe-Only</span>.
              </div>

              <label className="flex items-center space-x-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={hasRecovery}
                  onChange={(e) => setHasRecovery(e.target.checked)}
                  className="rounded border-gray-700 text-[#840032] focus:ring-[#840032]"
                />
                <span className="text-gray-200 font-semibold">Enable Orchestrator Recovery Binding</span>
              </label>

              {hasRecovery && (
                <div className="p-4 rounded-lg bg-[#0B0B0C] border border-[#26262B] space-y-3.5">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Target Runtime</label>
                      <select
                        value={recoveryRuntime}
                        onChange={(e) => setRecoveryRuntime(e.target.value as any)}
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white focus:outline-none focus:border-[#840032]"
                      >
                        <option value="kubernetes">Kubernetes Adapter</option>
                        <option value="docker">Docker Adapter</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-gray-300 font-semibold mb-1">Target Workload Identifier</label>
                      <input
                        type="text"
                        value={recoveryTarget}
                        onChange={(e) => setRecoveryTarget(e.target.value)}
                        placeholder={recoveryRuntime === 'kubernetes' ? 'default/order-service' : 'order-container'}
                        className="w-full bg-[#151517] border border-[#26262B] rounded-md px-3 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-[#840032]"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-gray-300 font-semibold mb-1.5">Permitted Recovery Actions</label>
                    <div className="grid grid-cols-2 gap-2">
                      {[
                        { id: 'scale', label: 'Scale Replicas', desc: 'Adjust replica count on thread exhaustion' },
                        { id: 'restart', label: 'Restart Pod/Container', desc: 'Restart on fatal deadlock or memory leak' },
                        { id: 'increase_pool', label: 'Increase Connection Pool', desc: 'Expand DB pool during lock pressure' },
                        { id: 'flush_cache', label: 'Flush Cache', desc: 'Evict stale entries on cache inconsistency' },
                        { id: 'reroute', label: 'Reroute Traffic', desc: 'Divert requests to backup node on degraded latency' },
                      ].map((act) => (
                        <label
                          key={act.id}
                          className={`p-2.5 rounded-lg border flex items-start space-x-2 cursor-pointer transition-colors ${
                            allowedActions.includes(act.id)
                              ? 'bg-[#840032]/20 border-[#840032] text-white'
                              : 'bg-[#151517] border-[#26262B] text-gray-400'
                          }`}
                        >
                          <input
                            type="checkbox"
                            checked={allowedActions.includes(act.id)}
                            onChange={() => handleToggleAction(act.id)}
                            className="mt-0.5 rounded border-gray-700 text-[#840032] focus:ring-[#840032]"
                          />
                          <div>
                            <div className="font-semibold text-xs text-gray-200">{act.label}</div>
                            <div className="text-[10px] text-gray-400">{act.desc}</div>
                          </div>
                        </label>
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* STEP 4: MONITORING MODE */}
          {step === 4 && (
            <div className="space-y-3">
              <label className="block text-gray-300 font-semibold mb-1.5">Monitoring & Execution Policy</label>
              <div className="space-y-2.5">
                {[
                  {
                    id: 'observe',
                    label: 'Observe (Telemetry & Predictive Dashboards)',
                    desc: 'Metrics collected and health predicted, but no automated or suggested remediation actions are generated.',
                    icon: Eye,
                    badge: 'Default',
                    allowed: true,
                  },
                  {
                    id: 'recommend',
                    label: 'Recommend (Operator Review Required)',
                    desc: 'Aegis TGNN predicts root-causes and generates topologically-ordered recovery plans for human approval.',
                    icon: Sparkles,
                    badge: 'Operator & Admin',
                    allowed: true,
                  },
                  {
                    id: 'auto_remediate',
                    label: 'Auto-Remediate (Autonomous Self-Healing)',
                    desc: 'Aegis autonomously executes approved recovery plans within blast-radius boundaries without manual intervention.',
                    icon: Zap,
                    badge: 'Admin Only',
                    allowed: isAdmin && hasRecovery && allowedActions.length > 0,
                    restrictedReason: !isAdmin
                      ? 'Requires Administrator role'
                      : !hasRecovery || allowedActions.length === 0
                      ? 'Requires a configured recovery binding with allowed actions'
                      : undefined,
                  },
                  {
                    id: 'ignore',
                    label: 'Ignore (Topology Structure Only)',
                    desc: 'All telemetry is dropped at ingestion. Node remains in graph as an unmonitored dependency for cascade prediction.',
                    icon: EyeOff,
                    badge: 'Structural Node',
                    allowed: true,
                  },
                ].map((m) => {
                  const Icon = m.icon;
                  const isSelected = monitoringMode === m.id;
                  const isDisabled = !m.allowed;

                  return (
                    <div
                      key={m.id}
                      onClick={() => {
                        if (!isDisabled) setMonitoringMode(m.id as any);
                      }}
                      className={`p-3.5 rounded-lg border transition-all ${
                        isDisabled
                          ? 'opacity-40 cursor-not-allowed bg-[#0B0B0C] border-[#26262B]'
                          : isSelected
                          ? 'bg-[#840032]/25 border-[#840032] text-white ring-1 ring-[#840032] cursor-pointer'
                          : 'bg-[#0B0B0C] border-[#26262B] text-gray-300 hover:border-gray-600 cursor-pointer'
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center space-x-2">
                          <Icon className="w-4 h-4 text-[#e2588a]" />
                          <span className="font-semibold text-xs text-white">{m.label}</span>
                        </div>
                        <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-[#26262B] text-gray-300 border border-gray-700">
                          {m.badge}
                        </span>
                      </div>
                      <p className="text-[11px] text-gray-400 mt-1 pl-6 leading-relaxed">{m.desc}</p>
                      {m.restrictedReason && (
                        <div className="mt-1.5 pl-6 text-[10px] text-amber-400 flex items-center space-x-1">
                          <AlertTriangle className="w-3 h-3" />
                          <span>{m.restrictedReason}</span>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* STEP 5: REVIEW */}
          {step === 5 && (
            <div className="space-y-3.5">
              <div className="p-4 rounded-lg bg-[#0B0B0C] border border-[#26262B] space-y-2 text-xs">
                <h3 className="font-semibold text-white text-sm border-b border-[#26262B] pb-2">
                  Service Configuration Review
                </h3>
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <span className="text-gray-400">ID:</span> <span className="font-mono text-white">{id}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Display Name:</span> <span className="text-white font-medium">{name}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Namespace:</span> <span className="font-mono text-white">{namespace}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Tier:</span> <span className="font-mono uppercase text-white">{tier}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Telemetry Engine:</span> <span className="font-semibold text-[#e2588a]">{sourceType}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Monitoring Mode:</span> <span className="font-semibold text-amber-400 uppercase">{monitoringMode}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Recovery Status:</span>{' '}
                    <span className={hasRecovery ? 'text-emerald-400 font-semibold' : 'text-gray-400'}>
                      {hasRecovery ? `${recoveryRuntime} (${allowedActions.join(', ')})` : 'Observe-Only'}
                    </span>
                  </div>
                </div>
              </div>

              {!testResult?.success && monitoringMode !== 'ignore' && (
                <div className="p-3 bg-amber-950/20 border border-amber-900/50 rounded-lg text-amber-300 text-[11px] flex items-center space-x-2">
                  <AlertTriangle className="w-4 h-4 shrink-0 text-amber-400" />
                  <span>
                    Warning: Telemetry source connection has not been verified yet. This service will show as "Unconnected" until telemetry samples arrive.
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Wizard Footer Navigation */}
        <div className="p-4 bg-[#0B0B0C] border-t border-[#26262B] flex items-center justify-between">
          <div>
            {step > 1 && (
              <button
                type="button"
                onClick={() => setStep(step - 1)}
                className="px-3.5 py-1.5 rounded-md bg-[#26262B] hover:bg-[#34343B] text-gray-200 flex items-center space-x-1 text-xs transition-colors"
              >
                <ArrowLeft className="w-3.5 h-3.5" />
                <span>Back</span>
              </button>
            )}
          </div>

          <div className="flex items-center space-x-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3.5 py-1.5 rounded-md text-xs text-gray-400 hover:text-white transition-colors"
            >
              Cancel
            </button>

            {step < 5 ? (
              <button
                type="button"
                disabled={step === 1 && (!id || !name)}
                onClick={() => setStep(step + 1)}
                className="px-4 py-1.5 rounded-md bg-[#840032] hover:bg-[#a80f49] text-white flex items-center space-x-1 text-xs font-semibold disabled:opacity-40 transition-colors shadow"
              >
                <span>Continue</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            ) : (
              <button
                type="button"
                disabled={submitting || !id || !name}
                onClick={handleSave}
                className="px-5 py-1.5 rounded-md bg-emerald-600 hover:bg-emerald-500 text-white flex items-center space-x-1.5 text-xs font-semibold disabled:opacity-40 transition-colors shadow"
              >
                <Check className="w-4 h-4" />
                <span>{submitting ? 'Saving Service...' : editService ? 'Update Service' : 'Save Service'}</span>
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
