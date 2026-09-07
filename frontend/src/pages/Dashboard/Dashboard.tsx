import React from 'react';
import { useTelemetryStore, calculateSystemRisk, getMLPrediction } from '../../store/useTelemetryStore';
import { StatusCard } from '../../components/StatusCard';
import { MetricChart } from '../../components/MetricChart';
import { LogStreamer } from '../../components/LogStreamer';
import { Activity, ShieldAlert, Server, Clock, GitFork, ArrowRight, BrainCircuit } from 'lucide-react';

interface DashboardProps {
  setActiveTab: (tab: string) => void;
}

export const Dashboard: React.FC<DashboardProps> = ({ setActiveTab }) => {
  const { services, activeFaults, alerts, logs } = useTelemetryStore();

  const risk = calculateSystemRisk(services, activeFaults);
  const prediction = getMLPrediction(services, activeFaults);

  const isCritical = risk >= 80;
  const isDegraded = risk >= 40 && risk < 80;

  const runningServicesCount = services.filter((s) => s.status !== 'critical').length;
  const activeAlertsCount = alerts.filter((a) => !a.resolved).length;

  // Aggregate global metric trends from services history
  const historyLen = services[0]?.history.length || 0;
  const globalHistory = Array.from({ length: historyLen }).map((_, idx) => {
    const timestamp = services[0]?.history[idx]?.timestamp || '';
    const avgCpu = Math.round(services.reduce((acc, s) => acc + (s.history[idx]?.cpu || s.cpu), 0) / services.length);
    const avgLat = Math.round(services.reduce((acc, s) => acc + (s.history[idx]?.latency || s.latency), 0) / services.length);
    const avgMem = Math.round(services.reduce((acc, s) => acc + (s.history[idx]?.memory || s.memory), 0) / services.length);
    const totalRps = services.reduce((acc, s) => acc + (s.history[idx]?.rps || s.rps), 0);

    return {
      timestamp,
      cpu: avgCpu,
      latency: avgLat,
      memory: avgMem,
      rps: totalRps,
    };
  });

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Top Header Banner */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#26262B] pb-4">
        <div>
          <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
            Mission Control Overview
          </h1>
          <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
            Real-time topology telemetry, TGNN anomaly prediction, and autonomous remediation
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => setActiveTab('graph')}
            className="flex items-center space-x-2 px-3.5 py-1.5 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-all shadow-sm"
          >
            <GitFork className="w-3.5 h-3.5 text-white" />
            <span>DEPENDENCY GRAPH</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Mission Control Status Metric Tiles */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <StatusCard
          title="SYSTEM HEALTH"
          value={isCritical ? 'CRITICAL' : isDegraded ? 'WARNING' : 'HEALTHY'}
          icon={Activity}
          statusColor={isCritical ? 'critical' : isDegraded ? 'warning' : 'healthy'}
          badgeText={isCritical ? 'CRITICAL' : isDegraded ? 'WARNING' : 'HEALTHY'}
          trend="8 Active Nodes"
        />

        <StatusCard
          title="FAILURE RISK"
          value={`${risk}%`}
          icon={ShieldAlert}
          statusColor={risk > 70 ? 'critical' : risk > 30 ? 'warning' : 'healthy'}
          badgeText={risk > 70 ? 'CRITICAL RISK' : risk > 30 ? 'ATTN' : 'STABLE'}
          trend={activeFaults.length > 0 ? `${activeFaults.length} Faults Active` : 'Nominal'}
        />

        <StatusCard
          title="PREDICTION HORIZON"
          value={`${prediction.estimatedFailureSec}s`}
          icon={Clock}
          statusColor="intervention"
          badgeText="TGNN"
          trend={`Confidence: ${prediction.confidence}%`}
        />

        <StatusCard
          title="RUNNING SERVICES"
          value={`${runningServicesCount}/${services.length}`}
          icon={Server}
          statusColor="healthy"
          badgeText="K8S"
          trend="aegis-mesh-01"
        />

        <StatusCard
          title="ACTIVE ALERTS"
          value={activeAlertsCount}
          icon={ShieldAlert}
          statusColor={activeAlertsCount > 0 ? 'warning' : 'healthy'}
          badgeText={activeAlertsCount > 0 ? 'ATTN' : 'CLEAR'}
          trend="Live Telemetry"
        />
      </div>

      {/* Anomaly Diagnostic Callout Banner if Risk Elevated */}
      {risk > 30 && (
        <div className="p-4 bg-[#840032]/20 border border-[#840032] rounded-lg flex flex-wrap items-center justify-between gap-4 shadow-sm">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-[#840032] text-white border border-[#a80f49] rounded-md">
              <BrainCircuit className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="text-[11px] font-semibold text-[#f07b9e] uppercase tracking-[0.07em] leading-[16px]">
                  ACTIVE INCIDENT DETECTED
                </span>
                <span className="text-[10px] font-sans font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white px-2 py-0.5 rounded">
                  {prediction.failureProbability}% PROBABILITY
                </span>
              </div>
              <p className="text-[12px] font-normal leading-[18px] text-gray-300 mt-0.5">
                Root Cause isolated to:{' '}
                <span className="font-mono font-semibold text-white">
                  {prediction.rootCauseServiceName}
                </span>{' '}
                ({prediction.rootCauseReason}). Recommended action: {prediction.recommendedAction}.
              </p>
            </div>
          </div>

          <button
            onClick={() => setActiveTab('predictions')}
            className="px-3.5 py-1.5 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors shadow-sm"
          >
            VIEW PREDICTION & RECOVERY
          </button>
        </div>
      )}

      {/* Main Metric Charts Grid (CPU, Latency, Memory, Throughput) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <MetricChart
          title="Cluster Avg CPU Utilization"
          data={globalHistory}
          dataKey="cpu"
          lineColor="#AD2831"
          unit="%"
        />

        <MetricChart
          title="Cluster Avg Latency (p95)"
          data={globalHistory}
          dataKey="latency"
          lineColor="#D4A017"
          unit="ms"
        />

        <MetricChart
          title="Cluster Avg Memory Footprint"
          data={globalHistory}
          dataKey="memory"
          lineColor="#89023E"
          unit="%"
        />

        <MetricChart
          title="Total Cluster Throughput (RPS)"
          data={globalHistory}
          dataKey="rps"
          lineColor="#3F8E4F"
          unit="req/s"
        />
      </div>

      {/* Live Recent Event Log Streamer */}
      <div className="space-y-2">
        <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
          RECENT EVENTS
        </h3>
        <LogStreamer logs={logs} maxHeight="max-h-64" />
      </div>
    </div>
  );
};
