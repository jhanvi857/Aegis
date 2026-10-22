import { apiFetch } from './client';

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
  status?: string;
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
  confidence?: number;
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

export const registryApi = {
  getSystemMode: async (): Promise<SystemModeInfo> => {
    return await apiFetch<SystemModeInfo>('/system/mode');
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

  listVersions: async (): Promise<TopologyVersionItem[]> => {
    return await apiFetch<TopologyVersionItem[]>('/registry/versions');
  },

  importTopology: async (payload: {
    systemName?: string;
    description?: string;
    namespace?: string;
    clusterId?: string;
    services?: TopologyServiceItem[];
    nodes?: any[];
    edges?: TopologyEdgeItem[];
  }): Promise<{ success: boolean; version: number; servicesCount: number; edgesCount: number }> => {
    return await apiFetch('/registry/import', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
};
