export type HealthStatus = 'healthy' | 'degraded' | 'critical';

export interface ServiceMetric {
  timestamp: string;
  cpu: number;        // Percentage 0-100
  memory: number;     // Percentage 0-100
  latency: number;    // Milliseconds
  rps: number;        // Requests per second
  errorRate: number;  // Percentage 0-100
  queueDepth: number; // Pending queue count
}

export interface Microservice {
  id: string;
  name: string;
  type: 'gateway' | 'service' | 'database' | 'cache' | 'queue';
  status: HealthStatus;
  replicas: number;
  maxReplicas: number;
  cpu: number;
  memory: number;
  latency: number;
  rps: number;
  errorRate: number;
  queueDepth: number;
  version: string;
  uptime: string;
  dependencies: string[]; // Service IDs this service calls
  history: ServiceMetric[];
}

export interface LiveAlert {
  id: string;
  timestamp: string;
  serviceId: string;
  serviceName: string;
  severity: 'info' | 'warning' | 'critical';
  message: string;
  resolved: boolean;
}

export interface LogEntry {
  id: string;
  timestamp: string;
  serviceId: string;
  serviceName: string;
  level: 'INFO' | 'WARN' | 'ERROR';
  message: string;
}

export type FaultType =
  | 'latency'
  | 'cpu_stress'
  | 'kill_service'
  | 'memory_leak'
  | 'packet_loss'
  | 'mq_lag'
  | 'cache_down'
  | 'db_lock'
  | 'slow_query'
  | 'thread_exhaustion';

export interface ChaosInjection {
  id: string;
  faultType: FaultType;
  title: string;
  targetServiceId: string;
  targetServiceName: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  durationSeconds: number;
  remainingSeconds: number;
  status: 'pending' | 'running' | 'completed' | 'cancelled';
  startedAt: string;
}

export interface AffectedNodeInfo {
  nodeId: string;
  hopDistance: number;
  etaSeconds: number;
  riskScore: number;
}

export interface PredictionOutput {
  failureProbability: number; // 0 - 100
  confidence: number;          // 0 - 100
  estimatedFailureSec: number;
  rootCauseServiceId: string;
  rootCauseServiceName: string;
  rootCauseReason: string;
  faultFamily?: string;
  blastRadius: string[];      // Array of affected service IDs in order of cascade
  affectedNodes?: AffectedNodeInfo[];
  recommendedAction: string;
  recommendedActionId: string;
  recommendedActions?: RecoveryAction[];
}

export interface RecoveryAction {
  id: string;
  title: string;
  description: string;
  targetServiceId: string;
  actionType: 'scale' | 'restart' | 'flush_cache' | 'pool_expand' | 'increase_pool' | 'reroute_traffic' | 'isolate' | string;
  confidence: number;
  status: 'idle' | 'executing' | 'completed' | 'failed' | 'pending_approval';
  riskLevel?: 'LOW' | 'MEDIUM' | 'HIGH';
  requiresApproval?: boolean;
  parameters?: Record<string, string>;
  executedAt?: string;
}

export interface RecoveryHistoryItem {
  id: string;
  timestamp: string;
  actionTitle: string;
  targetService: string;
  status: 'Completed' | 'Failed' | 'Verified' | 'Ineffective' | 'RolledBack' | 'Verifying';
  duration: string;
  riskBefore: number;
  riskAfter: number;
}

export interface PolicyAssessment {
  actionType: string;
  targetServiceId: string;
  targetTier: string;
  targetCriticality: number;
  blastRadius: string[];
  blastRadiusSize: number;
  riskScore: number;
  riskLevel: 'LOW' | 'MEDIUM' | 'HIGH';
  requiresApproval: boolean;
  cooldownRemainingSec: number;
  justification: string;
}

export interface Experiment {
  id: string;
  name: string;
  description: string;
  faultType: FaultType;
  targetService: string;
  predictionAccuracy: number;
  detectionTimeSec: number;
  mttrSec: number;
  status: 'idle' | 'running' | 'passed';
}

export interface SystemOverview {
  overallStatus: HealthStatus;
  runningServicesCount: number;
  activeAlertsCount: number;
  systemFailureRisk: number; // Percentage
  predictionHorizonSec: number;
  avgLatency: number;
  avgCpu: number;
  avgMemory: number;
  totalRps: number;
}
