import React, { useMemo, useState, useCallback } from 'react';
import type { Node, Edge } from '@xyflow/react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { useTelemetryStore, getMLPrediction } from '../../store/useTelemetryStore';
import { registryApi, type TopologyInfo } from '../../api/registry';
import { apiFetch } from '../../api/client';
import { TopologyNode } from '../TopologyNode';
import { ImportExportModal } from '../GraphBuilder/ImportExportModal';
import { GitFork, Layers, ShieldAlert, Upload } from 'lucide-react';

interface TopologyGraphProps {
  height?: string;
  showMiniMap?: boolean;
  showControls?: boolean;
  showTopologySelector?: boolean;
  onNodeSelect?: (serviceId: string) => void;
  className?: string;
}

export const TopologyGraph: React.FC<TopologyGraphProps> = ({
  height = '500px',
  showMiniMap = true,
  showControls = true,
  showTopologySelector = true,
  onNodeSelect,
  className = '',
}) => {
  const { services, activeFaults, setSelectedServiceId, tickTelemetry } = useTelemetryStore();
  const [topologies, setTopologies] = useState<TopologyInfo[]>([]);
  const [selectedTopo, setSelectedTopo] = useState<string>('');
  const [switchingTopo, setSwitchingTopo] = useState<boolean>(false);
  const [importModalOpen, setImportModalOpen] = useState<boolean>(false);

  const prediction = getMLPrediction(services, activeFaults);
  const rootCauseId = prediction.rootCauseServiceId;
  const blastRadiusSet = useMemo(() => new Set(prediction.blastRadius || []), [prediction.blastRadius]);

  React.useEffect(() => {
    registryApi.listTopologies()
      .then((items) => {
        setTopologies(items);
        const active = items.find((item) => item.active);
        if (active) setSelectedTopo(active.filename);
      })
      .catch((err) => console.error('Failed to load available topologies:', err));
  }, []);

  const handleSelectTopology = async (filename: string) => {
    setSelectedTopo(filename);
    setSwitchingTopo(true);
    try {
      await apiFetch('/topology/select', {
        method: 'POST',
        body: JSON.stringify({ filename }),
      });
      await tickTelemetry();
    } catch (err) {
      console.error('Failed to switch topology:', err);
    } finally {
      setSwitchingTopo(false);
    }
  };

  const nodeTypes = useMemo(() => ({ topologyNode: TopologyNode }), []);

  // Compute hierarchical layout based on DAG levels
  const nodePositions = useMemo(() => {
    const positions: Record<string, { x: number; y: number }> = {};
    if (services.length === 0) return positions;

    // If no edges exist between any services, lay them out in a clean multi-row grid
    const totalEdges = services.reduce((acc, s) => acc + (s.dependencies ? s.dependencies.length : 0), 0);
    if (totalEdges === 0) {
      const COLS = Math.min(4, Math.max(2, Math.ceil(Math.sqrt(services.length))));
      const X_SPACING = 240;
      const Y_SPACING = 150;
      services.forEach((s, idx) => {
        const row = Math.floor(idx / COLS);
        const col = idx % COLS;
        positions[s.id] = {
          x: 100 + col * X_SPACING,
          y: 40 + row * Y_SPACING,
        };
      });
      return positions;
    }

    const inDegree: Record<string, number> = {};
    services.forEach((s) => {
      inDegree[s.id] = inDegree[s.id] || 0;
      (s.dependencies || []).forEach((dep) => {
        inDegree[dep] = (inDegree[dep] || 0) + 1;
      });
    });

    const levels: Record<string, number> = {};
    const queue: { id: string; level: number }[] = [];

    services.forEach((s) => {
      if (s.type === 'gateway' || inDegree[s.id] === 0) {
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
      const svc = services.find((s) => s.id === id);
      if (svc && svc.dependencies) {
        svc.dependencies.forEach((depId) => {
          const nextLevel = level + 1;
          if (levels[depId] === undefined || levels[depId] < nextLevel) {
            levels[depId] = nextLevel;
            queue.push({ id: depId, level: nextLevel });
          }
        });
      }
    }

    const maxLevel = Math.max(0, ...Object.values(levels));
    services.forEach((s) => {
      if (levels[s.id] === undefined) {
        levels[s.id] = maxLevel + 1;
      }
    });

    const levelBuckets: Record<number, string[]> = {};
    Object.entries(levels).forEach(([id, lvl]) => {
      levelBuckets[lvl] = levelBuckets[lvl] || [];
      levelBuckets[lvl].push(id);
    });

    const Y_SPACING = 150;
    const X_SPACING = 240;

    Object.entries(levelBuckets).forEach(([lvlStr, nodeIds]) => {
      const lvl = parseInt(lvlStr, 10);
      const totalWidth = (nodeIds.length - 1) * X_SPACING;
      const startX = 400 - totalWidth / 2;

      nodeIds.forEach((id, index) => {
        positions[id] = {
          x: startX + index * X_SPACING,
          y: 40 + lvl * Y_SPACING,
        };
      });
    });

    return positions;
  }, [services]);

  // Construct ReactFlow Nodes
  const nodes: Node[] = useMemo(() => {
    return services.map((svc) => {
      const isRootCause = rootCauseId === svc.id;
      const isInBlast = blastRadiusSet.has(svc.id);
      const pos = nodePositions[svc.id] || { x: 100, y: 100 };

      return {
        id: svc.id,
        type: 'topologyNode',
        position: pos,
        data: {
          label: svc.name,
          serviceId: svc.id,
          type: svc.type,
          status: svc.status,
          dataStatus: svc.dataStatus,
          cpu: svc.cpu,
          latency: svc.latency,
          rps: svc.rps,
          replicas: svc.replicas,
          isRootCause,
          isInBlastRadius: isInBlast,
        },
      };
    });
  }, [services, nodePositions, rootCauseId, blastRadiusSet]);

  // Construct ReactFlow Edges with animated state pulses
  const edges: Edge[] = useMemo(() => {
    const edgeList: Edge[] = [];
    services.forEach((src) => {
      (src.dependencies || []).forEach((targetId) => {
        const targetSvc = services.find((s) => s.id === targetId);
        const isDegradedOrCritical =
          src.status === 'critical' ||
          src.status === 'degraded' ||
          targetSvc?.status === 'critical' ||
          targetSvc?.status === 'degraded';

        const isBlastEdge = blastRadiusSet.has(src.id) || blastRadiusSet.has(targetId);
        const isRootEdge = rootCauseId === src.id || rootCauseId === targetId;

        const strokeColor = isRootEdge
          ? '#AD2831'
          : isBlastEdge
          ? '#9333EA'
          : isDegradedOrCritical
          ? '#D4A017'
          : '#3B4252';

        edgeList.push({
          id: `edge-${src.id}-${targetId}`,
          source: src.id,
          target: targetId,
          animated: isDegradedOrCritical || isBlastEdge,
          style: {
            stroke: strokeColor,
            strokeWidth: isRootEdge || isBlastEdge ? 2.5 : 1.5,
            strokeDasharray: isBlastEdge ? '5,5' : undefined,
          },
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: strokeColor,
            width: 14,
            height: 14,
          },
        });
      });
    });
    return edgeList;
  }, [services, blastRadiusSet, rootCauseId]);

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      setSelectedServiceId(node.id);
      if (onNodeSelect) {
        onNodeSelect(node.id);
      }
    },
    [setSelectedServiceId, onNodeSelect]
  );

  return (
    <div className={`relative bg-[#0F0F11] border border-[#26262B] rounded-xl overflow-hidden flex flex-col ${className}`} style={{ height }}>
      {/* Top Toolbar */}
      <div className="flex flex-wrap items-center justify-between px-4 py-2.5 bg-[#151517] border-b border-[#26262B] gap-3 z-10">
        <div className="flex items-center space-x-3">
          <div className="p-1.5 bg-[#840032]/20 border border-[#840032] rounded-md text-[#e2588a]">
            <GitFork className="w-4 h-4" />
          </div>
          <div>
            <span className="text-xs font-semibold text-white tracking-wide">
              LIVE TOPOLOGY GRAPH
            </span>
            <span className="text-[11px] text-gray-400 ml-2 font-mono">
              {services.length} nodes · {edges.length} directed edges
            </span>
          </div>
        </div>

        {/* Legend & Controls */}
        <div className="flex items-center space-x-4 text-[10px] font-sans font-medium text-gray-400">
          <div className="hidden sm:flex items-center space-x-3">
            <span className="flex items-center space-x-1">
              <span className="w-2 h-2 rounded-full bg-[#3F8E4F]" />
              <span>Healthy</span>
            </span>
            <span className="flex items-center space-x-1">
              <span className="w-2 h-2 rounded-full bg-[#D4A017]" />
              <span>Warning</span>
            </span>
            <span className="flex items-center space-x-1">
              <span className="w-2 h-2 rounded-full bg-[#AD2831]" />
              <span>Critical</span>
            </span>
            {rootCauseId && (
              <span className="flex items-center space-x-1 text-[#f87171] font-semibold">
                <span className="w-2 h-2 rounded-full bg-[#AD2831] animate-ping" />
                <span>Root Cause ({rootCauseId})</span>
              </span>
            )}
            {blastRadiusSet.size > 0 && (
              <span className="flex items-center space-x-1 text-purple-400">
                <span className="w-2 h-2 rounded-full bg-purple-500" />
                <span>Blast Radius ({blastRadiusSet.size})</span>
              </span>
            )}
          </div>

          {showTopologySelector && (
            <div className="flex items-center space-x-2 pl-3 border-l border-[#26262B]">
              <div className="flex items-center space-x-1.5">
                <Layers className="w-3.5 h-3.5 text-gray-400" />
                <select
                  value={selectedTopo}
                  onChange={(e) => handleSelectTopology(e.target.value)}
                  disabled={switchingTopo}
                  className="bg-[#0B0B0C] text-gray-200 border border-[#26262B] rounded px-2 py-0.5 text-[11px] focus:outline-none focus:border-[#840032]"
                >
                  {topologies.length === 0 && (
                    <option value="">No preset topologies</option>
                  )}
                  {topologies.map((topology) => (
                    <option key={topology.filename} value={topology.filename}>
                      {topology.name || topology.filename} ({topology.nodeCount} nodes)
                    </option>
                  ))}
                </select>
              </div>

              <button
                type="button"
                onClick={() => setImportModalOpen(true)}
                className="flex items-center space-x-1 px-2.5 py-0.5 rounded text-[11px] bg-[#1e1e24] hover:bg-[#282830] text-emerald-400 border border-emerald-900/50 transition-colors cursor-pointer font-medium"
                title="Insert or upload YAML/JSON to define service dependencies"
              >
                <Upload className="w-3 h-3" />
                <span>Import YAML/JSON</span>
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Main Graph Canvas */}
      <div className="flex-1 w-full h-full relative">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodeClick={onNodeClick}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          attributionPosition="bottom-left"
          className="bg-[#0B0B0C]"
        >
          <Background color="#1F1F24" gap={20} size={1} />
          {showControls && <Controls className="!bg-[#151517] !border-[#26262B] !rounded-lg fill-gray-300" />}
          {showMiniMap && (
            <MiniMap
              className="!bg-[#151517] !border !border-[#26262B] !rounded-lg"
              nodeColor={(n) => {
                const s = (n.data as any)?.status;
                const dataStatus = (n.data as any)?.dataStatus;
                if (s === 'offline' || s === 'failed' || s === 'unknown' || s === 'stale' || dataStatus === 'stale' || dataStatus === 'unknown') return '#6B7280';
                if (s === 'critical') return '#AD2831';
                if (s === 'degraded') return '#D4A017';
                return '#3F8E4F';
              }}
            />
          )}
        </ReactFlow>

        {/* Informative notice when services exist but no directed edges have been discovered yet */}
        {services.length > 0 && edges.length === 0 && (
          <div className="absolute top-3 left-1/2 -translate-x-1/2 z-10 bg-[#151517]/95 border border-[#26262B] text-gray-300 text-xs px-3.5 py-2 rounded-md shadow-lg flex items-center gap-3 backdrop-blur-md">
            <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" />
            <span>0 directed edges recorded.</span>
            <button
              type="button"
              onClick={() => setImportModalOpen(true)}
              className="text-xs font-semibold text-emerald-400 hover:text-emerald-300 underline flex items-center gap-1 cursor-pointer"
            >
              <Upload className="w-3 h-3" />
              <span>Insert YAML / JSON Topology</span>
            </button>
          </div>
        )}

        {/* Floating Root Cause Badge Banner if active fault */}
        {rootCauseId && (
          <div className="absolute bottom-4 left-4 z-10 bg-[#AD2831]/90 backdrop-blur-md text-white px-3 py-1.5 rounded-lg border border-red-500/50 shadow-lg flex items-center space-x-2 text-xs">
            <ShieldAlert className="w-4 h-4 text-white animate-pulse" />
            <div>
              <span className="font-semibold">ANOMALY PROPAGATING:</span> Root cause isolated to{' '}
              <span className="font-mono font-bold underline">{rootCauseId}</span>
            </div>
          </div>
        )}
      </div>

      {/* Direct Import Modal for YAML/JSON Topology */}
      <ImportExportModal
        isOpen={importModalOpen}
        onClose={() => setImportModalOpen(false)}
        onRefreshTopology={async () => {
          await tickTelemetry();
        }}
      />
    </div>
  );
};
