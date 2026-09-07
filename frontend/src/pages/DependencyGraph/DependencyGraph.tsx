import { useMemo, useState } from 'react';
import type {
  Node,
  Edge,
} from '@xyflow/react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { useTelemetryStore, getMLPrediction } from '../../store/useTelemetryStore';
import { TopologyNode } from '../../components/TopologyNode';
import { ServiceDetailModal } from '../Services/ServiceDetailModal';
import { LogStreamer } from '../../components/LogStreamer';
import {
  GitFork,
  BrainCircuit,
  ArrowRight,
  X,
} from 'lucide-react';

export const DependencyGraph: React.FC = () => {
  const { services, activeFaults, logs, setSelectedServiceId, selectedServiceId } = useTelemetryStore();
  const [activeSideNodeId, setActiveSideNodeId] = useState<string | null>(null);

  const prediction = getMLPrediction(services, activeFaults);

  // Map custom node types
  const nodeTypes = useMemo(() => ({ topologyNode: TopologyNode }), []);

  // Dynamically compute node positions based on topological dependency levels
  const nodePositions = useMemo(() => {
    const positions: Record<string, { x: number; y: number }> = {};
    if (services.length === 0) return positions;

    // 1. Determine in-degree for root detection
    const inDegree: Record<string, number> = {};
    services.forEach((s) => {
      inDegree[s.id] = inDegree[s.id] || 0;
      s.dependencies.forEach((dep) => {
        inDegree[dep] = (inDegree[dep] || 0) + 1;
      });
    });

    // 2. Identify root nodes (nodes with in-degree 0 or gateway type)
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

    // 3. BFS to compute level depth
    while (queue.length > 0) {
      const { id, level } = queue.shift()!;
      const svc = services.find((s) => s.id === id);
      if (svc) {
        svc.dependencies.forEach((depId) => {
          const nextLevel = level + 1;
          if (levels[depId] === undefined || levels[depId] < nextLevel) {
            levels[depId] = nextLevel;
            queue.push({ id: depId, level: nextLevel });
          }
        });
      }
    }

    // Assign unvisited nodes to trailing level
    const maxLevel = Math.max(0, ...Object.values(levels));
    services.forEach((s) => {
      if (levels[s.id] === undefined) {
        levels[s.id] = maxLevel + 1;
      }
    });

    // Group nodes by level
    const levelGroups: Record<number, string[]> = {};
    Object.entries(levels).forEach(([nodeId, lvl]) => {
      levelGroups[lvl] = levelGroups[lvl] || [];
      levelGroups[lvl].push(nodeId);
    });

    // Compute (x, y) coordinates
    const startY = 40;
    const levelHeight = 150;
    const nodeWidth = 280;
    const centerX = 450;

    Object.entries(levelGroups).forEach(([lvlStr, nodeIds]) => {
      const lvl = parseInt(lvlStr, 10);
      const count = nodeIds.length;
      const totalWidth = (count - 1) * nodeWidth;
      const startX = centerX - totalWidth / 2;

      nodeIds.forEach((id, idx) => {
        positions[id] = {
          x: Math.round(startX + idx * nodeWidth),
          y: startY + lvl * levelHeight,
        };
      });
    });

    return positions;
  }, [services]);

  // Build React Flow nodes array dynamically from Zustand state
  const nodes: Node[] = services.map((service) => {
    const pos = nodePositions[service.id] || { x: 100, y: 100 };
    const isRoot = prediction.rootCauseServiceId === service.id;

    return {
      id: service.id,
      type: 'topologyNode',
      position: pos,
      data: {
        label: service.name,
        serviceId: service.id,
        type: service.type,
        status: service.status,
        cpu: service.cpu,
        latency: service.latency,
        rps: service.rps,
        replicas: service.replicas,
        isRootCause: isRoot,
      },
    };
  });

  // Build React Flow edges array dynamically based on dependencies & health status
  const edges: Edge[] = [];
  services.forEach((service) => {
    service.dependencies.forEach((depId) => {
      const targetService = services.find((s) => s.id === depId);
      const isCascadeCritical =
        service.status === 'critical' || targetService?.status === 'critical';
      const isCascadeDegraded =
        service.status === 'degraded' || targetService?.status === 'degraded';

      const strokeColor = isCascadeCritical
        ? '#AD2831' // Critical
        : isCascadeDegraded
        ? '#D4A017' // Warning
        : '#3F8E4F'; // Healthy

      edges.push({
        id: `e-${service.id}-${depId}`,
        source: service.id,
        target: depId,
        animated: true,
        style: {
          stroke: strokeColor,
          strokeWidth: isCascadeCritical ? 3.5 : isCascadeDegraded ? 2.5 : 1.5,
        },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: strokeColor,
        },
      });
    });
  });

  const selectedService = services.find((s) => s.id === activeSideNodeId);

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] font-sans bg-[#0B0B0C]">
      {/* Topology Header Control Bar */}
      <div className="bg-[#151517] px-6 py-2.5 border-b border-[#26262B] flex flex-wrap items-center justify-between gap-4 z-10 shrink-0">
        <div>
          <div className="flex items-center space-x-2">
            <GitFork className="w-4 h-4 text-[#e2588a]" />
            <h1 className="font-bold text-white text-[18px] tracking-[-0.02em] leading-[24px]">
              Microservices Topology Graph
            </h1>
          </div>
          <p className="text-[11px] text-gray-400 font-normal leading-[16px]">
            Live TGNN dependency graph, dynamic cascade edge propagation, and health status
          </p>
        </div>

        {/* Legend */}
        <div className="flex items-center space-x-4 text-[11px]">
          <div className="flex items-center space-x-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-[#3F8E4F]" />
            <span className="text-gray-300 font-medium">HEALTHY</span>
          </div>

          <div className="flex items-center space-x-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-[#D4A017]" />
            <span className="text-gray-300 font-medium">WARNING</span>
          </div>

          <div className="flex items-center space-x-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-[#AD2831]" />
            <span className="text-gray-300 font-medium">CRITICAL</span>
          </div>

          <div className="flex items-center space-x-1.5 pl-3 border-l border-[#26262B]">
            <span className="w-4 h-0.5 bg-[#AD2831]" />
            <span className="text-[#AD2831] font-semibold text-[10px] tracking-[0.04em] uppercase">CASCADE PATH</span>
          </div>
        </div>
      </div>

      {/* Main Flow Canvas & Side Panel Container */}
      <div className="flex-1 relative flex overflow-hidden">
        {/* React Flow Canvas */}
        <div className="flex-1 h-full relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={(_, node) => setActiveSideNodeId(node.id)}
            fitView
            attributionPosition="bottom-right"
          >
            <Background color="#1F1F24" gap={20} size={1} />
            <Controls />
            <MiniMap
              style={{ background: '#151517', border: '1px solid #26262B' }}
              nodeColor={(node) => {
                const s = services.find((srv) => srv.id === node.id);
                if (s?.status === 'critical') return '#AD2831';
                if (s?.status === 'degraded') return '#D4A017';
                return '#3F8E4F';
              }}
            />
          </ReactFlow>
        </div>

        {/* Side Inspection Drawer when node clicked */}
        {activeSideNodeId && selectedService && (
          <div className="w-96 bg-[#151517] border-l border-[#26262B] p-4 flex flex-col justify-between overflow-y-auto font-sans shrink-0 shadow-2xl z-20">
            <div>
              {/* Drawer Header */}
              <div className="flex items-center justify-between border-b border-[#26262B] pb-3 mb-4">
                <div className="flex items-center space-x-2">
                  <span
                    className={`w-2 h-2 rounded-full ${
                      selectedService.status === 'critical'
                        ? 'bg-[#AD2831]'
                        : selectedService.status === 'degraded'
                        ? 'bg-[#D4A017]'
                        : 'bg-[#3F8E4F]'
                    }`}
                  />
                  <div>
                    <h3 className="font-mono text-[16px] font-semibold text-white leading-[24px]">
                      {selectedService.name}
                    </h3>
                    <span className="font-mono text-[11px] text-gray-400">
                      ID: {selectedService.id}
                    </span>
                  </div>
                </div>

                <button
                  onClick={() => setActiveSideNodeId(null)}
                  className="text-gray-400 hover:text-white"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>

              {/* Node Live Gauges */}
              <div className="space-y-3 mb-4">
                <div className="bg-[#0B0B0C] p-3 rounded-lg border border-[#26262B] space-y-2 text-xs">
                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">HEALTH STATUS</span>
                    <span
                      className={`text-[10px] font-semibold tracking-[0.04em] uppercase px-1.5 py-0.5 rounded border ${
                        selectedService.status === 'critical'
                          ? 'bg-[#AD2831]/20 text-[#f87171] border-[#AD2831]/60'
                          : selectedService.status === 'degraded'
                          ? 'bg-[#D4A017]/15 text-[#e5b533] border-[#D4A017]/60'
                          : 'bg-[#3F8E4F]/15 text-[#52b767] border-[#3F8E4F]/60'
                      }`}
                    >
                      {selectedService.status === 'degraded' ? 'WARNING' : selectedService.status.toUpperCase()}
                    </span>
                  </div>

                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">CPU USAGE</span>
                    <span className="font-mono text-xs font-semibold text-gray-100">
                      {selectedService.cpu}%
                    </span>
                  </div>

                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">LATENCY (P95)</span>
                    <span className="font-mono text-xs font-semibold text-gray-100">
                      {selectedService.latency}ms
                    </span>
                  </div>

                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">MEMORY LOAD</span>
                    <span className="font-mono text-xs font-semibold text-gray-100">
                      {selectedService.memory}%
                    </span>
                  </div>

                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">THROUGHPUT</span>
                    <span className="font-mono text-xs font-semibold text-gray-100">
                      {selectedService.rps} req/s
                    </span>
                  </div>

                  <div className="flex justify-between items-center">
                    <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400">REPLICAS</span>
                    <span className="font-mono text-xs font-semibold text-gray-100">
                      {selectedService.replicas}/{selectedService.maxReplicas}
                    </span>
                  </div>
                </div>

                {/* ML Root cause callout if this node is root cause */}
                {prediction.rootCauseServiceId === selectedService.id && (
                  <div className="p-3 bg-[#840032]/25 border border-[#840032] rounded-lg text-xs space-y-1">
                    <div className="flex items-center space-x-1.5 text-[#f07b9e] text-[11px] font-semibold uppercase tracking-[0.07em]">
                      <BrainCircuit className="w-3.5 h-3.5" />
                      <span>ISOLATED ROOT CAUSE</span>
                    </div>
                    <p className="text-[11px] text-gray-300 font-normal leading-[16px]">
                      {prediction.rootCauseReason}
                    </p>
                  </div>
                )}

                {/* Node Logs */}
                <div>
                  <h4 className="text-[11px] font-semibold text-gray-400 uppercase tracking-[0.07em] mb-1.5">
                    LIVE NODE LOGS
                  </h4>
                  <LogStreamer
                    logs={logs}
                    serviceFilter={selectedService.id}
                    maxHeight="max-h-40"
                  />
                </div>
              </div>
            </div>

            {/* View Full Detail Button */}
            <button
              onClick={() => {
                setSelectedServiceId(selectedService.id);
                setActiveSideNodeId(null);
              }}
              className="w-full py-2 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors flex items-center justify-center space-x-2 shadow-sm"
            >
              <span>OPEN METRIC INSPECTOR</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>
        )}
      </div>

      {selectedServiceId && (
        <ServiceDetailModal
          serviceId={selectedServiceId}
          onClose={() => setSelectedServiceId(null)}
        />
      )}
    </div>
  );
};
