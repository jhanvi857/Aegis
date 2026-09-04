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
    <div className="p-6 space-y-6 font-mono">
      {/* Top Header Banner */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#1E293B] pb-4">
        <div>
          <h1 className="text-xl font-bold text-white tracking-wider uppercase">
            System Mission Control Dashboard
          </h1>
          <p className="text-xs text-slate-400">
            Real-time Distributed Cloud Telemetry, AIOps Anomaly Prediction, and Incident Console
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => setActiveTab('graph')}
            className="flex items-center space-x-2 px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-xs font-bold border border-sky-400 transition-all shadow-md"
          >
            <GitFork className="w-4 h-4 text-sky-200" />
            <span>OPEN DEPENDENCY GRAPH HERO</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Top 5 Mission Control Status Metric Tiles */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <StatusCard
          title="System Health"
          value={isCritical ? 'CRITICAL' : isDegraded ? 'DEGRADED' : 'HEALTHY'}
          icon={Activity}
          statusColor={isCritical ? 'red' : isDegraded ? 'amber' : 'emerald'}
          badgeText={isCritical ? 'ALERT' : isDegraded ? 'WARN' : 'NOMINAL'}
          trend="8 Active Nodes"
        />

        <StatusCard
          title="Failure Risk"
          value={`${risk}%`}
          icon={ShieldAlert}
          statusColor={risk > 70 ? 'red' : risk > 30 ? 'amber' : 'emerald'}
          badgeText={risk > 70 ? 'ELEVATED' : 'STABLE'}
          trend={activeFaults.length > 0 ? `${activeFaults.length} Faults Active` : 'No Faults'}
        />

        <StatusCard
          title="Prediction Horizon"
          value={`${prediction.estimatedFailureSec}s`}
          icon={Clock}
          statusColor="purple"
          badgeText="ML MODEL"
          trend={`Confidence: ${prediction.confidence}%`}
        />

        <StatusCard
          title="Running Services"
          value={`${runningServicesCount}/${services.length}`}
          icon={Server}
          statusColor="sky"
          badgeText="K8S PODS"
          trend="Cluster: us-east-k8s-01"
        />

        <StatusCard
          title="Active Alerts"
          value={activeAlertsCount}
          icon={ShieldAlert}
          statusColor={activeAlertsCount > 0 ? 'amber' : 'emerald'}
          badgeText={activeAlertsCount > 0 ? 'ATTN' : 'CLEAR'}
          trend="Real-time Stream"
        />
      </div>

      {/* Anomaly Diagnostic Callout Banner if Risk Elevated */}
      {risk > 30 && (
        <div className="p-4 bg-rose-950/50 border border-rose-700 rounded-xl flex flex-wrap items-center justify-between gap-4 shadow-xl">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-rose-950 text-rose-300 border border-rose-700 rounded-lg">
              <BrainCircuit className="w-5 h-5 animate-pulse" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="font-bold text-rose-200 text-xs uppercase">
                  AIOps Anomaly Prediction Triggered
                </span>
                <span className="text-[10px] bg-rose-900 text-rose-100 border border-rose-600 px-2 py-0.5 rounded font-bold">
                  {prediction.failureProbability}% PROBABILITY
                </span>
              </div>
              <p className="text-xs text-slate-300 mt-0.5">
                Root Cause isolated to:{' '}
                <span className="font-bold text-white">
                  {prediction.rootCauseServiceName}
                </span>{' '}
                ({prediction.rootCauseReason}). Recommended action: {prediction.recommendedAction}.
              </p>
            </div>
          </div>

          <button
            onClick={() => setActiveTab('predictions')}
            className="px-3.5 py-2 bg-rose-800 hover:bg-rose-700 text-white rounded-lg text-xs font-bold border border-rose-500 transition-colors shadow-md"
          >
            VIEW DIAGNOSTICS & RECOVERY
          </button>
        </div>
      )}

      {/* Main Metric Charts Grid (CPU, Latency, Memory, Throughput) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <MetricChart
          title="Cluster Avg CPU Utilization"
          data={globalHistory}
          dataKey="cpu"
          lineColor="#38BDF8"
          unit="%"
        />

        <MetricChart
          title="Cluster Avg Latency (p95)"
          data={globalHistory}
          dataKey="latency"
          lineColor="#F59E0B"
          unit="ms"
        />

        <MetricChart
          title="Cluster Avg Memory Footprint"
          data={globalHistory}
          dataKey="memory"
          lineColor="#A855F7"
          unit="%"
        />

        <MetricChart
          title="Total Cluster Throughput (RPS)"
          data={globalHistory}
          dataKey="rps"
          lineColor="#06B6D4"
          unit="req/s"
        />
      </div>

      {/* Live Recent Event Log Streamer */}
      <div className="space-y-2">
        <h3 className="text-xs font-bold text-slate-300 uppercase tracking-wider">
          Real-time Event & Log Feed
        </h3>
        <LogStreamer logs={logs} maxHeight="max-h-64" />
      </div>
    </div>
  );
};
