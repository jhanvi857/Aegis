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
  HealthStatus,
  ServiceMetric,
} from '../types/telemetry';

// Seed microservices initial topologies
const INITIAL_SERVICES: Microservice[] = [
  {
    id: 'gateway',
    name: 'API Gateway',
    type: 'gateway',
    status: 'healthy',
    replicas: 4,
    maxReplicas: 10,
    cpu: 22,
    memory: 45,
    latency: 18,
    rps: 1250,
    errorRate: 0.1,
    queueDepth: 2,
    version: 'v2.4.1',
    uptime: '14d 6h',
    dependencies: ['auth', 'orders'],
    history: [],
  },
  {
    id: 'auth',
    name: 'Auth Service',
    type: 'service',
    status: 'healthy',
    replicas: 3,
    maxReplicas: 8,
    cpu: 28,
    memory: 52,
    latency: 24,
    rps: 840,
    errorRate: 0.2,
    queueDepth: 1,
    version: 'v1.9.0',
    uptime: '14d 6h',
    dependencies: ['redis'],
    history: [],
  },
  {
    id: 'orders',
    name: 'Orders Service',
    type: 'service',
    status: 'healthy',
    replicas: 5,
    maxReplicas: 12,
    cpu: 34,
    memory: 58,
    latency: 25,
    rps: 620,
    errorRate: 0.3,
    queueDepth: 4,
    version: 'v3.1.0',
    uptime: '10d 2h',
    dependencies: ['payment', 'notifications'],
    history: [],
  },
  {
    id: 'payment',
    name: 'Payment Service',
    type: 'service',
    status: 'healthy',
    replicas: 4,
    maxReplicas: 15,
    cpu: 38,
    memory: 64,
    latency: 35,
    rps: 410,
    errorRate: 0.4,
    queueDepth: 3,
    version: 'v2.0.4',
    uptime: '8d 19h',
    dependencies: ['database', 'redis'],
    history: [],
  },
  {
    id: 'database',
    name: 'PostgreSQL DB',
    type: 'database',
    status: 'healthy',
    replicas: 2,
    maxReplicas: 4,
    cpu: 42,
    memory: 71,
    latency: 12,
    rps: 1890,
    errorRate: 0.05,
    queueDepth: 5,
    version: 'v15.3',
    uptime: '45d 12h',
    dependencies: [],
    history: [],
  },
  {
    id: 'redis',
    name: 'Redis Cache',
    type: 'cache',
    status: 'healthy',
    replicas: 3,
    maxReplicas: 6,
    cpu: 18,
    memory: 38,
    latency: 2,
    rps: 3400,
    errorRate: 0.01,
    queueDepth: 0,
    version: 'v7.0',
    uptime: '45d 12h',
    dependencies: [],
    history: [],
  },
  {
    id: 'kafka',
    name: 'Kafka Broker',
    type: 'queue',
    status: 'healthy',
    replicas: 3,
    maxReplicas: 6,
    cpu: 31,
    memory: 62,
    latency: 8,
    rps: 2100,
    errorRate: 0.1,
    queueDepth: 12,
    version: 'v3.4',
    uptime: '30d 14h',
    dependencies: [],
    history: [],
  },
  {
    id: 'notifications',
    name: 'Notification Service',
    type: 'service',
    status: 'healthy',
    replicas: 2,
    maxReplicas: 6,
    cpu: 25,
    memory: 40,
    latency: 42,
    rps: 190,
    errorRate: 0.5,
    queueDepth: 8,
    version: 'v1.4.2',
    uptime: '12d 8h',
    dependencies: ['kafka'],
    history: [],
  },
];

// Initial baseline mock history generator
function generateInitialHistory(baseCpu: number, baseLat: number, baseMem: number): ServiceMetric[] {
  const history: ServiceMetric[] = [];
  const now = Date.now();
  for (let i = 20; i >= 0; i--) {
    const time = new Date(now - i * 5000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    history.push({
      timestamp: time,
      cpu: Math.max(10, Math.min(95, baseCpu + (Math.random() * 8 - 4))),
      memory: Math.max(15, Math.min(95, baseMem + (Math.random() * 4 - 2))),
      latency: Math.max(2, Math.min(500, baseLat + (Math.random() * 10 - 5))),
      rps: Math.floor(Math.random() * 100 + 300),
      errorRate: Math.random() * 0.5,
      queueDepth: Math.floor(Math.random() * 5),
    });
  }
  return history;
}

const INITIAL_EXPERIMENTS: Experiment[] = [
  {
    id: 'exp-1',
    name: 'Experiment 1: Slow Database Connection Saturation',
    description: 'Inject 300ms query latency into PostgreSQL and measure ML prediction lead time.',
    faultType: 'slow_db',
    targetService: 'database',
    predictionAccuracy: 96.4,
    detectionTimeSec: 4.2,
    mttrSec: 18.5,
    status: 'passed',
  },
  {
    id: 'exp-2',
    name: 'Experiment 2: Redis Sentinel Cache Crash',
    description: 'Simulate instant Redis cache eviction forcing 100% database fallback queries.',
    faultType: 'kill_redis',
    targetService: 'redis',
    predictionAccuracy: 94.1,
    detectionTimeSec: 3.1,
    mttrSec: 12.0,
    status: 'passed',
  },
  {
    id: 'exp-3',
    name: 'Experiment 3: Payment Microservice Memory Leak',
    description: 'Simulate memory pressure up to 98% in Payment Service pods.',
    faultType: 'memory_leak',
    targetService: 'payment',
    predictionAccuracy: 91.8,
    detectionTimeSec: 6.5,
    mttrSec: 24.0,
    status: 'passed',
  },
];

interface TelemetryState {
  services: Microservice[];
  selectedServiceId: string | null;
  activeFaults: ChaosInjection[];
  alerts: LiveAlert[];
  logs: LogEntry[];
  recoveryHistory: RecoveryHistoryItem[];
  experiments: Experiment[];
  
  // Time travel state
  isTimeTravelActive: boolean;
  timeTravelIndex: number; // 0 to history.length - 1
  historySnapshots: { timestamp: string; services: Microservice[]; risk: number }[];

  // App settings
  refreshIntervalMs: number;
  useRealWebSocket: boolean;
  isSimulating: boolean;

  // Actions
  setSelectedServiceId: (id: string | null) => void;
  injectChaosFault: (faultType: FaultType, targetServiceId: string, durationSec?: number) => void;
  stopChaosFault: (id: string) => void;
  clearAllChaosFaults: () => void;
  executeRecoveryAction: (actionType: 'scale' | 'restart' | 'flush_cache' | 'pool_expand', targetServiceId: string, actionTitle: string) => void;
  runExperimentScenario: (experimentId: string) => void;
  setTimeTravelActive: (active: boolean) => void;
  setTimeTravelIndex: (index: number) => void;
  resetDemoState: () => void;
  tickTelemetry: () => void;
}

export const useTelemetryStore = create<TelemetryState>((set, get) => {
  // Populate initial service metric histories
  const seededServices = INITIAL_SERVICES.map((s) => ({
    ...s,
    history: generateInitialHistory(s.cpu, s.latency, s.memory),
  }));

  return {
    services: seededServices,
    selectedServiceId: null,
    activeFaults: [],
    alerts: [
      {
        id: 'alt-1',
        timestamp: new Date().toLocaleTimeString(),
        serviceId: 'database',
        serviceName: 'PostgreSQL DB',
        severity: 'info',
        message: 'Normal query volume verified. Connection pool at 42% capacity.',
        resolved: true,
      },
    ],
    logs: [
      {
        id: 'log-1',
        timestamp: new Date().toLocaleTimeString(),
        serviceId: 'gateway',
        serviceName: 'API Gateway',
        level: 'INFO',
        message: 'Ingress controller health check passed OK.',
      },
      {
        id: 'log-2',
        timestamp: new Date().toLocaleTimeString(),
        serviceId: 'payment',
        serviceName: 'Payment Service',
        level: 'INFO',
        message: 'Stripe gateway connection pool initialized with 20 workers.',
      },
    ],
    recoveryHistory: [
      {
        id: 'rec-0',
        timestamp: '09:31:12',
        actionTitle: 'Scaled Payment Service Replicas',
        targetService: 'Payment Service',
        status: 'Completed',
        duration: '4.2s',
        riskBefore: 78,
        riskAfter: 12,
      },
    ],
    experiments: INITIAL_EXPERIMENTS,
    
    isTimeTravelActive: false,
    timeTravelIndex: 20,
    historySnapshots: [],

    refreshIntervalMs: 1000,
    useRealWebSocket: false,
    isSimulating: true,

    setSelectedServiceId: (id) => set({ selectedServiceId: id }),

    setTimeTravelActive: (active) => set({ isTimeTravelActive: active }),

    setTimeTravelIndex: (index) => set({ timeTravelIndex: index }),

    injectChaosFault: (faultType, targetServiceId, durationSec = 60) => {
      const { services, activeFaults, logs } = get();
      const targetService = services.find((s) => s.id === targetServiceId);
      if (!targetService) return;

      const faultTitles: Record<FaultType, string> = {
        slow_db: 'Slow Database Query Latency',
        high_cpu: 'High CPU Core Saturation',
        kill_redis: 'Kill Redis Cache Sentinel',
        kafka_lag: 'Kafka Message Queue Lag',
        packet_loss: 'Network Packet Loss',
        memory_leak: 'Payment Pod Memory Leak',
        network_delay: 'Cross-AZ Network Latency',
      };

      const newFault: ChaosInjection = {
        id: `fault-${Date.now()}`,
        faultType,
        title: faultTitles[faultType] || 'Fault Injection',
        targetServiceId: targetService.id,
        targetServiceName: targetService.name,
        severity: 'critical',
        durationSeconds: durationSec,
        remainingSeconds: durationSec,
        status: 'running',
        startedAt: new Date().toLocaleTimeString(),
      };

      const newLog: LogEntry = {
        id: `log-fault-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        serviceId: targetService.id,
        serviceName: targetService.name,
        level: 'ERROR',
        message: `CHAOS ENGINE INJECTED: ${newFault.title} on ${targetService.name}`,
      };

      const newAlert: LiveAlert = {
        id: `alt-fault-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        serviceId: targetService.id,
        serviceName: targetService.name,
        severity: 'critical',
        message: `Fault injected: ${newFault.title} on ${targetService.name}`,
        resolved: false,
      };

      set({
        activeFaults: [newFault, ...activeFaults],
        logs: [newLog, ...logs.slice(0, 99)],
        alerts: [newAlert, ...get().alerts.slice(0, 49)],
      });
    },

    stopChaosFault: (id) => {
      const { activeFaults, logs } = get();
      const fault = activeFaults.find((f) => f.id === id);
      if (!fault) return;

      const updated = activeFaults.filter((f) => f.id !== id);
      const newLog: LogEntry = {
        id: `log-stop-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        serviceId: fault.targetServiceId,
        serviceName: fault.targetServiceName,
        level: 'INFO',
        message: `CHAOS ENGINE CLEARED: Fault ${fault.title} terminated.`,
      };

      set({
        activeFaults: updated,
        logs: [newLog, ...logs.slice(0, 99)],
      });
    },

    clearAllChaosFaults: () => {
      set({ activeFaults: [] });
    },

    executeRecoveryAction: (actionType, targetServiceId, actionTitle) => {
      const { services, activeFaults, logs, recoveryHistory } = get();
      const service = services.find((s) => s.id === targetServiceId);
      const targetName = service ? service.name : targetServiceId;

      // Calculate current failure risk before action
      const currentRisk = calculateSystemRisk(services, activeFaults);

      // Perform recovery action
      const updatedServices = services.map((s) => {
        if (s.id === targetServiceId) {
          if (actionType === 'scale') {
            return { ...s, replicas: Math.min(s.maxReplicas, s.replicas + 3), status: 'healthy' as HealthStatus, cpu: 30, latency: 25, errorRate: 0.1 };
          }
          if (actionType === 'restart') {
            return { ...s, status: 'healthy' as HealthStatus, cpu: 20, latency: 15, errorRate: 0.05, queueDepth: 0 };
          }
          if (actionType === 'pool_expand' || actionType === 'flush_cache') {
            return { ...s, status: 'healthy' as HealthStatus, cpu: 25, latency: 12, queueDepth: 2, errorRate: 0.02 };
          }
        }
        return s;
      });

      // Clear related active faults
      const remainingFaults = activeFaults.filter((f) => f.targetServiceId !== targetServiceId);

      const postRisk = calculateSystemRisk(updatedServices, remainingFaults);

      const historyItem: RecoveryHistoryItem = {
        id: `rec-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        actionTitle,
        targetService: targetName,
        status: 'Completed',
        duration: '1.8s',
        riskBefore: currentRisk,
        riskAfter: postRisk,
      };

      const recoveryLog: LogEntry = {
        id: `log-rec-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        serviceId: targetServiceId,
        serviceName: targetName,
        level: 'INFO',
        message: `RECOVERY EXECUTED: ${actionTitle} on ${targetName}. Risk reduced from ${currentRisk}% to ${postRisk}%.`,
      };

      set({
        services: updatedServices,
        activeFaults: remainingFaults,
        recoveryHistory: [historyItem, ...recoveryHistory],
        logs: [recoveryLog, ...logs.slice(0, 99)],
      });
    },

    runExperimentScenario: (experimentId) => {
      const { experiments } = get();
      const exp = experiments.find((e) => e.id === experimentId);
      if (!exp) return;

      // Inject the experiment fault
      get().injectChaosFault(exp.faultType, exp.targetService, 30);
    },

    resetDemoState: () => {
      const resetServices = INITIAL_SERVICES.map((s) => ({
        ...s,
        status: 'healthy' as HealthStatus,
        cpu: s.cpu,
        memory: s.memory,
        latency: s.latency,
        errorRate: s.errorRate,
        history: generateInitialHistory(s.cpu, s.latency, s.memory),
      }));

      set({
        services: resetServices,
        activeFaults: [],
        alerts: [
          {
            id: `alt-reset-${Date.now()}`,
            timestamp: new Date().toLocaleTimeString(),
            serviceId: 'gateway',
            serviceName: 'API Gateway',
            severity: 'info',
            message: 'System state reset to baseline healthy telemetry parameters.',
            resolved: true,
          },
        ],
        logs: [
          {
            id: `log-reset-${Date.now()}`,
            timestamp: new Date().toLocaleTimeString(),
            serviceId: 'gateway',
            serviceName: 'API Gateway',
            level: 'INFO',
            message: 'Demo state reset complete. All services operating normally.',
          },
        ],
        isTimeTravelActive: false,
      });
    },

    tickTelemetry: () => {
      const { services, activeFaults, logs, alerts, isTimeTravelActive, historySnapshots } = get();
      if (isTimeTravelActive) return; // Freeze live tick during time travel replay

      const nowTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

      // Process fault countdowns
      const updatedFaults: ChaosInjection[] = activeFaults
        .map((f) => ({ ...f, remainingSeconds: f.remainingSeconds - 1 }))
        .filter((f) => f.remainingSeconds > 0);

      // Determine active affected services
      const faultyServiceIds = new Set(updatedFaults.map((f) => f.targetServiceId));

      const newLogs: LogEntry[] = [];
      const newAlerts: LiveAlert[] = [];

      const nextServices = services.map((service) => {
        let cpu = service.cpu;
        let memory = service.memory;
        let latency = service.latency;
        let errorRate = service.errorRate;
        let queueDepth = service.queueDepth;
        let status: HealthStatus = 'healthy';

        const isDirectFault = faultyServiceIds.has(service.id);
        const hasUpstreamFault = service.dependencies.some((depId) => faultyServiceIds.has(depId));

        if (isDirectFault) {
          const fault = updatedFaults.find((f) => f.targetServiceId === service.id);
          status = 'critical';
          if (fault?.faultType === 'slow_db' || service.id === 'database') {
            cpu = Math.min(98, cpu + Math.floor(Math.random() * 5 + 3));
            latency = Math.min(450, latency + Math.floor(Math.random() * 20 + 15));
            queueDepth = Math.min(80, queueDepth + 4);
            errorRate = Math.min(15, errorRate + 0.8);
          } else if (fault?.faultType === 'high_cpu') {
            cpu = Math.min(99, cpu + Math.floor(Math.random() * 6 + 5));
            latency = Math.min(300, latency + 10);
          } else if (fault?.faultType === 'kill_redis') {
            latency = Math.min(250, latency + 15);
            errorRate = Math.min(25, errorRate + 2.5);
          } else {
            cpu = Math.min(95, cpu + 5);
            latency = Math.min(350, latency + 25);
            errorRate = Math.min(20, errorRate + 1.2);
          }
        } else if (hasUpstreamFault) {
          // Cascading impact on dependent services
          status = 'degraded';
          latency = Math.min(280, latency + Math.floor(Math.random() * 8 + 5));
          cpu = Math.min(85, cpu + Math.floor(Math.random() * 3 + 2));
          errorRate = Math.min(8, errorRate + 0.3);
          queueDepth = Math.min(35, queueDepth + 2);
        } else {
          // Normal fluctuation around baseline
          status = 'healthy';
          cpu = Math.max(15, Math.min(50, cpu + (Math.random() * 4 - 2)));
          memory = Math.max(20, Math.min(75, memory + (Math.random() * 2 - 1)));
          latency = Math.max(5, Math.min(45, latency + (Math.random() * 4 - 2)));
          errorRate = Math.max(0.01, Math.min(1.0, errorRate + (Math.random() * 0.1 - 0.05)));
          queueDepth = Math.max(0, Math.min(10, queueDepth + (Math.random() > 0.5 ? 1 : -1)));
        }

        // Round values cleanly
        cpu = Math.round(cpu * 10) / 10;
        memory = Math.round(memory * 10) / 10;
        latency = Math.round(latency);
        errorRate = Math.round(errorRate * 100) / 100;

        // Generate periodic log entries
        if (Math.random() > 0.65) {
          const level = status === 'critical' ? 'ERROR' : status === 'degraded' ? 'WARN' : 'INFO';
          const msg = status === 'critical'
            ? `High latency detected: ${latency}ms, CPU: ${cpu}%. Connections queue saturating.`
            : status === 'degraded'
            ? `Upstream dependency latency degradation observed (${latency}ms).`
            : `Processed HTTP request batch OK. Latency: ${latency}ms.`;

          newLogs.push({
            id: `log-tick-${Date.now()}-${service.id}`,
            timestamp: nowTime,
            serviceId: service.id,
            serviceName: service.name,
            level,
            message: msg,
          });
        }

        // Append to history sliding window (keep last 25 points)
        const updatedHistory: ServiceMetric[] = [
          ...service.history.slice(1),
          {
            timestamp: nowTime,
            cpu,
            memory,
            latency,
            rps: service.rps + Math.floor(Math.random() * 40 - 20),
            errorRate,
            queueDepth,
          },
        ];

        return {
          ...service,
          status,
          cpu,
          memory,
          latency,
          errorRate,
          queueDepth,
          history: updatedHistory,
        };
      });

      // Maintain time travel snapshot buffer (last 30 snapshots)
      const currentRisk = calculateSystemRisk(nextServices, updatedFaults);
      const updatedSnapshots = [
        ...historySnapshots.slice(-29),
        { timestamp: nowTime, services: nextServices, risk: currentRisk },
      ];

      set({
        services: nextServices,
        activeFaults: updatedFaults,
        logs: [...newLogs, ...logs].slice(0, 100),
        alerts: [...newAlerts, ...alerts].slice(0, 50),
        historySnapshots: updatedSnapshots,
      });
    },
  };
});

// Helper function to calculate system risk percentage
export function calculateSystemRisk(services: Microservice[], faults: ChaosInjection[]): number {
  if (faults.length > 0) {
    const activeFault = faults[0];
    if (activeFault.faultType === 'slow_db') return 97;
    if (activeFault.faultType === 'high_cpu') return 89;
    if (activeFault.faultType === 'kill_redis') return 84;
    return 79;
  }
  const criticalCount = services.filter((s) => s.status === 'critical').length;
  const degradedCount = services.filter((s) => s.status === 'degraded').length;
  if (criticalCount > 0) return 92;
  if (degradedCount > 0) return 54;
  return 12;
}

// Derive prediction outputs based on active state
export function getMLPrediction(services: Microservice[], faults: ChaosInjection[]): PredictionOutput {
  const isFaulty = faults.length > 0;
  const criticalService = services.find((s) => s.status === 'critical');
  const degradedService = services.find((s) => s.status === 'degraded');

  if (isFaulty || criticalService) {
    const rootId = faults[0]?.targetServiceId || criticalService?.id || 'database';
    const rootName = faults[0]?.targetServiceName || criticalService?.name || 'PostgreSQL DB';
    return {
      failureProbability: 97,
      confidence: 95,
      estimatedFailureSec: 26,
      rootCauseServiceId: rootId,
      rootCauseServiceName: rootName,
      rootCauseReason: 'Connection pool saturation causing cascade upstream latency',
      blastRadius: [rootId, 'payment', 'orders', 'gateway'],
      recommendedAction: 'Scale Payment Replicas x3 & Increase DB Connection Pool',
      recommendedActionId: 'action-scale-payment',
    };
  }

  if (degradedService) {
    return {
      failureProbability: 48,
      confidence: 82,
      estimatedFailureSec: 85,
      rootCauseServiceId: degradedService.id,
      rootCauseServiceName: degradedService.name,
      rootCauseReason: 'Elevated latency jitter in downstream service dependencies',
      blastRadius: [degradedService.id, 'gateway'],
      recommendedAction: 'Flush Redis Cache & Scale Worker Replicas',
      recommendedActionId: 'action-flush-redis',
    };
  }

  return {
    failureProbability: 12,
    confidence: 98,
    estimatedFailureSec: 3600,
    rootCauseServiceId: 'none',
    rootCauseServiceName: 'None (System Optimal)',
    rootCauseReason: 'All telemetry metrics operating within nominal SLA thresholds',
    blastRadius: [],
    recommendedAction: 'No action required. Telemetry metrics stable.',
    recommendedActionId: 'none',
  };
}
