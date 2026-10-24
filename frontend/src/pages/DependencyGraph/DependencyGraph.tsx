import React, { useMemo, useState, useEffect, useCallback } from 'react';
import type {
  Connection,
  OnConnect,
} from '@xyflow/react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  MarkerType,
  useNodesState,
  useEdgesState,
  useReactFlow,
  ReactFlowProvider,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import {
  registryApi,
  type TopologyServiceItem,
  type TopologyEdgeItem,
  type ValidationResponse,
  type EdgeType,
} from '../../api/registry';
import { CustomGraphNode } from '../../components/TopologyGraph/CustomGraphNode';
import { CustomGraphEdge } from '../../components/TopologyGraph/CustomGraphEdge';
import { ValidationDrawer } from '../../components/GraphBuilder/ValidationDrawer';
import { ImportExportModal } from '../../components/GraphBuilder/ImportExportModal';
import { ServiceWizardModal } from '../../components/ServiceWizard/ServiceWizardModal';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { useAuthStore } from '../../store/useAuthStore';
import {
  Layout,
  Undo2,
  Redo2,
  CheckCircle2,
  AlertTriangle,
  Upload,
  Plus,
  RefreshCw,
  Sparkles,
  Focus,
} from 'lucide-react';

// Custom Node and Edge Types
const nodeTypes = {
  customNode: CustomGraphNode as any,
  topologyNode: CustomGraphNode as any,
};

const edgeTypes = {
  customEdge: CustomGraphEdge as any,
};

// Hierarchical / Topological Auto-Layout Algorithm
function computeAutoLayout(
  services: TopologyServiceItem[],
  edges: TopologyEdgeItem[]
): Record<string, { x: number; y: number }> {
  const positions: Record<string, { x: number; y: number }> = {};
  if (services.length === 0) return positions;

  const inDegree: Record<string, number> = {};
  const adj: Record<string, string[]> = {};

  services.forEach((s) => {
    inDegree[s.id] = 0;
    adj[s.id] = [];
  });

  edges.forEach((e) => {
    if (inDegree[e.targetServiceId] !== undefined) {
      inDegree[e.targetServiceId]++;
    }
    if (adj[e.sourceServiceId]) {
      adj[e.sourceServiceId].push(e.targetServiceId);
    }
  });

  // Assign levels using BFS
  const levels: Record<string, number> = {};
  const queue: { id: string; level: number }[] = [];

  services.forEach((s) => {
    if (inDegree[s.id] === 0 || s.type === 'gateway') {
      levels[s.id] = 0;
      queue.push({ id: s.id, level: 0 });
    }
  });

  if (queue.length === 0 && services.length > 0) {
    levels[services[0].id] = 0;
    queue.push({ id: services[0].id, level: 0 });
  }

  while (queue.length > 0) {
    const { id, level } = queue.shift()!;
    (adj[id] || []).forEach((nextId) => {
      const nextLevel = level + 1;
      if (levels[nextId] === undefined || levels[nextId] < nextLevel) {
        levels[nextId] = nextLevel;
        queue.push({ id: nextId, level: nextLevel });
      }
    });
  }

  // Handle remaining unvisited nodes
  const maxLevel = Math.max(0, ...Object.values(levels));
  services.forEach((s) => {
    if (levels[s.id] === undefined) {
      levels[s.id] = maxLevel + 1;
    }
  });

  // Group by level
  const levelGroups: Record<number, string[]> = {};
  Object.entries(levels).forEach(([id, lvl]) => {
    levelGroups[lvl] = levelGroups[lvl] || [];
    levelGroups[lvl].push(id);
  });

  const X_SPACING = 320;
  const Y_SPACING = 150;

  Object.entries(levelGroups).forEach(([lvlStr, group]) => {
    const lvl = Number(lvlStr);
    const totalHeight = group.length * Y_SPACING;
    const startY = 80 - totalHeight / 2;

    group.forEach((id, idx) => {
      positions[id] = {
        x: 80 + lvl * X_SPACING,
        y: Math.max(50, startY + idx * Y_SPACING + 100),
      };
    });
  });

  return positions;
}

const DependencyGraphContent: React.FC = () => {
  const { user } = useAuthStore();
  const isOperator = user?.role === 'operator' || user?.role === 'admin';

  const { tickTelemetry } = useTelemetryStore();
  const reactFlowInstance = useReactFlow();

  // State
  const [services, setServices] = useState<TopologyServiceItem[]>([]);
  const [rawEdges, setRawEdges] = useState<TopologyEdgeItem[]>([]);
  const [suggestions, setSuggestions] = useState<TopologyEdgeItem[]>([]);
  const [validation, setValidation] = useState<ValidationResponse | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  // Filters & Neighborhood Focus
  const [selectedNamespace, setSelectedNamespace] = useState<string>('ALL');
  const [neighborhoodFocus, setNeighborhoodFocus] = useState<boolean>(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  // Modals & Drawers
  const [validationOpen, setValidationOpen] = useState<boolean>(false);
  const [importExportOpen, setImportExportOpen] = useState<boolean>(false);
  const [wizardOpen, setWizardOpen] = useState<boolean>(false);
  const [newEdgePending, setNewEdgePending] = useState<Connection | null>(null);
  const [selectedEdgeType, setSelectedEdgeType] = useState<EdgeType>('sync');

  // React Flow Elements
  const [nodes, setNodes, onNodesChange] = useNodesState<any>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<any>([]);

  // Undo / Redo history
  const [history, setHistory] = useState<{ nodes: any[]; edges: any[] }[]>([]);
  const [historyIndex, setHistoryIndex] = useState<number>(-1);

  // Load all topology data
  const loadTopology = useCallback(async () => {
    setLoading(true);
    try {
      const [svcs, edgs, suggs, valRes] = await Promise.all([
        registryApi.listServices(),
        registryApi.listEdges(),
        registryApi.listSuggestions().catch(() => []),
        registryApi.validateTopology().catch(() => null),
      ]);

      setServices(svcs || []);
      setRawEdges(edgs || []);
      setSuggestions(suggs || []);
      setValidation(valRes);
    } catch (err) {
      console.error('Failed to load topology graph:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadTopology();
  }, [loadTopology]);

  // Compute 1-hop & 2-hop neighborhood for focus mode
  const neighborhoodSet = useMemo(() => {
    if (!neighborhoodFocus || !selectedNodeId) return null;
    const set = new Set<string>([selectedNodeId]);

    // 1-hop
    rawEdges.forEach((e) => {
      if (e.sourceServiceId === selectedNodeId) set.add(e.targetServiceId);
      if (e.targetServiceId === selectedNodeId) set.add(e.sourceServiceId);
    });

    // 2-hop
    rawEdges.forEach((e) => {
      if (set.has(e.sourceServiceId)) set.add(e.targetServiceId);
      if (set.has(e.targetServiceId)) set.add(e.sourceServiceId);
    });

    return set;
  }, [neighborhoodFocus, selectedNodeId, rawEdges]);

  // Accept / Reject Suggestions
  const handleAcceptSuggestion = async (edgeId: string) => {
    try {
      await registryApi.acceptSuggestion(edgeId);
      await loadTopology();
      await tickTelemetry();
    } catch (err: any) {
      alert(`Failed to accept suggestion: ${err.message}`);
    }
  };

  const handleRejectSuggestion = async (edgeId: string) => {
    try {
      await registryApi.rejectSuggestion(edgeId);
      await loadTopology();
    } catch (err: any) {
      alert(`Failed to reject suggestion: ${err.message}`);
    }
  };

  // Convert services & edges to React Flow nodes & edges
  useEffect(() => {
    if (services.length === 0) {
      setNodes([]);
      setEdges([]);
      return;
    }

    const filteredServices = selectedNamespace === 'ALL'
      ? services
      : services.filter((s) => (s.namespace || 'default') === selectedNamespace);

    const positions = computeAutoLayout(filteredServices, rawEdges);

    const newNodes: any[] = filteredServices.map((svc) => {
      const pos = positions[svc.id] || { x: 100, y: 100 };
      const isDimmed = neighborhoodSet !== null && !neighborhoodSet.has(svc.id);

      return {
        id: svc.id,
        type: 'customNode',
        position: pos,
        data: {
          label: svc.name,
          serviceId: svc.id,
          type: svc.type || 'service',
          monitoringMode: svc.monitoringMode || 'observe',
          status: svc.status || 'unconnected',
          tier: svc.tier || 'tier-2',
          namespace: svc.namespace || 'default',
          cpu: svc.cpu,
          latency: svc.latency,
          rps: svc.rps,
          hasRecoveryBinding: Boolean(svc.recoveryBinding),
          isUnconnected: !svc.telemetrySource || svc.status === 'unconnected',
          isUnmonitored: svc.monitoringMode === 'ignore',
          isDimmed,
        },
      };
    });

    // Edges
    const visibleServiceIds = new Set(filteredServices.map((s) => s.id));
    const activeEdges = rawEdges.filter((e) => visibleServiceIds.has(e.sourceServiceId) && visibleServiceIds.has(e.targetServiceId));

    const flowEdges: any[] = activeEdges.map((e, idx) => ({
      id: e.id || `edge-${e.sourceServiceId}-${e.targetServiceId}-${idx}`,
      source: e.sourceServiceId,
      target: e.targetServiceId,
      type: 'customEdge',
      data: {
        edgeId: e.id,
        sourceType: e.source || 'manual',
        state: e.state || 'active',
        edgeType: e.edgeType || 'sync',
        confidence: e.confidence,
        onAccept: handleAcceptSuggestion,
        onReject: handleRejectSuggestion,
      },
      markerEnd: { type: MarkerType.ArrowClosed, color: '#840032', width: 14, height: 14 },
    }));

    // Add suggestions
    suggestions.forEach((sug, idx) => {
      if (visibleServiceIds.has(sug.sourceServiceId) && visibleServiceIds.has(sug.targetServiceId)) {
        flowEdges.push({
          id: sug.id || `sugg-${sug.sourceServiceId}-${sug.targetServiceId}-${idx}`,
          source: sug.sourceServiceId,
          target: sug.targetServiceId,
          type: 'customEdge',
          data: {
            edgeId: sug.id,
            sourceType: 'suggested',
            state: 'suggested',
            edgeType: sug.edgeType || 'sync',
            confidence: sug.confidence || 0.85,
            onAccept: handleAcceptSuggestion,
            onReject: handleRejectSuggestion,
          },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#C084FC', width: 14, height: 14 },
        });
      }
    });

    setNodes(newNodes);
    setEdges(flowEdges);

    // Record initial history
    if (history.length === 0) {
      setHistory([{ nodes: newNodes, edges: flowEdges }]);
      setHistoryIndex(0);
    }
  }, [services, rawEdges, suggestions, selectedNamespace, neighborhoodSet]);

  // Connect Handler (creates an edge between nodes)
  const onConnect: OnConnect = useCallback((connection) => {
    if (!connection.source || !connection.target) return;
    if (connection.source === connection.target) {
      alert('Self-loops are not allowed in service dependency graphs.');
      return;
    }
    setNewEdgePending(connection);
  }, []);

  const confirmNewEdge = async () => {
    if (!newEdgePending?.source || !newEdgePending?.target) return;

    try {
      const edgePayload: TopologyEdgeItem = {
        sourceServiceId: newEdgePending.source,
        targetServiceId: newEdgePending.target,
        edgeType: selectedEdgeType,
        source: 'manual',
        state: 'active',
      };
      await registryApi.createEdge(edgePayload);
      setNewEdgePending(null);
      await loadTopology();
      await tickTelemetry();
    } catch (err: any) {
      alert(`Failed to create edge: ${err.message}`);
    }
  };

  // Auto Layout action
  const handleAutoLayout = () => {
    const positions = computeAutoLayout(services, rawEdges);
    setNodes((prevNodes) =>
      prevNodes.map((n) => ({
        ...n,
        position: positions[n.id] || n.position,
      }))
    );
    setTimeout(() => {
      reactFlowInstance.fitView({ padding: 0.2, duration: 400 });
    }, 50);
  };

  // Focus Node
  const handleFocusNode = (serviceId: string) => {
    const node = nodes.find((n) => n.id === serviceId);
    if (node) {
      reactFlowInstance.setCenter(node.position.x + 100, node.position.y + 50, { zoom: 1.2, duration: 500 });
      setSelectedNodeId(serviceId);
    }
  };

  // Undo & Redo
  const handleUndo = () => {
    if (historyIndex > 0) {
      const nextIdx = historyIndex - 1;
      setHistoryIndex(nextIdx);
      setNodes(history[nextIdx].nodes);
      setEdges(history[nextIdx].edges);
    }
  };

  const handleRedo = () => {
    if (historyIndex < history.length - 1) {
      const nextIdx = historyIndex + 1;
      setHistoryIndex(nextIdx);
      setNodes(history[nextIdx].nodes);
      setEdges(history[nextIdx].edges);
    }
  };

  // Delete Selected Node or Edge
  const handleDeleteSelected = async () => {
    const selectedNodes = nodes.filter((n) => n.selected);
    const selectedEdges = edges.filter((e) => e.selected);

    if (selectedNodes.length === 0 && selectedEdges.length === 0) return;

    if (!confirm('Are you sure you want to delete the selected items?')) return;

    for (const node of selectedNodes) {
      await registryApi.deleteService(node.id).catch(console.error);
    }
    for (const edge of selectedEdges) {
      await registryApi.deleteEdge(edge.source, edge.target).catch(console.error);
    }

    await loadTopology();
    await tickTelemetry();
  };

  // Keyboard shortcut listener for Delete/Backspace
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.key === 'Delete' || e.key === 'Backspace') && !['INPUT', 'TEXTAREA'].includes((e.target as any)?.tagName)) {
        handleDeleteSelected();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [nodes, edges]);

  // Unique namespaces for filtering
  const namespaces = useMemo(() => {
    const set = new Set<string>();
    services.forEach((s) => set.add(s.namespace || 'default'));
    return Array.from(set);
  }, [services]);

  return (
    <div className="flex h-[calc(100vh-4rem)] w-full overflow-hidden bg-[#0B0B0C] font-sans">
      {/* Left Palette & Workspace */}
      <div className="flex-1 flex flex-col relative h-full">
        {/* Top Workbench Toolbar */}
        <div className="h-12 bg-[#151517] border-b border-[#26262B] px-4 flex items-center justify-between z-10 select-none">
          {/* Left tools: Title, Add, Auto Layout, Undo/Redo */}
          <div className="flex items-center space-x-2">
            <span className="font-semibold text-white text-xs flex items-center space-x-1.5 mr-2">
              <span className="w-2 h-2 rounded-full bg-[#840032]" />
              <span>Dependency Graph Builder</span>
            </span>

            {isOperator && (
              <button
                onClick={() => setWizardOpen(true)}
                className="px-2.5 py-1 rounded bg-[#840032] hover:bg-[#a80f49] text-white flex items-center space-x-1 text-xs font-semibold shadow transition-colors"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Add Node</span>
              </button>
            )}

            <button
              onClick={handleAutoLayout}
              className="px-2.5 py-1 rounded bg-[#0B0B0C] border border-[#26262B] hover:bg-[#202024] text-gray-300 flex items-center space-x-1 text-xs transition-colors"
              title="Organize nodes into hierarchical DAG levels"
            >
              <Layout className="w-3.5 h-3.5 text-purple-400" />
              <span>Auto Layout</span>
            </button>

            <div className="flex items-center space-x-0.5 border-l border-[#26262B] pl-2 ml-1">
              <button
                disabled={historyIndex <= 0}
                onClick={handleUndo}
                className="p-1 rounded text-gray-400 hover:text-white disabled:opacity-30 transition-colors"
                title="Undo (Ctrl+Z)"
              >
                <Undo2 className="w-3.5 h-3.5" />
              </button>
              <button
                disabled={historyIndex >= history.length - 1}
                onClick={handleRedo}
                className="p-1 rounded text-gray-400 hover:text-white disabled:opacity-30 transition-colors"
                title="Redo (Ctrl+Y)"
              >
                <Redo2 className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Neighborhood Focus Toggle */}
            <button
              onClick={() => setNeighborhoodFocus(!neighborhoodFocus)}
              className={`px-2.5 py-1 rounded text-xs flex items-center space-x-1 border transition-colors ${
                neighborhoodFocus
                  ? 'bg-amber-500/20 border-amber-500 text-amber-300 font-semibold'
                  : 'bg-[#0B0B0C] border-[#26262B] text-gray-400 hover:text-gray-200'
              }`}
              title="Highlight 1 & 2 hop neighborhood of selected node, dimming others"
            >
              <Focus className="w-3.5 h-3.5" />
              <span>Focus Neighborhood</span>
            </button>
          </div>

          {/* Right tools: Namespace filter, Validation, Import/Export, Refresh */}
          <div className="flex items-center space-x-2">
            {/* Namespace Filter */}
            {namespaces.length > 1 && (
              <div className="flex items-center space-x-1 text-xs">
                <span className="text-gray-400 text-[11px]">NS:</span>
                <select
                  value={selectedNamespace}
                  onChange={(e) => setSelectedNamespace(e.target.value)}
                  className="bg-[#0B0B0C] border border-[#26262B] rounded px-2 py-0.5 text-xs text-gray-300 focus:outline-none"
                >
                  <option value="ALL">All Namespaces</option>
                  {namespaces.map((ns) => (
                    <option key={ns} value={ns}>{ns}</option>
                  ))}
                </select>
              </div>
            )}

            {/* Validation Drawer Button */}
            <button
              onClick={() => setValidationOpen(!validationOpen)}
              className={`px-2.5 py-1 rounded text-xs font-semibold flex items-center space-x-1.5 border transition-colors ${
                validation?.valid
                  ? 'bg-emerald-950/20 border-emerald-800 text-emerald-300'
                  : 'bg-rose-950/20 border-rose-800 text-rose-300'
              }`}
            >
              {validation?.valid ? (
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
              ) : (
                <AlertTriangle className="w-3.5 h-3.5 text-rose-400" />
              )}
              <span>
                {validation?.summary ? `${validation.summary.errorCount} Err, ${validation.summary.warningCount} Warn` : 'Validate'}
              </span>
            </button>

            {/* Import / Export */}
            <button
              onClick={() => setImportExportOpen(true)}
              className="px-2.5 py-1 rounded bg-[#0B0B0C] border border-[#26262B] hover:bg-[#202024] text-gray-300 flex items-center space-x-1 text-xs transition-colors"
            >
              <Upload className="w-3.5 h-3.5 text-gray-400" />
              <span>Import / Export</span>
            </button>

            {/* Refresh */}
            <button
              onClick={loadTopology}
              className="p-1 rounded bg-[#0B0B0C] border border-[#26262B] text-gray-400 hover:text-white transition-colors"
              title="Refresh topology from backend"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-[#e2588a]' : ''}`} />
            </button>
          </div>
        </div>

        {/* Pending Suggestions Notification Banner */}
        {suggestions.length > 0 && (
          <div className="bg-purple-950/40 border-b border-purple-800/60 px-4 py-2 flex items-center justify-between text-xs text-purple-200 z-10">
            <div className="flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-purple-400 shrink-0" />
              <span>
                <strong className="text-white">{suggestions.length} Suggested Dependencies</strong> discovered via trace telemetry.
                Review and accept them directly on the canvas or reject to permanently dismiss.
              </span>
            </div>
            <div className="text-[11px] text-purple-300/80 font-mono">
              Trace Auto-Discovery Active
            </div>
          </div>
        )}

        {/* Main React Flow Canvas */}
        <div className="flex-1 w-full h-full relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={(_, node) => setSelectedNodeId(node.id)}
            onPaneClick={() => setSelectedNodeId(null)}
            fitView
            minZoom={0.2}
            maxZoom={2.5}
            className="bg-[#0B0B0C]"
          >
            <Background color="#1C1C20" gap={24} size={1.5} />
            <Controls className="!bg-[#151517] !border-[#26262B] !text-gray-300" />
            <MiniMap
              nodeStrokeColor="#840032"
              nodeColor="#151517"
              maskColor="rgba(11, 11, 12, 0.75)"
              className="!bg-[#0B0B0C] !border-[#26262B] !rounded-lg"
            />
          </ReactFlow>
        </div>
      </div>

      {/* Validation Side Drawer */}
      <ValidationDrawer
        isOpen={validationOpen}
        onClose={() => setValidationOpen(false)}
        validation={validation}
        isLoading={loading}
        onRefresh={loadTopology}
        onFocusNode={handleFocusNode}
      />

      {/* Create Edge Modal with Edge Type Picker */}
      {newEdgePending && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4 animate-in fade-in">
          <div className="bg-[#151517] border border-[#26262B] rounded-xl p-5 max-w-sm w-full text-xs text-gray-200 space-y-3.5 shadow-2xl">
            <h3 className="font-semibold text-white text-sm">Create Service Dependency</h3>
            <div className="p-2.5 rounded bg-[#0B0B0C] border border-[#26262B] font-mono text-[11px]">
              <div className="text-gray-400">Source: <span className="text-white font-semibold">{newEdgePending.source}</span></div>
              <div className="text-gray-400 mt-1">Target: <span className="text-white font-semibold">{newEdgePending.target}</span></div>
            </div>

            <div>
              <label className="block text-gray-300 font-semibold mb-1">Relationship / Call Type:</label>
              <div className="grid grid-cols-2 gap-2">
                {[
                  { id: 'sync', label: 'Sync RPC/HTTP', desc: 'Direct synchronous call' },
                  { id: 'async', label: 'Async / Queue', desc: 'Event broker / message queue' },
                  { id: 'db', label: 'Database Access', desc: 'Query or transaction' },
                  { id: 'cache', label: 'Cache Lookup', desc: 'In-memory Redis/Memcached' },
                ].map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => setSelectedEdgeType(t.id as EdgeType)}
                    className={`p-2 rounded border text-left transition-colors ${
                      selectedEdgeType === t.id
                        ? 'bg-[#840032]/25 border-[#840032] text-white'
                        : 'bg-[#0B0B0C] border-[#26262B] text-gray-400'
                    }`}
                  >
                    <div className="font-semibold text-[11px] text-gray-200">{t.label}</div>
                    <div className="text-[9px] text-gray-400">{t.desc}</div>
                  </button>
                ))}
              </div>
            </div>

            <div className="flex items-center justify-end space-x-2 pt-2 border-t border-[#26262B]">
              <button
                type="button"
                onClick={() => setNewEdgePending(null)}
                className="px-3 py-1.5 rounded text-gray-400 hover:text-white"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={confirmNewEdge}
                className="px-4 py-1.5 rounded bg-[#840032] hover:bg-[#a80f49] text-white font-semibold shadow"
              >
                Create Edge
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Add / Edit Service Wizard */}
      <ServiceWizardModal
        isOpen={wizardOpen}
        onClose={() => setWizardOpen(false)}
        onSaved={loadTopology}
      />

      {/* Import / Export Modal */}
      <ImportExportModal
        isOpen={importExportOpen}
        onClose={() => setImportExportOpen(false)}
        onRefreshTopology={loadTopology}
      />
    </div>
  );
};

export const DependencyGraph: React.FC = () => {
  return (
    <ReactFlowProvider>
      <DependencyGraphContent />
    </ReactFlowProvider>
  );
};
