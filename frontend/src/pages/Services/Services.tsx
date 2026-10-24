import React, { useState, useEffect, useMemo } from 'react';
import {
  Server,
  Search,
  Plus,
  RefreshCw,
  Activity,
  Trash2,
  Edit3,
  Shield,
  AlertTriangle,
  Upload,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Database,
  HardDrive,
  Layers,
} from 'lucide-react';
import { registryApi, type TopologyServiceItem, type MonitoringMode } from '../../api/registry';
import { ServiceWizardModal } from '../../components/ServiceWizard/ServiceWizardModal';
import { AutoRemediateConfirmModal } from '../../components/GraphBuilder/AutoRemediateConfirmModal';
import { ImportExportModal } from '../../components/GraphBuilder/ImportExportModal';
import { useAuthStore } from '../../store/useAuthStore';
import { useTelemetryStore } from '../../store/useTelemetryStore';

export const Services: React.FC = () => {
  const { user } = useAuthStore();
  const isAdmin = user?.role === 'admin';
  const isOperator = user?.role === 'operator' || isAdmin;

  const { tickTelemetry } = useTelemetryStore();

  const [services, setServices] = useState<TopologyServiceItem[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [filterMode, setFilterMode] = useState<string>('ALL');
  const [filterStatus, setFilterStatus] = useState<string>('ALL');
  const [filterTier, setFilterTier] = useState<string>('ALL');

  // Pagination
  const [currentPage, setCurrentPage] = useState<number>(1);
  const pageSize = 15;

  // Modals
  const [wizardOpen, setWizardOpen] = useState<boolean>(false);
  const [editingService, setEditingService] = useState<TopologyServiceItem | null>(null);
  const [autoRemediateTarget, setAutoRemediateTarget] = useState<TopologyServiceItem | null>(null);
  const [importExportOpen, setImportExportOpen] = useState<boolean>(false);

  // Testing connection indicator
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testNotice, setTestNotice] = useState<{ id: string; success: boolean; msg: string } | null>(null);

  const loadServices = async () => {
    setLoading(true);
    try {
      const data = await registryApi.listServices();
      setServices(data || []);
    } catch (err) {
      console.error('Failed to load services:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadServices();
  }, []);

  const handleTestConnection = async (svc: TopologyServiceItem) => {
    setTestingId(svc.id);
    setTestNotice(null);
    try {
      const res = await registryApi.testConnection(svc.id);
      if (res.success) {
        setTestNotice({
          id: svc.id,
          success: true,
          msg: `Connected! Discovered: ${res.metricsFound?.join(', ')}`,
        });
      } else {
        setTestNotice({
          id: svc.id,
          success: false,
          msg: `Failed: ${res.errors?.join('; ') || 'No response'}`,
        });
      }
      await loadServices();
    } catch (err: any) {
      setTestNotice({
        id: svc.id,
        success: false,
        msg: err.message || 'Connection test failed',
      });
    } finally {
      setTestingId(null);
      setTimeout(() => setTestNotice(null), 5000);
    }
  };

  const handleDeleteService = async (id: string, name: string) => {
    if (!confirm(`Are you sure you want to delete service "${name}" (${id}) from the catalog?`)) return;
    try {
      await registryApi.deleteService(id);
      await loadServices();
      await tickTelemetry();
    } catch (err: any) {
      alert(`Delete failed: ${err.message}`);
    }
  };

  const handleQuickModeChange = async (svc: TopologyServiceItem, newMode: MonitoringMode) => {
    if (newMode === 'auto_remediate') {
      if (!isAdmin) {
        alert('Permission Denied: Only administrators can enable auto_remediate mode.');
        return;
      }
      if (!svc.recoveryBinding || svc.recoveryBinding.allowedActions?.length === 0) {
        alert('Cannot enable auto-remediation: Service has no recovery binding configured.');
        return;
      }
      setAutoRemediateTarget(svc);
      return;
    }

    try {
      await registryApi.updateService(svc.id, { monitoringMode: newMode });
      await loadServices();
    } catch (err: any) {
      alert(`Mode update failed: ${err.message}`);
    }
  };

  const confirmAutoRemediate = async () => {
    if (!autoRemediateTarget) return;
    try {
      await registryApi.updateService(autoRemediateTarget.id, { monitoringMode: 'auto_remediate' });
      await loadServices();
    } catch (err: any) {
      alert(`Failed to set auto-remediate: ${err.message}`);
    }
  };

  const filteredServices = useMemo(() => {
    return services.filter((s) => {
      if (filterMode !== 'ALL' && s.monitoringMode !== filterMode) return false;
      if (filterStatus !== 'ALL') {
        const status = s.status || 'unconnected';
        if (filterStatus === 'CONNECTED' && status === 'unconnected') return false;
        if (filterStatus === 'UNCONNECTED' && status !== 'unconnected') return false;
        if (filterStatus === 'HEALTHY' && status !== 'healthy' && status !== 'healthy_data') return false;
      }
      if (filterTier !== 'ALL' && s.tier !== filterTier) return false;
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        return (
          s.name.toLowerCase().includes(q) ||
          s.id.toLowerCase().includes(q) ||
          (s.namespace && s.namespace.toLowerCase().includes(q)) ||
          (s.owner && s.owner.toLowerCase().includes(q))
        );
      }
      return true;
    });
  }, [services, filterMode, filterStatus, filterTier, searchQuery]);

  const totalPages = Math.ceil(filteredServices.length / pageSize) || 1;
  const paginatedServices = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredServices.slice(start, start + pageSize);
  }, [filteredServices, currentPage, pageSize]);

  const getTypeIcon = (type?: string) => {
    switch (type) {
      case 'database': return <Database className="w-3.5 h-3.5 text-sky-400" />;
      case 'cache': return <HardDrive className="w-3.5 h-3.5 text-emerald-400" />;
      case 'queue': return <Layers className="w-3.5 h-3.5 text-amber-400" />;
      default: return <Server className="w-3.5 h-3.5 text-purple-400" />;
    }
  };

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#26262B] pb-4">
        <div>
          <div className="flex items-center space-x-2.5">
            <div className="p-2 rounded-lg bg-[#840032]/25 text-[#e2588a] border border-[#840032]">
              <Server className="w-5 h-5" />
            </div>
            <div>
              <h1 className="text-xl font-bold text-white tracking-tight">
                Service Catalog & Dependency Builder
              </h1>
              <p className="text-xs text-gray-400">
                Register services, connect real-time telemetry sources, define recovery boundaries, and assign monitoring policies
              </p>
            </div>
          </div>
        </div>

        {/* Action Controls */}
        <div className="flex items-center space-x-2.5">
          <button
            onClick={() => setImportExportOpen(true)}
            className="px-3 py-1.5 rounded-md bg-[#151517] hover:bg-[#202024] border border-[#26262B] text-gray-300 hover:text-white flex items-center space-x-1.5 text-xs font-medium transition-colors"
          >
            <Upload className="w-3.5 h-3.5 text-gray-400" />
            <span>Import / Export</span>
          </button>

          <button
            onClick={loadServices}
            className="p-2 rounded-md bg-[#151517] hover:bg-[#202024] border border-[#26262B] text-gray-400 hover:text-white transition-colors"
            title="Refresh services"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-[#e2588a]' : ''}`} />
          </button>

          {isOperator && (
            <button
              onClick={() => {
                setEditingService(null);
                setWizardOpen(true);
              }}
              className="px-4 py-1.5 rounded-md bg-[#840032] hover:bg-[#a80f49] text-white flex items-center space-x-1.5 text-xs font-semibold shadow transition-colors"
            >
              <Plus className="w-4 h-4" />
              <span>Add Service</span>
            </button>
          )}
        </div>
      </div>

      {/* Filter and Search Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 bg-[#151517] p-3 rounded-lg border border-[#26262B]">
        <div className="flex flex-wrap items-center gap-2">
          {/* Search Input */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-2.5" />
            <input
              type="text"
              placeholder="Filter by name, ID, owner..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="bg-[#0B0B0C] border border-[#26262B] rounded-md pl-7 pr-3 py-1.5 text-xs text-gray-200 focus:outline-none focus:border-[#840032] w-48 sm:w-64"
            />
          </div>

          {/* Mode Filter */}
          <select
            value={filterMode}
            onChange={(e) => setFilterMode(e.target.value)}
            className="bg-[#0B0B0C] border border-[#26262B] rounded-md px-2.5 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-[#840032]"
          >
            <option value="ALL">All Modes</option>
            <option value="observe">Observe</option>
            <option value="recommend">Recommend</option>
            <option value="auto_remediate">Auto-Remediate</option>
            <option value="ignore">Ignore</option>
          </select>

          {/* Status Filter */}
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bg-[#0B0B0C] border border-[#26262B] rounded-md px-2.5 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-[#840032]"
          >
            <option value="ALL">All Statuses</option>
            <option value="CONNECTED">Connected</option>
            <option value="UNCONNECTED">Unconnected</option>
            <option value="HEALTHY">Healthy Data</option>
          </select>

          {/* Tier Filter */}
          <select
            value={filterTier}
            onChange={(e) => setFilterTier(e.target.value)}
            className="bg-[#0B0B0C] border border-[#26262B] rounded-md px-2.5 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-[#840032]"
          >
            <option value="ALL">All Tiers</option>
            <option value="tier-1">Tier 1</option>
            <option value="tier-2">Tier 2</option>
            <option value="tier-3">Tier 3</option>
          </select>
        </div>

        <div className="text-xs text-gray-400 font-mono">
          Showing <span className="text-white font-semibold">{filteredServices.length}</span> services
        </div>
      </div>

      {/* Test Notice Toast */}
      {testNotice && (
        <div
          className={`p-3 rounded-lg text-xs flex items-center space-x-2 border transition-all ${testNotice.success
              ? 'bg-emerald-950/30 border-emerald-800 text-emerald-300'
              : 'bg-rose-950/30 border-rose-800 text-rose-300'
            }`}
        >
          {testNotice.success ? <CheckCircle2 className="w-4 h-4 text-emerald-400" /> : <AlertTriangle className="w-4 h-4 text-rose-400" />}
          <span className="font-semibold">{testNotice.id}:</span>
          <span>{testNotice.msg}</span>
        </div>
      )}

      {/* Empty State / Onboarding */}
      {services.length === 0 && !loading && (
        <div className="p-8 rounded-xl bg-[#151517] border border-[#26262B] text-center max-w-xl mx-auto space-y-4">
          <div className="w-12 h-12 rounded-full bg-[#840032]/20 border border-[#840032] flex items-center justify-center mx-auto text-[#e2588a]">
            <Server className="w-6 h-6" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white">No Microservices Registered</h2>
            <p className="text-xs text-gray-400 max-w-sm mx-auto mt-1">
              Start building your self-healing distributed system mesh in four straightforward steps:
            </p>
          </div>

          <div className="p-4 bg-[#0B0B0C] border border-[#26262B] rounded-lg text-left text-xs space-y-2 max-w-md mx-auto">
            <div className="flex items-center space-x-2 text-gray-300">
              <span className="w-5 h-5 rounded-full bg-[#26262B] text-white flex items-center justify-center text-[10px] font-bold">1</span>
              <span>Add services to your mesh catalog</span>
            </div>
            <div className="flex items-center space-x-2 text-gray-300">
              <span className="w-5 h-5 rounded-full bg-[#26262B] text-white flex items-center justify-center text-[10px] font-bold">2</span>
              <span>Connect live telemetry (Prometheus / Kubernetes / Agent)</span>
            </div>
            <div className="flex items-center space-x-2 text-gray-300">
              <span className="w-5 h-5 rounded-full bg-[#26262B] text-white flex items-center justify-center text-[10px] font-bold">3</span>
              <span>Draw service dependency graph</span>
            </div>
            <div className="flex items-center space-x-2 text-gray-300">
              <span className="w-5 h-5 rounded-full bg-[#26262B] text-white flex items-center justify-center text-[10px] font-bold">4</span>
              <span>Choose monitoring modes & recovery bindings</span>
            </div>
          </div>

          <div className="flex items-center justify-center space-x-3 pt-2">
            <button
              onClick={() => {
                setEditingService(null);
                setWizardOpen(true);
              }}
              className="px-4 py-2 rounded-md bg-[#840032] hover:bg-[#a80f49] text-white font-semibold text-xs flex items-center space-x-1.5 shadow"
            >
              <Plus className="w-4 h-4" />
              <span>Add Your First Service</span>
            </button>
            <button
              onClick={() => setImportExportOpen(true)}
              className="px-4 py-2 rounded-md bg-[#26262B] hover:bg-[#34343B] text-gray-200 text-xs font-semibold flex items-center space-x-1.5"
            >
              <Upload className="w-3.5 h-3.5" />
              <span>Import Topology YAML</span>
            </button>
          </div>
        </div>
      )}

      {/* Services Table */}
      {services.length > 0 && (
        <div className="bg-[#151517] border border-[#26262B] rounded-lg overflow-hidden shadow-lg">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs text-gray-300 divide-y divide-[#26262B]">
              <thead className="bg-[#0B0B0C] text-[11px] font-semibold text-gray-400 uppercase tracking-wider select-none">
                <tr>
                  <th className="px-4 py-3">Workload Name & ID</th>
                  <th className="px-3 py-3">Namespace</th>
                  <th className="px-3 py-3">Tier</th>
                  <th className="px-3 py-3">Telemetry Engine</th>
                  <th className="px-3 py-3">Monitoring Mode</th>
                  <th className="px-3 py-3">Health & Status</th>
                  <th className="px-3 py-3">Recovery Binding</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#26262B] divide-opacity-60">
                {paginatedServices.map((svc) => {
                  const isUnconnected = !svc.telemetrySource || svc.status === 'unconnected';
                  const isAuto = svc.monitoringMode === 'auto_remediate';
                  const isRec = svc.monitoringMode === 'recommend';
                  const isIgn = svc.monitoringMode === 'ignore';

                  return (
                    <tr key={svc.id} className="hover:bg-[#18181B] transition-colors">
                      {/* Name & ID */}
                      <td className="px-4 py-3">
                        <div className="flex items-center space-x-2.5">
                          <div className="p-1.5 rounded bg-[#0B0B0C] border border-[#26262B] shrink-0">
                            {getTypeIcon(svc.type)}
                          </div>
                          <div className="min-w-0">
                            <div className="font-semibold text-white truncate">{svc.name}</div>
                            <div className="font-mono text-[10px] text-gray-400 truncate">{svc.id}</div>
                          </div>
                        </div>
                      </td>

                      {/* Namespace */}
                      <td className="px-3 py-3 font-mono text-[11px] text-gray-400">
                        {svc.namespace || 'default'}
                      </td>

                      {/* Tier */}
                      <td className="px-3 py-3">
                        <span className="font-mono uppercase text-[10px] px-1.5 py-0.5 rounded bg-[#0B0B0C] border border-[#26262B] text-gray-300">
                          {svc.tier || 'tier-2'}
                        </span>
                      </td>

                      {/* Telemetry Source */}
                      <td className="px-3 py-3">
                        {svc.telemetrySource ? (
                          <div className="font-mono text-[11px] text-gray-200 flex items-center space-x-1.5">
                            <span className="capitalize">{svc.telemetrySource.type}</span>
                            {svc.telemetrySource.type === 'agent_push' && (
                              <span className="text-[9px] px-1 py-0.2 rounded bg-purple-950 border border-purple-800 text-purple-300">
                                PUSH
                              </span>
                            )}
                          </div>
                        ) : (
                          <span className="text-amber-400 font-medium italic">None</span>
                        )}
                      </td>

                      {/* Monitoring Mode Controls */}
                      <td className="px-3 py-3">
                        <div className="flex items-center space-x-1">
                          <select
                            disabled={!isOperator}
                            value={svc.monitoringMode || 'observe'}
                            onChange={(e) => handleQuickModeChange(svc, e.target.value as MonitoringMode)}
                            className={`rounded px-2 py-1 text-[11px] font-semibold border bg-[#0B0B0C] focus:outline-none ${isAuto
                                ? 'border-emerald-600/70 text-emerald-300'
                                : isRec
                                  ? 'border-amber-600/70 text-amber-300'
                                  : isIgn
                                    ? 'border-gray-700 text-gray-400'
                                    : 'border-blue-600/70 text-blue-300'
                              }`}
                          >
                            <option value="observe">Observe</option>
                            <option value="recommend">Recommend</option>
                            <option value="auto_remediate" disabled={!isAdmin || !svc.recoveryBinding}>
                              Auto-Remediate {!isAdmin ? '(Admin)' : !svc.recoveryBinding ? '(No Binding)' : ''}
                            </option>
                            <option value="ignore">Ignore</option>
                          </select>
                        </div>
                      </td>

                      {/* Status */}
                      <td className="px-3 py-3">
                        {isUnconnected ? (
                          <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-amber-500/10 border border-amber-500/40 text-amber-300">
                            <AlertTriangle className="w-3 h-3" />
                            <span>Unconnected</span>
                          </span>
                        ) : (
                          <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-500/10 border border-emerald-500/40 text-emerald-300">
                            <CheckCircle2 className="w-3 h-3" />
                            <span>Healthy Data</span>
                          </span>
                        )}
                      </td>

                      {/* Recovery Binding */}
                      <td className="px-3 py-3">
                        {svc.recoveryBinding ? (
                          <div className="flex items-center space-x-1 text-emerald-400 text-[11px]">
                            <Shield className="w-3.5 h-3.5 shrink-0" />
                            <span className="font-mono text-[10px] truncate max-w-28" title={`${svc.recoveryBinding.runtime}: ${svc.recoveryBinding.target}`}>
                              {svc.recoveryBinding.target}
                            </span>
                          </div>
                        ) : (
                          <span className="text-gray-400 text-[10px]">Observe-only</span>
                        )}
                      </td>

                      {/* Row Actions */}
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end space-x-1">
                          <button
                            type="button"
                            onClick={() => handleTestConnection(svc)}
                            disabled={testingId === svc.id}
                            className="p-1.5 rounded hover:bg-[#26262B] text-gray-400 hover:text-white transition-colors"
                            title="Test telemetry source connection"
                          >
                            <Activity className={`w-3.5 h-3.5 ${testingId === svc.id ? 'animate-spin text-[#e2588a]' : ''}`} />
                          </button>

                          {isOperator && (
                            <button
                              type="button"
                              onClick={() => {
                                setEditingService(svc);
                                setWizardOpen(true);
                              }}
                              className="p-1.5 rounded hover:bg-[#26262B] text-gray-400 hover:text-white transition-colors"
                              title="Edit service details"
                            >
                              <Edit3 className="w-3.5 h-3.5" />
                            </button>
                          )}

                          {isOperator && (
                            <button
                              type="button"
                              onClick={() => handleDeleteService(svc.id, svc.name)}
                              className="p-1.5 rounded hover:bg-rose-950/40 text-gray-400 hover:text-rose-400 transition-colors"
                              title="Delete service"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Pagination Footer */}
          {totalPages > 1 && (
            <div className="px-4 py-3 bg-[#0B0B0C] border-t border-[#26262B] flex items-center justify-between text-xs text-gray-400">
              <div>
                Page <span className="font-semibold text-white">{currentPage}</span> of{' '}
                <span className="font-semibold text-white">{totalPages}</span>
              </div>
              <div className="flex items-center space-x-1.5">
                <button
                  disabled={currentPage <= 1}
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  className="p-1 rounded bg-[#151517] border border-[#26262B] disabled:opacity-40 hover:text-white"
                >
                  <ChevronLeft className="w-4 h-4" />
                </button>
                <button
                  disabled={currentPage >= totalPages}
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  className="p-1 rounded bg-[#151517] border border-[#26262B] disabled:opacity-40 hover:text-white"
                >
                  <ChevronRight className="w-4 h-4" />
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Service Wizard Modal */}
      <ServiceWizardModal
        isOpen={wizardOpen}
        onClose={() => {
          setWizardOpen(false);
          setEditingService(null);
        }}
        onSaved={loadServices}
        editService={editingService}
      />

      {/* Auto-Remediate Confirmation Modal */}
      <AutoRemediateConfirmModal
        isOpen={Boolean(autoRemediateTarget)}
        onClose={() => setAutoRemediateTarget(null)}
        service={autoRemediateTarget}
        blastRadius={autoRemediateTarget?.tags || []}
        onConfirm={confirmAutoRemediate}
      />

      {/* Import / Export Modal */}
      <ImportExportModal
        isOpen={importExportOpen}
        onClose={() => setImportExportOpen(false)}
        onRefreshTopology={loadServices}
      />
    </div>
  );
};
