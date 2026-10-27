import React, { useState } from 'react';
import { useTelemetryStore, calculateSystemRisk, getMLPrediction } from '../../store/useTelemetryStore';
import { StatusCard } from '../../components/StatusCard';
import { MetricChart } from '../../components/MetricChart';
import { LogStreamer } from '../../components/LogStreamer';
import { TopologyGraph } from '../../components/TopologyGraph';
import { ChaosControls } from '../../components/ChaosControls';
import { PredictionPanel } from '../../components/PredictionPanel';
import { RecoveryLog } from '../../components/RecoveryLog';
import {
  Activity,
  ShieldAlert,
  Server,
  Clock,
  BrainCircuit,
  Zap,
  RotateCcw,
  Sparkles,
  ShieldCheck,
} from 'lucide-react';

interface DashboardProps {
  setActiveTab: (tab: string) => void;
}

export const Dashboard: React.FC<DashboardProps> = ({ setActiveTab }) => {
  const {
    services,
    activeFaults,
    alerts,
    logs,
    autonomousStatus,
    injectChaosFault,
    clearAllChaosFaults,
  } = useTelemetryStore();

  const [demoBannerMessage, setDemoBannerMessage] = useState<string | null>(null);

  const prediction = getMLPrediction(services, activeFaults);
  const isIncident = prediction.incident === true || prediction.state === 'incident';
  const risk = isIncident ? calculateSystemRisk(services, activeFaults) : 0;

  const isCritical = isIncident && risk >= 80;
  const isDegraded = isIncident && risk >= 40 && risk < 80;

  const runningServicesCount = services.filter((s) => s.dataStatus === 'fresh' && (s.status === 'healthy' || s.status === 'degraded')).length;
  const activeAlertsCount = alerts.filter((a) => !a.resolved).length;
  const isAutonomous = autonomousStatus?.enabled ?? false;
  const hasFreshData = services.some((s) => s.dataStatus === 'fresh');
  const freshServiceCount = services.filter((s) => s.dataStatus === 'fresh').length;

  // Aggregate global metric trends from services history
  const historyLen = services[0]?.history?.length || 0;
  const globalHistory = Array.from({ length: historyLen }).map((_, idx) => {
    const timestamp = services[0]?.history[idx]?.timestamp || '';
    const averageObserved = (signal: string, key: 'cpu' | 'latency' | 'memory') => {
      const values = services.flatMap((s) => {
        const point = s.history?.[idx];
        const fresh = s.dataStatus === 'fresh' && (s.observedSignals?.includes(signal) ?? false);
        return fresh && point && typeof point[key] === 'number' ? [point[key]] : [];
      });
      return values.length ? Math.round(values.reduce((a, b) => a + b, 0) / values.length) : undefined;
    };
    const observedRps = services.flatMap((s) => {
      const point = s.history?.[idx];
      return s.dataStatus === 'fresh' && s.observedSignals?.includes('rps') && point ? [point.rps] : [];
    });

    return {
      timestamp,
      cpu: averageObserved('cpu', 'cpu'),
      latency: averageObserved('latency', 'latency'),
      memory: averageObserved('memory', 'memory'),
      rps: observedRps.length ? Math.round(observedRps.reduce((a, b) => a + b, 0)) : undefined,
    };
  });

  const triggerDemoScenario = async (scenario: 'cpu' | 'latency' | 'crash' | 'leak' | 'reset') => {
    if (scenario === 'reset') {
      await clearAllChaosFaults();
      setDemoBannerMessage('Simulation reset: All active faults terminated. Cluster healthy.');
      setTimeout(() => setDemoBannerMessage(null), 4000);
      return;
    }

    // Adapt target service dynamically to whatever topology is currently loaded
    const ids = services.map((s) => s.id);
    const targetA = ids[0] || 'service-0';
    const targetB = ids.length > 1 ? ids[1] : targetA;
    const targetC = ids.length > 2 ? ids[2] : targetB;
    const targetD = ids.length > 3 ? ids[3] : targetC;

    if (scenario === 'cpu') {
      await injectChaosFault('cpu_stress', targetC, 35);
      setDemoBannerMessage(
        `Scenario 1 Injected: CPU saturation on ${targetC}! Observe the service-level anomaly signal and recommended recovery action.`
      );
    } else if (scenario === 'latency') {
      await injectChaosFault('latency', targetB, 35);
      setDemoBannerMessage(
        `Scenario 2 Injected: 950ms latency deadlock on ${targetB}! Observe propagation & approval gate.`
      );
    } else if (scenario === 'crash') {
      await injectChaosFault('kill_service', targetA, 30);
      setDemoBannerMessage(
        `Scenario 3 Injected: Container SIGKILL crash on ${targetA}! Observe cascade & pod restart.`
      );
    } else if (scenario === 'leak') {
      await injectChaosFault('memory_leak', targetD, 40);
      setDemoBannerMessage(
        `Scenario 4 Injected: Memory leak on ${targetD}! Observe bottom-up SCC DAG recovery ordering.`
      );
    }
    setTimeout(() => setDemoBannerMessage(null), 6000);
  };

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Top Header Banner with 1-Click Interactive Demo Scenarios */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#26262B] pb-4">
        <div>
          <div className="flex items-center space-x-2.5">
            <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
              Aegis Mission Control
            </h1>
            {isAutonomous && <span className="flex items-center space-x-1 px-2.5 py-0.5 rounded-full text-[11px] font-mono font-semibold bg-[#840032]/30 text-[#f07b9e] border border-[#840032]">
              <Sparkles className="w-3 h-3" />
              <span>AUTONOMOUS POLICY ENABLED</span>
            </span>}
          </div>
          <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
            Live service signals, directed dependency analysis, and policy-gated recovery recommendations
          </p>
        </div>

        {/* 1-Click Interactive Demo Flow Trigger Buttons */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider hidden lg:inline">
            DEMO PRESETS:
          </span>

          <button
            onClick={() => triggerDemoScenario('cpu')}
            className="flex items-center space-x-1.5 px-3 py-1.5 bg-[#151517] hover:bg-[#1f1f24] text-gray-200 hover:text-white rounded-md text-xs font-semibold border border-[#26262B] hover:border-[#AD2831] transition-all shadow-sm"
            title="Inject CPU load on a discovered service and watch autonomous recovery"
          >
            <Zap className="w-3.5 h-3.5 text-amber-400" />
            <span>CPU Cascade</span>
          </button>

          <button
            onClick={() => triggerDemoScenario('latency')}
            className="flex items-center space-x-1.5 px-3 py-1.5 bg-[#151517] hover:bg-[#1f1f24] text-gray-200 hover:text-white rounded-md text-xs font-semibold border border-[#26262B] hover:border-[#D4A017] transition-all shadow-sm"
            title="Inject latency on a discovered service and observe approval gate"
          >
            <Activity className="w-3.5 h-3.5 text-blue-400" />
            <span>Latency Gate</span>
          </button>

          <button
            onClick={() => triggerDemoScenario('leak')}
            className="flex items-center space-x-1.5 px-3 py-1.5 bg-[#151517] hover:bg-[#1f1f24] text-gray-200 hover:text-white rounded-md text-xs font-semibold border border-[#26262B] hover:border-[#840032] transition-all shadow-sm"
            title="Inject memory pressure on a discovered service and observe DAG dependency ordering"
          >
            <BrainCircuit className="w-3.5 h-3.5 text-purple-400" />
            <span>Memory SCC</span>
          </button>

          <button
            onClick={() => triggerDemoScenario('reset')}
            className="flex items-center space-x-1.5 px-3 py-1.5 bg-[#151517] hover:bg-[#1f1f24] text-gray-400 hover:text-gray-200 rounded-md text-xs font-semibold border border-[#26262B] transition-all shadow-sm"
            title="Halt all chaos faults and restore healthy baseline"
          >
            <RotateCcw className="w-3.5 h-3.5 text-gray-400" />
            <span>Reset</span>
          </button>
        </div>
      </div>

      {/* Demo Notification Banner */}
      {demoBannerMessage && (
        <div className="p-3 bg-[#840032]/20 border border-[#840032] rounded-lg text-xs font-mono text-[#f07b9e] flex items-center justify-between shadow animate-in fade-in duration-200">
          <div className="flex items-center space-x-2">
            <Zap className="w-4 h-4 text-amber-400" />
            <span>{demoBannerMessage}</span>
          </div>
          <span className="text-[10px] text-gray-400 font-sans">LIVE DEMO FLOW</span>
        </div>
      )}

      {/* Mission Control Status Metric Cards */}
      {services.length === 0 || !hasFreshData || freshServiceCount < services.length ? (
        <div className="rounded-lg border border-amber-700/50 bg-amber-950/20 px-4 py-3 text-xs text-amber-200" role="status">
          {services.length === 0
            ? 'No services are registered yet. Connect an OpenTelemetry source to discover services.'
            : !hasFreshData
              ? `Telemetry is unavailable or stale for all ${services.length} registered services. Health and incident state are unknown.`
              : `Telemetry is fresh for ${freshServiceCount} of ${services.length} registered services. Stale services are excluded from current health summaries.`}
        </div>
      ) : null}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <StatusCard
          title="SYSTEM HEALTH"
          value={!services.length || !hasFreshData ? 'UNKNOWN' : isCritical ? 'CRITICAL' : isDegraded ? 'WARNING' : 'HEALTHY'}
          icon={Activity}
          statusColor={!hasFreshData ? 'warning' : isCritical ? 'critical' : isDegraded ? 'warning' : 'healthy'}
          badgeText={!services.length || !hasFreshData ? 'NO FRESH TELEMETRY' : isCritical ? 'CRITICAL' : isDegraded ? 'WARNING' : 'HEALTHY'}
          trend={`${services.length} discovered services`}
        />

        <StatusCard
          title="FAILURE RISK"
          value={!hasFreshData ? 'UNKNOWN' : isIncident ? `${risk}%` : 'NOMINAL'}
          icon={ShieldAlert}
          statusColor={risk > 70 ? 'critical' : risk > 30 ? 'warning' : 'healthy'}
          badgeText={!hasFreshData ? 'NO FRESH DATA' : isIncident ? (risk > 70 ? 'HIGH SCORE' : 'ANOMALY') : 'NO INCIDENT'}
          trend={isIncident ? (prediction.faultFamily ? `Fault: ${prediction.faultFamily}` : `${activeAlertsCount} Alerts`) : hasFreshData ? 'No current anomaly evidence' : 'Awaiting telemetry'}
        />

        <StatusCard
          title="PREDICTION HORIZON"
          value={isIncident && prediction.estimatedFailureSec > 0 ? `${prediction.estimatedFailureSec}s` : 'n/a'}
          icon={Clock}
          statusColor="intervention"
          badgeText="Baseline"
          trend={isIncident && prediction.confidence > 0 ? `Confidence: ${prediction.confidence}%` : 'Model: Nominal'}
        />

        <StatusCard
          title="RUNNING SERVICES"
          value={!services.length || !hasFreshData ? '—' : `${runningServicesCount}/${services.length}`}
          icon={Server}
          statusColor="healthy"
          badgeText={hasFreshData ? 'OBSERVED' : 'UNKNOWN'}
          trend={hasFreshData ? 'Based on reported service state' : 'Waiting for telemetry'}
        />

        <StatusCard
          title="AUTONOMOUS RECOVERY"
          value={isAutonomous ? 'ACTIVE' : 'PAUSED'}
          icon={ShieldCheck}
          statusColor={isAutonomous ? 'healthy' : 'warning'}
          badgeText={isAutonomous ? 'CLOSED-LOOP' : 'MANUAL'}
          trend="Rulebook Planner"
        />
      </div>

      {/* Active Anomaly Diagnostic Callout Banner */}
      {isIncident && (
        <div className="p-4 bg-[#840032]/20 border border-[#840032] rounded-lg flex flex-wrap items-center justify-between gap-4 shadow-sm animate-pulse">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-[#840032] text-white border border-[#a80f49] rounded-md">
              <BrainCircuit className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="text-[11px] font-semibold text-[#f07b9e] uppercase tracking-[0.07em] leading-[16px]">
                  LIVE INCIDENT FROM OBSERVED SIGNALS
                </span>
                <span className="text-[10px] font-mono font-semibold tracking-[0.04em] uppercase bg-[#840032] text-white px-2 py-0.5 rounded">
                  {prediction.failureProbability}% SCORE
                </span>
              </div>
              <p className="text-[12px] font-normal leading-[18px] text-gray-300 mt-0.5">
                Leading candidate:{' '}
                <span className="font-mono font-semibold text-white">
                  {prediction.rootCauseServiceName || prediction.rootCauseServiceId}
                </span>{' '}
                ({prediction.rootCauseReason}). Mitigation:{' '}
                <span className="font-semibold text-white">{prediction.recommendedAction}</span>.
              </p>
            </div>
          </div>

          <button
            onClick={() => setActiveTab('predictions')}
            className="px-3.5 py-1.5 bg-[#840032] hover:bg-[#9b053d] text-white rounded-md text-xs font-semibold border border-[#840032] transition-colors shadow-sm"
          >
            DEEP DIVE →
          </button>
        </div>
      )}

      {/* Main Core Section: TopologyGraph (Left 60%) + PredictionPanel (Right 40%) */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        <div className="lg:col-span-7 flex flex-col">
          <TopologyGraph
            height="460px"
            showMiniMap={true}
            showControls={true}
            showTopologySelector={true}
            onNodeSelect={(nodeId) => console.log('Selected node:', nodeId)}
            className="h-full shadow-lg"
          />
        </div>

        <div className="lg:col-span-5 flex flex-col">
          <PredictionPanel className="h-full shadow-lg" />
        </div>
      </div>

      {/* Secondary Section: RecoveryLog (Left 50%) + ChaosControls (Right 50%) */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <RecoveryLog className="shadow-lg" />
        <ChaosControls className="shadow-lg" />
      </div>

      {/* Main Metric Charts Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
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
        <div className="flex items-center justify-between">
          <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
            REAL-TIME SYSTEM AUDIT & TELEMETRY STREAM
          </h3>
          <span className="text-[10px] text-gray-400 font-mono">
            {logs.length} events logged
          </span>
        </div>
        <LogStreamer logs={logs} maxHeight="max-h-64" />
      </div>
    </div>
  );
};
