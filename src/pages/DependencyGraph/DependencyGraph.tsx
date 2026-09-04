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

  // Compute positions for 8 microservices topology graph layout
  const nodePositions: Record<string, { x: number; y: number }> = {
    gateway: { x: 350, y: 30 },
    auth: { x: 150, y: 160 },
    orders: { x: 550, y: 160 },
    payment: { x: 450, y: 310 },
    notifications: { x: 750, y: 310 },
    database: { x: 300, y: 460 },
    redis: { x: 600, y: 460 },
    kafka: { x: 800, y: 460 },
  };

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
        ? '#DC2626' // Red
        : isCascadeDegraded
        ? '#D97706' // Amber
        : '#0284C7'; // Sky Blue

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
    <div className="flex flex-col h-[calc(100vh-4rem)] font-mono bg-[#0B0F19]">
      {/* Topology Header Control Bar */}
      <div className="bg-[#111827] px-6 py-3 border-b border-[#1F2937] flex flex-wrap items-center justify-between gap-4 z-10 shrink-0">
        <div>
          <div className="flex items-center space-x-2">
            <GitFork className="w-5 h-5 text-sky-400" />
            <h1 className="font-bold text-gray-100 text-sm uppercase tracking-wide">
              Interactive Microservices Topology Graph (Hero Feature)
            </h1>
          </div>
          <p className="text-[11px] text-gray-400">
            Real-time particle edge traffic animation, health rings, and failure cascade propagation
          </p>
        </div>

        {/* Legend */}
        <div className="flex items-center space-x-4 text-xs">
          <div className="flex items-center space-x-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-emerald-500" />
            <span className="text-gray-300">Healthy</span>
          </div>

          <div className="flex items-center space-x-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-amber-500" />
            <span className="text-gray-300">Degraded</span>
          </div>

          <div className="flex items-center space-x-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-red-500 animate-pulse" />
            <span className="text-gray-300">Critical Failure</span>
          </div>

          <div className="flex items-center space-x-1.5 pl-3 border-l border-[#1F2937]">
            <span className="w-4 h-0.5 bg-red-500" />
            <span className="text-red-400 font-bold">Traffic Cascade Backup</span>
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
            <Background color="#1F2937" gap={20} size={1} />
            <Controls />
            <MiniMap
              style={{ background: '#111827', border: '1px solid #1F2937' }}
              nodeColor={(node) => {
                const s = services.find((srv) => srv.id === node.id);
                if (s?.status === 'critical') return '#DC2626';
                if (s?.status === 'degraded') return '#D97706';
                return '#16A34A';
              }}
            />
          </ReactFlow>
        </div>

        {/* Side Inspection Drawer when node clicked */}
        {activeSideNodeId && selectedService && (
          <div className="w-96 bg-[#111827] border-l border-[#1F2937] p-5 flex flex-col justify-between overflow-y-auto font-mono shrink-0 shadow-2xl z-20">
            <div>
              {/* Drawer Header */}
              <div className="flex items-center justify-between border-b border-[#1F2937] pb-3 mb-4">
                <div className="flex items-center space-x-2">
                  <span
                    className={`w-3 h-3 rounded-full ${
                      selectedService.status === 'critical'
                        ? 'bg-red-500'
                        : selectedService.status === 'degraded'
                        ? 'bg-amber-500'
                        : 'bg-emerald-500'
                    }`}
                  />
                  <div>
                    <h3 className="font-bold text-gray-100 text-sm">
                      {selectedService.name}
                    </h3>
                    <span className="text-[10px] text-gray-400 uppercase">
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
              <div className="space-y-3 mb-6">
                <div className="bg-[#0B0F19] p-3 rounded border border-[#1F2937] space-y-2 text-xs">
                  <div className="flex justify-between">
                    <span className="text-gray-400">Health Status:</span>
                    <span
                      className={`font-bold uppercase ${
                        selectedService.status === 'critical'
                          ? 'text-red-400'
                          : selectedService.status === 'degraded'
                          ? 'text-amber-400'
                          : 'text-emerald-400'
                      }`}
                    >
                      {selectedService.status}
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-gray-400">CPU Usage:</span>
                    <span className="text-gray-100 font-bold">
                      {selectedService.cpu}%
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-gray-400">Latency (p95):</span>
                    <span className="text-gray-100 font-bold">
                      {selectedService.latency}ms
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-gray-400">Memory Load:</span>
                    <span className="text-gray-100 font-bold">
                      {selectedService.memory}%
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-gray-400">RPS:</span>
                    <span className="text-gray-100 font-bold">
                      {selectedService.rps} req/s
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-gray-400">Replicas:</span>
                    <span className="text-gray-100 font-bold">
                      {selectedService.replicas}/{selectedService.maxReplicas}
                    </span>
                  </div>
                </div>

                {/* ML Root cause callout if this node is root cause */}
                {prediction.rootCauseServiceId === selectedService.id && (
                  <div className="p-3 bg-red-950/60 border border-red-800 rounded text-xs space-y-1">
                    <div className="flex items-center space-x-1.5 text-red-400 font-bold uppercase">
                      <BrainCircuit className="w-4 h-4" />
                      <span>ISOLATED ROOT CAUSE</span>
                    </div>
                    <p className="text-[11px] text-red-300">
                      {prediction.rootCauseReason}
                    </p>
                  </div>
                )}

                {/* Node Logs */}
                <div>
                  <h4 className="text-[11px] font-bold text-gray-400 uppercase mb-1">
                    Live Node Logs
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
              className="w-full py-2 bg-sky-900 hover:bg-sky-800 text-white rounded text-xs font-bold border border-sky-700 transition-colors flex items-center justify-center space-x-2"
            >
              <span>OPEN FULL METRIC DASHBOARD</span>
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
