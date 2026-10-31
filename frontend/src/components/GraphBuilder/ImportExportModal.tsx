import React, { useState, useEffect } from 'react';
import {
  Download,
  Upload,
  History,
  Check,
  AlertTriangle,
  RotateCcw,
  X,
  Plus,
  Edit2,
  Trash2,
  Copy,
} from 'lucide-react';
import { registryApi, type TopologyVersionItem, type ImportPreviewDiff } from '../../api/registry';

interface ImportExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  onRefreshTopology: () => Promise<void>;
}

export const ImportExportModal: React.FC<ImportExportModalProps> = ({
  isOpen,
  onClose,
  onRefreshTopology,
}) => {
  const [activeTab, setActiveTab] = useState<'export' | 'import' | 'history'>('import');

  // Export State
  const [exportFormat, setExportFormat] = useState<'yaml' | 'json' | 'csv'>('yaml');
  const [exportContent, setExportContent] = useState<string>('');
  const [exportLoading, setExportLoading] = useState<boolean>(false);
  const [copied, setCopied] = useState<boolean>(false);

  // Import State
  const [importText, setImportText] = useState<string>('');
  const [importing, setImporting] = useState<boolean>(false);
  const [dryRunLoading, setDryRunLoading] = useState<boolean>(false);
  const [dryRunDiff, setDryRunDiff] = useState<ImportPreviewDiff | null>(null);
  const [importError, setImportError] = useState<string | null>(null);
  const [importSuccessMsg, setImportSuccessMsg] = useState<string | null>(null);

  // Version History State
  const [versions, setVersions] = useState<TopologyVersionItem[]>([]);
  const [versionsLoading, setVersionsLoading] = useState<boolean>(false);
  const [rollingBack, setRollingBack] = useState<number | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    if (activeTab === 'export') {
      loadExport(exportFormat);
    } else if (activeTab === 'history') {
      loadVersions();
    }
  }, [isOpen, activeTab, exportFormat]);

  const loadExport = async (fmt: 'yaml' | 'json' | 'csv') => {
    setExportLoading(true);
    try {
      const data = await registryApi.exportTopology(fmt);
      setExportContent(data);
    } catch (err: any) {
      setExportContent(`Failed to generate export: ${err.message}`);
    } finally {
      setExportLoading(false);
    }
  };

  const loadVersions = async () => {
    setVersionsLoading(true);
    try {
      const list = await registryApi.listVersions();
      setVersions(list);
    } catch (err) {
      console.error('Failed to load topology versions:', err);
    } finally {
      setVersionsLoading(false);
    }
  };

  const handleCopyExport = () => {
    navigator.clipboard.writeText(exportContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownloadExport = () => {
    const blob = new Blob([exportContent], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `aegis-topology-${new Date().toISOString().slice(0, 10)}.${exportFormat}`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (evt) => {
      const text = evt.target?.result as string;
      if (text) {
        setImportText(text);
        setImportError(null);
        setDryRunDiff(null);
      }
    };
    reader.readAsText(file);
    e.target.value = '';
  };

  const handleLoadSampleMesh = () => {
    setImportText(`version: "1.0"
system_name: "shopkart-mesh"
description: "10-node microservice mesh with directed dependencies"
nodes:
  - id: "edge-gateway"
    name: "Edge API Gateway"
    type: "gateway"
    dependencies: ["checkout", "cart", "catalog", "search", "reviews"]
  - id: "checkout"
    name: "Checkout Orchestrator"
    type: "service"
    dependencies: ["cart", "catalog", "payments", "notifications", "inventory"]
  - id: "payments"
    name: "Payments Service"
    type: "service"
    dependencies: ["external-bank-mock"]
  - id: "reviews"
    name: "Customer Reviews"
    type: "service"
    dependencies: ["catalog"]
  - id: "search"
    name: "Catalog Search"
    type: "service"
    dependencies: ["catalog"]
  - id: "cart"
    name: "Shopping Cart"
    type: "service"
    dependencies: []
  - id: "catalog"
    name: "Product Catalog"
    type: "service"
    dependencies: []
  - id: "inventory"
    name: "Inventory Management"
    type: "service"
    dependencies: []
  - id: "notifications"
    name: "Notification Dispatcher"
    type: "service"
    dependencies: []
  - id: "external-bank-mock"
    name: "External Bank Mock"
    type: "service"
    dependencies: []
`);
    setImportError(null);
    setDryRunDiff(null);
  };

  const parseImportPayload = () => {
    const trimmed = importText.trim();
    if (!trimmed) throw new Error('Import content is empty');
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      return JSON.parse(trimmed);
    }
    return { format: 'yaml', data: trimmed };
  };

  const handleDryRunPreview = async () => {
    setDryRunLoading(true);
    setImportError(null);
    setDryRunDiff(null);
    try {
      const payload = parseImportPayload();
      const res = await registryApi.importTopology(payload, true);
      if (res.preview) {
        setDryRunDiff(res.preview);
      } else {
        setDryRunDiff({ additions: [], modifications: [], deletions: [] });
      }
    } catch (err: any) {
      setImportError(err.message || 'Invalid import format. Please provide valid JSON or YAML.');
    } finally {
      setDryRunLoading(false);
    }
  };

  const handleApplyImport = async () => {
    setImporting(true);
    setImportError(null);
    try {
      const payload = parseImportPayload();
      const res = await registryApi.importTopology(payload, false);
      setImportSuccessMsg(`Topology applied successfully! Created version v${res.version || 1}.`);
      setDryRunDiff(null);
      await onRefreshTopology();
      setTimeout(() => {
        onClose();
      }, 1500);
    } catch (err: any) {
      setImportError(err.message || 'Import failed to apply');
    } finally {
      setImporting(false);
    }
  };

  const handleRollback = async (versionNumber: number) => {
    if (!confirm(`Are you sure you want to rollback topology to Version v${versionNumber}?`)) return;
    setRollingBack(versionNumber);
    try {
      await registryApi.rollbackVersion(versionNumber);
      await onRefreshTopology();
      await loadVersions();
    } catch (err: any) {
      alert(`Rollback failed: ${err.message}`);
    } finally {
      setRollingBack(null);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-xs p-4 font-sans animate-in fade-in duration-200">
      <div className="bg-[#151517] border border-[#26262B] rounded-xl shadow-2xl max-w-3xl w-full flex flex-col max-h-[88vh] overflow-hidden text-gray-200">
        {/* Modal Header */}
        <div className="p-4 border-b border-[#26262B] flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <h2 className="text-base font-semibold text-white">Topology Management</h2>
            <div className="flex bg-[#0B0B0C] border border-[#26262B] rounded-md p-0.5 space-x-1 ml-4 text-xs">
              <button
                onClick={() => setActiveTab('import')}
                className={`px-3 py-1 rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors ${
                  activeTab === 'import' ? 'bg-[#840032] text-white' : 'text-gray-400 hover:text-white'
                }`}
              >
                <Upload className="w-3.5 h-3.5" />
                <span>Import & Dry-Run</span>
              </button>
              <button
                onClick={() => setActiveTab('export')}
                className={`px-3 py-1 rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors ${
                  activeTab === 'export' ? 'bg-[#840032] text-white' : 'text-gray-400 hover:text-white'
                }`}
              >
                <Download className="w-3.5 h-3.5" />
                <span>Export</span>
              </button>
              <button
                onClick={() => setActiveTab('history')}
                className={`px-3 py-1 rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors ${
                  activeTab === 'history' ? 'bg-[#840032] text-white' : 'text-gray-400 hover:text-white'
                }`}
              >
                <History className="w-3.5 h-3.5" />
                <span>Version History</span>
              </button>
            </div>
          </div>
          <button onClick={onClose} className="p-1 rounded text-gray-400 hover:text-white hover:bg-gray-800">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Modal Body */}
        <div className="flex-1 overflow-y-auto p-5 text-xs">
          {/* TAB 1: IMPORT */}
          {activeTab === 'import' && (
            <div className="space-y-4">
              <div className="bg-[#0B0B0C] border border-[#26262B] p-3 rounded-lg text-gray-400 text-xs">
                <span className="font-semibold text-white">Dry-Run Guaranteed:</span> Paste your topology definition below. Aegis will compute and preview all graph additions, changes, and removals before committing them to the live control plane.
              </div>

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-gray-300 font-semibold">Topology Payload (JSON / YAML / CSV):</label>
                  <div className="flex items-center space-x-2">
                    <input
                      type="file"
                      ref={fileInputRef}
                      onChange={handleFileUpload}
                      accept=".yaml,.yml,.json,.csv"
                      className="hidden"
                    />
                    <button
                      type="button"
                      onClick={() => fileInputRef.current?.click()}
                      className="px-2.5 py-1 text-[11px] rounded bg-[#26262B] hover:bg-[#34343B] text-gray-200 border border-[#3A3A42] flex items-center gap-1 transition-colors"
                    >
                      <Upload className="w-3 h-3 text-emerald-400" />
                      <span>Upload File (.yaml / .json)</span>
                    </button>
                    <button
                      type="button"
                      onClick={handleLoadSampleMesh}
                      className="px-2.5 py-1 text-[11px] rounded bg-[#1e1e24] hover:bg-[#282830] text-emerald-400 border border-emerald-900/60 transition-colors"
                    >
                      <span>Load 10-Node Mesh Template</span>
                    </button>
                  </div>
                </div>
                <textarea
                  rows={9}
                  value={importText}
                  onChange={(e) => setImportText(e.target.value)}
                  placeholder={`version: "1.0"\nnodes:\n  - id: "edge-gateway"\n    dependencies: ["checkout", "cart"]\n  - id: "checkout"\n    dependencies: ["payments"]\n  - id: "payments"\n    dependencies: ["external-bank-mock"]`}
                  className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-lg p-3 font-mono text-[11px] text-gray-200 focus:outline-none focus:border-[#840032]"
                />
              </div>

              {importError && (
                <div className="p-3 bg-rose-950/20 border border-rose-900/50 rounded-lg text-rose-300 flex items-center space-x-2">
                  <AlertTriangle className="w-4 h-4 shrink-0 text-rose-400" />
                  <span>{importError}</span>
                </div>
              )}

              {importSuccessMsg && (
                <div className="p-3 bg-emerald-950/20 border border-emerald-900/50 rounded-lg text-emerald-300 flex items-center space-x-2">
                  <Check className="w-4 h-4 shrink-0 text-emerald-400" />
                  <span>{importSuccessMsg}</span>
                </div>
              )}

              {/* Action Buttons */}
              <div className="flex items-center space-x-3">
                <button
                  type="button"
                  disabled={dryRunLoading || !importText.trim()}
                  onClick={handleDryRunPreview}
                  className="px-4 py-2 rounded-md bg-[#26262B] hover:bg-[#34343B] text-white font-semibold disabled:opacity-40 transition-colors"
                >
                  {dryRunLoading ? 'Analyzing diff...' : 'Run Dry-Run Preview'}
                </button>
                <button
                  type="button"
                  disabled={importing || !importText.trim()}
                  onClick={handleApplyImport}
                  className="px-4 py-2 rounded-md bg-emerald-600 hover:bg-emerald-500 text-white font-semibold disabled:opacity-40 transition-colors shadow flex items-center gap-1.5"
                >
                  <Check className="w-3.5 h-3.5" />
                  <span>{importing ? 'Applying to Live Graph...' : 'Apply Changes to Topology'}</span>
                </button>
              </div>

              {/* Dry-Run Diff Preview */}
              {dryRunDiff && (
                <div className="mt-4 p-4 rounded-lg bg-[#0B0B0C] border border-[#26262B] space-y-3">
                  <h3 className="font-semibold text-white flex items-center space-x-2">
                    <span>Dry-Run Diff Preview</span>
                    <span className="text-[10px] bg-purple-900/40 text-purple-300 border border-purple-700 px-2 py-0.5 rounded-full font-mono">
                      Validated
                    </span>
                  </h3>

                  <div className="grid grid-cols-3 gap-2">
                    <div className="p-2.5 rounded bg-emerald-950/15 border border-emerald-900/40">
                      <div className="flex items-center space-x-1 text-emerald-400 font-bold">
                        <Plus className="w-3.5 h-3.5" />
                        <span>Additions ({dryRunDiff.additions?.length || 0})</span>
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-gray-400 space-y-0.5 max-h-28 overflow-y-auto">
                        {dryRunDiff.additions?.map((item: any, i: number) => (
                          <div key={i} className="truncate text-emerald-300">
                            + {item.id || `${item.sourceServiceId} → ${item.targetServiceId}`}
                          </div>
                        ))}
                      </div>
                    </div>

                    <div className="p-2.5 rounded bg-sky-950/15 border border-sky-900/40">
                      <div className="flex items-center space-x-1 text-sky-400 font-bold">
                        <Edit2 className="w-3.5 h-3.5" />
                        <span>Modifications ({dryRunDiff.modifications?.length || 0})</span>
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-gray-400 space-y-0.5 max-h-28 overflow-y-auto">
                        {dryRunDiff.modifications?.map((item: any, i: number) => (
                          <div key={i} className="truncate text-sky-300">
                            ~ {item.id || item.name}
                          </div>
                        ))}
                      </div>
                    </div>

                    <div className="p-2.5 rounded bg-rose-950/15 border border-rose-900/40">
                      <div className="flex items-center space-x-1 text-rose-400 font-bold">
                        <Trash2 className="w-3.5 h-3.5" />
                        <span>Deletions ({dryRunDiff.deletions?.length || 0})</span>
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-gray-400 space-y-0.5 max-h-28 overflow-y-auto">
                        {dryRunDiff.deletions?.map((item: any, i: number) => (
                          <div key={i} className="truncate text-rose-300">
                            - {item.id || item}
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* TAB 2: EXPORT */}
          {activeTab === 'export' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2">
                  <span className="text-gray-400 font-semibold">Format:</span>
                  {(['yaml', 'json', 'csv'] as const).map((fmt) => (
                    <button
                      key={fmt}
                      onClick={() => setExportFormat(fmt)}
                      className={`px-3 py-1 rounded text-xs font-mono font-semibold uppercase transition-colors ${
                        exportFormat === fmt ? 'bg-[#840032] text-white' : 'bg-[#0B0B0C] border border-[#26262B] text-gray-400'
                      }`}
                    >
                      {fmt}
                    </button>
                  ))}
                </div>

                <div className="flex items-center space-x-2">
                  <button
                    onClick={handleCopyExport}
                    className="px-3 py-1.5 rounded bg-[#26262B] hover:bg-[#34343B] text-gray-200 flex items-center space-x-1 font-medium transition-colors"
                  >
                    {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    <span>{copied ? 'Copied!' : 'Copy'}</span>
                  </button>
                  <button
                    onClick={handleDownloadExport}
                    className="px-3 py-1.5 rounded bg-[#840032] hover:bg-[#a80f49] text-white flex items-center space-x-1 font-semibold transition-colors"
                  >
                    <Download className="w-3.5 h-3.5" />
                    <span>Download {exportFormat.toUpperCase()}</span>
                  </button>
                </div>
              </div>

              <div className="p-2.5 rounded bg-amber-950/15 border border-amber-900/30 text-[11px] text-amber-300/80">
                Security Note: All sensitive telemetry authentication tokens and passwords are redacted from export files.
              </div>

              <textarea
                rows={12}
                readOnly
                value={exportLoading ? 'Loading export...' : exportContent}
                className="w-full bg-[#0B0B0C] border border-[#26262B] rounded-lg p-3 font-mono text-[11px] text-gray-300 focus:outline-none select-all"
              />
            </div>
          )}

          {/* TAB 3: VERSION HISTORY */}
          {activeTab === 'history' && (
            <div className="space-y-3">
              <div className="text-gray-400 text-xs">
                Every service, dependency, or configuration edit creates an immutable topology snapshot. You can inspect or rollback to any previous version.
              </div>

              {versionsLoading ? (
                <div className="py-8 text-center text-gray-500">Loading version history...</div>
              ) : versions.length === 0 ? (
                <div className="py-8 text-center text-gray-500">No previous versions found.</div>
              ) : (
                <div className="divide-y divide-[#26262B] border border-[#26262B] rounded-lg overflow-hidden bg-[#0B0B0C]">
                  {versions.map((ver) => (
                    <div key={ver.id || ver.version} className="p-3.5 flex items-center justify-between hover:bg-[#151517] transition-colors">
                      <div>
                        <div className="flex items-center space-x-2">
                          <span className="font-mono text-sm font-bold text-white">v{ver.version}</span>
                          <span className="text-xs text-gray-300 font-medium">{ver.description || 'Topology update'}</span>
                        </div>
                        <div className="text-[11px] text-gray-400 mt-1 flex items-center space-x-3">
                          <span>Author: <span className="font-mono text-gray-300">{ver.createdBy || 'operator'}</span></span>
                          <span>•</span>
                          <span>{new Date(ver.createdAt).toLocaleString()}</span>
                        </div>
                      </div>

                      <button
                        onClick={() => handleRollback(ver.version)}
                        disabled={rollingBack === ver.version}
                        className="px-3 py-1.5 rounded bg-[#26262B] hover:bg-[#840032] text-gray-200 hover:text-white flex items-center space-x-1.5 text-xs font-semibold transition-colors disabled:opacity-40"
                      >
                        <RotateCcw className={`w-3.5 h-3.5 ${rollingBack === ver.version ? 'animate-spin' : ''}`} />
                        <span>Rollback</span>
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
