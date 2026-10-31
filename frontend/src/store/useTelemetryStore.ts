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
import { registryApi, type SystemModeInfo } from '../api/registry';
import { useAuthStore } from './useAuthStore';

interface TelemetryState {
  services: Microservice[];
  selectedServiceId: string | null;
  activeFaults: ChaosInjection[];
  alerts: LiveAlert[];
  logs: LogEntry[];
  recoveryHistory: RecoveryHistoryItem[];
  experiments: Experiment[];
  latestPrediction: PredictionOutput | null;
  systemMode: SystemModeInfo | null;
  autonomousStatus: {
    enabled: boolean;
    policy: string;
    autoExecuteLow: boolean;
    cooldownSec: number;
    maxConcurrent: number;
    activeDispatches: number;
  } | null;

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

  executeRecoveryAction: (actionType: string, targetServiceId: string, actionTitle?: string, approved?: boolean) => Promise<void>;
  approveAction: (actionId: string, targetServiceId?: string, actionType?: string, approver?: string) => Promise<void>;
  rejectAction: (actionId: string, targetServiceId?: string) => Promise<void>;
  runExperimentScenario: (experimentId: string) => Promise<void>;
  resetSystemState: () => Promise<void>;
  resetDemoState: () => Promise<void>;
  fetchSystemMode: () => Promise<void>;
  setSystemMode: (demoMode: boolean) => Promise<void>;
  fetchAutonomousStatus: () => Promise<void>;
  setAutonomousStatus: (enabled: boolean) => Promise<void>;
}

const DEFAULT_PREDICTION: PredictionOutput = {
  failureProbability: 0,
  confidence: 0,
  estimatedFailureSec: 0,
  rootCauseServiceId: '',
  rootCauseServiceName: 'No candidate',
  rootCauseReason: 'Waiting for fresh telemetry and a backend assessment',
  blastRadius: [],
  recommendedAction: 'No action recommended until fresh evidence is available',
  recommendedActionId: 'act-none',
  incident: false,
  state: 'nominal',
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
  systemMode: null,
  autonomousStatus: null,

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
      const [services, faults, alerts, logs, prediction, recHistory, experiments, systemMode, autoStatus] = await Promise.allSettled([
        telemetryApi.getServices(),
        chaosApi.getActiveFaults(),
        telemetryApi.getAlerts(),
        telemetryApi.getLogs(),
        predictionApi.getLatestPrediction(),
        recoveryApi.getHistory(),
        telemetryApi.getExperiments(),
        registryApi.getSystemMode(),
        recoveryApi.getAutonomousStatus(),
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
      if (systemMode.status === 'fulfilled' && systemMode.value) {
        updates.systemMode = systemMode.value;
      }
      if (autoStatus.status === 'fulfilled' && autoStatus.value) {
        updates.autonomousStatus = autoStatus.value;
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
      const [latestServices, latestFaults, latestAlerts, latestLogs, latestPred, latestHist, latestAuto] = await Promise.all([
        telemetryApi.getServices(),
        chaosApi.getActiveFaults(),
        telemetryApi.getAlerts(),
        telemetryApi.getLogs(),
        predictionApi.getLatestPrediction(),
        recoveryApi.getHistory().catch(() => get().recoveryHistory),
        recoveryApi.getAutonomousStatus().catch(() => get().autonomousStatus),
      ]);

      const nowTime = new Date().toLocaleTimeString();

      const safeServices = Array.isArray(latestServices) ? latestServices : get().services;
      const safeFaults = Array.isArray(latestFaults) ? latestFaults : [];
      const safeAlerts = Array.isArray(latestAlerts) ? latestAlerts : [];
      const safeLogs = Array.isArray(latestLogs) ? latestLogs : [];
      const safePred = latestPred || get().latestPrediction || DEFAULT_PREDICTION;
      const safeHist = Array.isArray(latestHist) ? latestHist : get().recoveryHistory;
      const safeAuto = latestAuto || get().autonomousStatus;

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
        recoveryHistory: safeHist,
        autonomousStatus: safeAuto,
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

  executeRecoveryAction: async (actionType, targetServiceId, actionTitle, approved = true) => {
    try {
      const authUser = useAuthStore.getState().user;
      if (!authUser?.email) {
        throw new Error('Operator authentication required to dispatch recovery actions.');
      }
      const approverEmail = authUser.email;

      const actionId = (actionTitle && (actionTitle.startsWith('act-') || actionTitle.startsWith('rec-')))
        ? actionTitle
        : `act-${actionType}-${targetServiceId || 'svc'}`;

      if (approved) {
        // Pre-register operator authorization with Authority Boundary Gate
        await recoveryApi.approveAction(
          actionId,
          targetServiceId,
          actionType,
          approverEmail
        );
      }

      await recoveryApi.executeAction(actionType, targetServiceId, actionId, approved);
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error executing recovery action:', err);
      throw err;
    }
  },

  approveAction: async (actionId, targetServiceId, actionType, approver) => {
    try {
      const authUser = useAuthStore.getState().user;
      const finalApprover = approver || authUser?.email;
      if (!finalApprover) {
        throw new Error('Operator identity required for authority boundary approval.');
      }
      await recoveryApi.approveAction(actionId, targetServiceId, actionType, finalApprover);
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error approving action:', err);
      throw err;
    }
  },

  rejectAction: async (actionId, targetServiceId) => {
    try {
      await recoveryApi.rejectAction(actionId, targetServiceId);
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Error rejecting action:', err);
      throw err;
    }
  },

  runExperimentScenario: async (experimentId) => {
    const { experiments } = get();
    const exp = experiments.find((e) => e.id === experimentId);
    if (!exp) return;
    await get().injectChaosFault(exp.faultType, exp.targetService, 30);
  },

  resetSystemState: async () => {
    await get().clearAllChaosFaults();
    await get().tickTelemetry();
  },

  resetDemoState: async () => {
    await get().resetSystemState();
  },

  fetchSystemMode: async () => {
    try {
      const mode = await registryApi.getSystemMode();
      set({ systemMode: mode });
    } catch (err) {
      console.warn('[useTelemetryStore] Failed to fetch system mode:', err);
    }
  },

  setSystemMode: async (demoMode: boolean) => {
    try {
      const mode = await registryApi.setSystemMode(demoMode);
      set({ systemMode: mode });
      await get().tickTelemetry();
    } catch (err) {
      console.error('[useTelemetryStore] Failed to update system mode:', err);
      throw err;
    }
  },

  fetchAutonomousStatus: async () => {
    try {
      const status = await recoveryApi.getAutonomousStatus();
      set({ autonomousStatus: status });
    } catch (err) {
      console.warn('[useTelemetryStore] Failed to fetch autonomous status:', err);
    }
  },

  setAutonomousStatus: async (enabled: boolean) => {
    try {
      await recoveryApi.setAutonomousStatus(enabled);
      await get().fetchAutonomousStatus();
    } catch (err) {
      console.error('[useTelemetryStore] Failed to update autonomous status:', err);
      throw err;
    }
  },
}));

// Derive system failure risk percentage strictly from backend prediction
export function calculateSystemRisk(_services?: Microservice[], _faults?: ChaosInjection[]): number {
  const storeState = useTelemetryStore.getState();
  const pred = storeState.latestPrediction;
  if (!pred) return 0;
  if (pred.incident === false || pred.state === 'nominal') {
    return 0;
  }
  return Math.round(pred.failureProbability);
}

// Return dynamic prediction output from the store or authentic state if not yet available
export function getMLPrediction(_services?: Microservice[], _faults?: ChaosInjection[]): PredictionOutput {
  const storeState = useTelemetryStore.getState();
  if (storeState.latestPrediction && storeState.latestPrediction.rootCauseServiceId !== undefined) {
    return storeState.latestPrediction;
  }

  // Authentic fallback: do not fabricate numbers when model prediction has not been received
  return {
    failureProbability: 0,
    confidence: 0,
    estimatedFailureSec: 0,
    rootCauseServiceId: '',
    rootCauseServiceName: 'Model Unavailable',
    rootCauseReason: 'Waiting for fresh telemetry and a backend assessment',
    blastRadius: [],
    recommendedAction: 'Awaiting model inference; no action recommended',
    recommendedActionId: 'act-none',
  };
}
