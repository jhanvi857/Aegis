import { create } from 'zustand';
import type {
  Microservice,
  LiveAlert,
  LogEntry,
  ChaosInjection,
  PredictionOutput,
  RecoveryHistoryItem,
  Experiment,
  FaultType,
} from '../types/telemetry';
import { telemetryApi } from '../api/telemetry';
import { chaosApi } from '../api/chaos';
import { predictionApi } from '../api/prediction';
import { recoveryApi } from '../api/recovery';

interface TelemetryState {
  services: Microservice[];
  selectedServiceId: string | null;
  activeFaults: ChaosInjection[];
  alerts: LiveAlert[];
  logs: LogEntry[];
  recoveryHistory: RecoveryHistoryItem[];
  experiments: Experiment[];
  latestPrediction: PredictionOutput | null;

  isTimeTravelActive: boolean;
  timeTravelIndex: number;
  historySnapshots: { timestamp: string; services: Microservice[] }[];

  refreshIntervalMs: number;
  isConnectedToBackend: boolean;
  lastBackendSync: string | null;

  // Actions
  setSelectedServiceId: (id: string | null) => void;
  setTimeTravelActive: (active: boolean) => void;
  setTimeTravelIndex: (index: number) => void;

  fetchInitialData: () => Promise<void>;
  tickTelemetry: () => Promise<void>;

  injectChaosFault: (faultType: FaultType, targetServiceId: string, durationSec?: number) => Promise<void>;
  stopChaosFault: (id: string) => Promise<void>;
  clearAllChaosFaults: () => Promise<void>;

  executeRecoveryAction: (actionType: string, targetServiceId: string, actionTitle: string) => Promise<void>;
  runExperimentScenario: (experimentId: string) => Promise<void>;
  resetDemoState: () => Promise<void>;
}

const DEFAULT_PREDICTION: PredictionOutput = {
  failureProbability: 4.5,
  confidence: 98.0,
  estimatedFailureSec: 600,
  rootCauseServiceId: '',
  rootCauseServiceName: 'None',
  rootCauseReason: 'All observed services operating within normal SLA thresholds',
  blastRadius: [],
  recommendedAction: 'Cluster nominal; maintain continuous observation',
  recommendedActionId: 'act-none',
};

export const useTelemetryStore = create<TelemetryState>((set, get) => ({
  services: [],
  selectedServiceId: null,
  activeFaults: [],
  alerts: [],
  logs: [],
  recoveryHistory: [],
  experiments: [],
  latestPrediction: DEFAULT_PREDICTION,

  isTimeTravelActive: false,
  timeTravelIndex: 0,
  historySnapshots: [],

  refreshIntervalMs: 1000,
  isConnectedToBackend: false,
  lastBackendSync: null,

  setSelectedServiceId: (id) => set({ selectedServiceId: id }),

  setTimeTravelActive: (active) =>
    set((state) => {
      const maxIndex = Math.max(0, state.historySnapshots.length - 1);
      return {
        isTimeTravelActive: active,
        timeTravelIndex: active ? maxIndex : state.timeTravelIndex,
      };
    }),

  setTimeTravelIndex: (index) => set({ timeTravelIndex: index }),

  fetchInitialData: async () => {
    try {
      const [services, faults, alerts, logs, prediction, recHistory, experiments] = await Promise.allSettled([
        telemetryApi.getServices(),
        chaosApi.getActiveFaults(),
        telemetryApi.getAlerts(),
        telemetryApi.getLogs(),
        predictionApi.getLatestPrediction(),
        recoveryApi.getHistory(),
        telemetryApi.getExperiments(),
      ]);

      const updates: Partial<TelemetryState> = {
        isConnectedToBackend: true,
        lastBackendSync: new Date().toLocaleTimeString(),
      };

      if (services.status === 'fulfilled' && Array.isArray(services.value)) {
        updates.services = services.value;
      }
      if (faults.status === 'fulfilled' && Array.isArray(faults.value)) {
        updates.activeFaults = faults.value;
      }
      if (alerts.status === 'fulfilled' && Array.isArray(alerts.value)) {
        updates.alerts = alerts.value;
      }
      if (logs.status === 'fulfilled' && Array.isArray(logs.value)) {
        updates.logs = logs.value;
      }
      if (prediction.status === 'fulfilled' && prediction.value) {
        updates.latestPrediction = prediction.value;
      }
      if (recHistory.status === 'fulfilled' && Array.isArray(recHistory.value)) {
        updates.recoveryHistory = recHistory.value;
      }
      if (experiments.status === 'fulfilled' && Array.isArray(experiments.value)) {
        updates.experiments = experiments.value;
      }

      set(updates);
    } catch (err) {
      console.warn('[useTelemetryStore] Backend initial connect failed, will retry on tick:', err);
      set({ isConnectedToBackend: false });
    }
  },

  tickTelemetry: async () => {
    const { isTimeTravelActive, historySnapshots } = get();
    if (isTimeTravelActive) return;

    try {
      const [latestServices, latestFaults, latestAlerts, latestLogs, latestPred] = await Promise.all([
        telemetryApi.getServices(),
        chaosApi.getActiveFaults(),
        telemetryApi.getAlerts(),
        telemetryApi.getLogs(),
        predictionApi.getLatestPrediction(),
      ]);

      const nowTime = new Date().toLocaleTimeString();

      const safeServices = Array.isArray(latestServices) ? latestServices : get().services;
      const safeFaults = Array.isArray(latestFaults) ? latestFaults : [];
      const safeAlerts = Array.isArray(latestAlerts) ? latestAlerts : [];
      const safeLogs = Array.isArray(latestLogs) ? latestLogs : [];
      const safePred = latestPred || get().latestPrediction || DEFAULT_PREDICTION;

      // Record snapshot for time-travel scrubber
      const newSnapshot = {
        timestamp: nowTime,
        services: safeServices,
      };
      const updatedSnapshots = [...historySnapshots.slice(-49), newSnapshot];

      set({
        services: safeServices,
        activeFaults: safeFaults,
        alerts: safeAlerts,
        logs: safeLogs,
        latestPrediction: safePred,
        historySnapshots: updatedSnapshots,
        isConnectedToBackend: true,
        lastBackendSync: nowTime,
      });
    } catch {
      // Backend temporarily offline; mark connection status
      set({ isConnectedToBackend: false });
    }
  },

  injectChaosFault: async (faultType, targetServiceId, durationSec = 60) => {
    try {
      await chaosApi.injectFault(faultType, targetServiceId, durationSec);
      // Immediately refresh state
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error injecting chaos fault:', err);
    }
  },

  stopChaosFault: async (id) => {
    try {
      await chaosApi.stopFault(id);
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error stopping chaos fault:', err);
    }
  },

  clearAllChaosFaults: async () => {
    const { activeFaults } = get();
    for (const fault of activeFaults) {
      try {
        await chaosApi.stopFault(fault.id);
      } catch {
        // continue clearing
      }
    }
    await get().tickTelemetry();
  },

  executeRecoveryAction: async (actionType, targetServiceId, actionTitle) => {
    try {
      await recoveryApi.executeAction(actionType, targetServiceId, actionTitle);
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error executing recovery action:', err);
    }
  },

  runExperimentScenario: async (experimentId) => {
    const { experiments } = get();
    const exp = experiments.find((e) => e.id === experimentId);
    if (!exp) return;
    await get().injectChaosFault(exp.faultType, exp.targetService, 30);
  },

  resetDemoState: async () => {
    await get().clearAllChaosFaults();
    await get().tickTelemetry();
  },
}));

// Helper function to calculate system failure risk percentage from live services and active faults
export function calculateSystemRisk(services: Microservice[], faults: ChaosInjection[]): number {
  if (faults.length > 0) {
    const primaryFault = faults[0];
    if (primaryFault.faultType === 'kill_service') return 98;
    if (primaryFault.faultType === 'cpu_stress') return 91;
    if (primaryFault.faultType === 'latency') return 84;
    return 80;
  }

  const criticalNodes = services.filter((s) => s.status === 'critical');
  const degradedNodes = services.filter((s) => s.status === 'degraded');

  if (criticalNodes.length > 0) {
    return Math.min(99, 80 + criticalNodes.length * 5);
  }
  if (degradedNodes.length > 0) {
    return Math.min(75, 45 + degradedNodes.length * 8);
  }

  return 8;
}

// Return dynamic prediction output from the store or evaluate dynamically
export function getMLPrediction(services: Microservice[], faults: ChaosInjection[]): PredictionOutput {
  const storeState = useTelemetryStore.getState();
  if (storeState.latestPrediction && storeState.latestPrediction.rootCauseServiceId !== '') {
    return storeState.latestPrediction;
  }

  // Derive dynamic prediction from live services if store has not polled yet
  const faultyService = faults.length > 0
    ? services.find((s) => s.id === faults[0].targetServiceId)
    : undefined;

  const criticalService = services.find((s) => s.status === 'critical');
  const degradedService = services.find((s) => s.status === 'degraded');

  const rootSvc = faultyService || criticalService;

  if (rootSvc) {
    // Dynamic blast radius from downstream dependencies
    const blastSet = new Set<string>();
    const queue = [...rootSvc.dependencies];
    while (queue.length > 0) {
      const curr = queue.shift()!;
      if (!blastSet.has(curr)) {
        blastSet.add(curr);
        const depSvc = services.find((s) => s.id === curr);
        if (depSvc) {
          queue.push(...depSvc.dependencies);
        }
      }
    }

    return {
      failureProbability: 92.5,
      confidence: 96.0,
      estimatedFailureSec: 25,
      rootCauseServiceId: rootSvc.id,
      rootCauseServiceName: rootSvc.name,
      rootCauseReason: 'High error rate escalation and latency anomaly',
      blastRadius: Array.from(blastSet),
      recommendedAction: `Execute autonomous remediation on ${rootSvc.name}`,
      recommendedActionId: `act-${rootSvc.id}`,
    };
  }

  if (degradedService) {
    return {
      failureProbability: 45.0,
      confidence: 85.0,
      estimatedFailureSec: 90,
      rootCauseServiceId: degradedService.id,
      rootCauseServiceName: degradedService.name,
      rootCauseReason: 'Moderate latency jitter and queue backpressure',
      blastRadius: [...degradedService.dependencies],
      recommendedAction: `Scale replicas or restart ${degradedService.name}`,
      recommendedActionId: `act-${degradedService.id}`,
    };
  }

  return DEFAULT_PREDICTION;
}
