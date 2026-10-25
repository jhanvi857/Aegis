import { apiFetch, API_BASE_URL } from './client';

export type MonitoringMode = 'ignore' | 'observe' | 'recommend' | 'auto_remediate';
export type ServiceStatus = 'unconnected' | 'healthy_data' | 'no_data' | 'stale' | 'healthy' | 'degraded' | 'critical';
export type EdgeSource = 'manual' | 'suggested' | 'discovered';
export type EdgeState = 'active' | 'suggested' | 'rejected';
export type EdgeType = 'sync' | 'async' | 'db' | 'cache';

export interface TelemetrySource {
  type: 'prometheus' | 'kubernetes' | 'agent_push' | string;
  baseUrl?: string;
  job?: string;
  selector?: string;
  cpuQuery?: string;
  memoryQuery?: string;
  latencyQuery?: string;
  errorRateQuery?: string;
  queueDepthQuery?: string;
  clusterId?: string;
  namespace?: string;
  workloadKind?: string;
  workloadName?: string;
  serviceId?: string;
  apiKey?: string;
  authToken?: string;
  authUsername?: string;
  authPassword?: string;
  hasAuth?: boolean;
}

export interface RecoveryBinding {
  runtime: 'kubernetes' | 'docker' | string;
  target: string;
  allowedActions: string[];
}

export interface TopologyServiceItem {
  id: string;
  name: string;
  type?: string;
  namespace?: string;
  clusterId?: string;
  tier?: string;
  capabilities?: string;
  owner?: string;
  version?: string;
  replicas?: number;
  maxReplicas?: number;
  monitoringMode?: MonitoringMode;
  telemetrySource?: TelemetrySource;
  recoveryBinding?: RecoveryBinding;
  status?: ServiceStatus | string;
  description?: string;
  tags?: string[];
  lastTelemetryTime?: string;
  cpu?: number;
  memory?: number;
  latency?: number;
  rps?: number;
  errorRate?: number;
  queueDepth?: number;
}

export interface TopologyEdgeItem {
  id?: string;
  sourceServiceId: string;
  targetServiceId: string;
  relationship?: string;
  namespace?: string;
  clusterId?: string;
  source?: EdgeSource;
  state?: EdgeState;
  edgeType?: EdgeType;
  confidence?: number;
  createdBy?: string;
  isManualOverride?: boolean;
  sampleCount?: number;
}

export interface TopologyVersionItem {
  id: string;
  version: number;
  description: string;
  snapshot: Record<string, any>;
  createdBy: string;
  createdAt: string;
}

export interface SystemModeInfo {
  demoMode: boolean;
  simulationMode: boolean;
  modeLabel: string;
  clusterId: string;
  namespace: string;
}

export interface TopologyInfo {
  id: string;
  filename: string;
  name: string;
  description: string;
  nodeCount: number;
  active: boolean;
}

export interface ValidationIssue {
  type: 'error' | 'warning' | 'info';
  code: string;
  message: string;
  serviceId?: string;
  sourceId?: string;
  targetId?: string;
  details?: string[];
}

export interface ValidationSummary {
  serviceCount: number;
  edgeCount: number;
  errorCount: number;
  warningCount: number;
  sccCycles: number;
}

export interface ValidationResponse {
  valid: boolean;
  issues: ValidationIssue[];
  summary: ValidationSummary;
}

export interface TestConnectionResponse {
  success: boolean;
  metricsFound?: string[];
  lastSampleTime?: string;
  errors?: string[];
}

export interface ImportPreviewDiff {
  additions: any[];
  modifications: any[];
  deletions: any[];
}

export interface ImportResponse {
  success: boolean;
  dryRun?: boolean;
  version?: number;
  servicesCount?: number;
  edgesCount?: number;
  preview?: ImportPreviewDiff;
  message?: string;
}

export const registryApi = {
  getSystemMode: async (): Promise<SystemModeInfo> => {
    return await apiFetch<SystemModeInfo>('/system/mode');
  },

  listTopologies: async (): Promise<TopologyInfo[]> => {
    return await apiFetch<TopologyInfo[]>('/topology/list');
  },

  setSystemMode: async (demoMode: boolean): Promise<SystemModeInfo> => {
    return await apiFetch<SystemModeInfo>('/system/mode', {
      method: 'POST',
      body: JSON.stringify({ demoMode }),
    });
  },

  listServices: async (namespace?: string, clusterId?: string, tier?: string): Promise<TopologyServiceItem[]> => {
    const params = new URLSearchParams();
    if (namespace) params.set('namespace', namespace);
    if (clusterId) params.set('clusterId', clusterId);
    if (tier) params.set('tier', tier);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return await apiFetch<TopologyServiceItem[]>(`/registry/services${qs}`);
  },

  getService: async (id: string): Promise<TopologyServiceItem> => {
    return await apiFetch<TopologyServiceItem>(`/registry/services/${encodeURIComponent(id)}`);
  },

  createService: async (svc: TopologyServiceItem): Promise<TopologyServiceItem> => {
    return await apiFetch<TopologyServiceItem>('/registry/services', {
      method: 'POST',
      body: JSON.stringify(svc),
    });
  },

  updateService: async (id: string, svc: Partial<TopologyServiceItem>): Promise<TopologyServiceItem> => {
    return await apiFetch<TopologyServiceItem>(`/registry/services/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(svc),
    });
  },

  deleteService: async (id: string): Promise<{ success: boolean; message: string }> => {
    return await apiFetch<{ success: boolean; message: string }>(`/registry/services/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },

  testConnection: async (id: string, sourceOverride?: TelemetrySource): Promise<TestConnectionResponse> => {
    return await apiFetch<TestConnectionResponse>(`/registry/services/${encodeURIComponent(id)}/test-connection`, {
      method: 'POST',
      body: JSON.stringify(sourceOverride ? { telemetrySource: sourceOverride } : {}),
    });
  },

  listEdges: async (namespace?: string, clusterId?: string): Promise<TopologyEdgeItem[]> => {
    const params = new URLSearchParams();
    if (namespace) params.set('namespace', namespace);
    if (clusterId) params.set('clusterId', clusterId);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return await apiFetch<TopologyEdgeItem[]>(`/registry/edges${qs}`);
  },

  createEdge: async (edge: TopologyEdgeItem): Promise<TopologyEdgeItem> => {
    return await apiFetch<TopologyEdgeItem>('/registry/edges', {
      method: 'POST',
      body: JSON.stringify(edge),
    });
  },

  deleteEdge: async (source: string, target: string): Promise<{ success: boolean; message: string }> => {
    return await apiFetch<{ success: boolean; message: string }>(`/registry/edges/${encodeURIComponent(source)}/${encodeURIComponent(target)}`, {
      method: 'DELETE',
    });
  },

  listSuggestions: async (): Promise<TopologyEdgeItem[]> => {
    return await apiFetch<TopologyEdgeItem[]>('/registry/suggestions');
  },

  acceptSuggestion: async (id: string): Promise<{ success: boolean; message: string; edge: TopologyEdgeItem }> => {
    return await apiFetch<{ success: boolean; message: string; edge: TopologyEdgeItem }>(`/registry/suggestions/${encodeURIComponent(id)}/accept`, {
      method: 'POST',
    });
  },

  rejectSuggestion: async (id: string): Promise<{ success: boolean; message: string }> => {
    return await apiFetch<{ success: boolean; message: string }>(`/registry/suggestions/${encodeURIComponent(id)}/reject`, {
      method: 'POST',
    });
  },

  validateTopology: async (payload?: { services?: TopologyServiceItem[]; edges?: TopologyEdgeItem[] }): Promise<ValidationResponse> => {
    return await apiFetch<ValidationResponse>('/registry/validate', {
      method: 'POST',
      body: JSON.stringify(payload || {}),
    });
  },

  listVersions: async (): Promise<TopologyVersionItem[]> => {
    return await apiFetch<TopologyVersionItem[]>('/registry/versions');
  },

  rollbackVersion: async (version: number): Promise<{ success: boolean; message: string; version: number }> => {
    return await apiFetch<{ success: boolean; message: string; version: number }>('/registry/rollback', {
      method: 'POST',
      body: JSON.stringify({ version }),
    });
  },

  importTopology: async (payload: any, dryRun = false): Promise<ImportResponse> => {
    const qs = dryRun ? '?dryRun=true' : '';
    return await apiFetch<ImportResponse>(`/registry/import${qs}`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  exportTopology: async (format: 'yaml' | 'json' | 'csv' = 'yaml'): Promise<string> => {
    const token = localStorage.getItem('aegis_auth_token');
    const headers: Record<string, string> = {};
    if (token) headers['Authorization'] = `Bearer ${token}`;

    const res = await fetch(`${API_BASE_URL}/registry/export?format=${format}`, { headers });
    if (!res.ok) {
      throw new Error(`Export failed: HTTP ${res.status}`);
    }
    return await res.text();
  },
};
