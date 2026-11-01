import { apiFetch } from './client';
import type { PredictionOutput } from '../types/telemetry';

export interface AppService {
  id: string;
  name: string;
  base_url?: string;
  host?: string;
  environment?: string;
  depends_on?: string[];
  health_url?: string;
  region?: string;
  provider?: string;
  mode: 'observe' | 'recommend' | 'execute' | 'auto_low';
  allowed_actions: string[];
}

export interface Application {
  id: string;
  name: string;
  services: AppService[];
  createdAt: string;
}

export const appsApi = {
  list: () => apiFetch<Application[]>('/apps'),
  get: (id: string) => apiFetch<Application>(`/apps/${encodeURIComponent(id)}`),
  getServicePrediction: (appId: string, serviceId: string) =>
    apiFetch<PredictionOutput>(`/prediction/latest?app_id=${encodeURIComponent(appId)}&service_id=${encodeURIComponent(serviceId)}`),
  create: (payload: { name: string; services: AppService[]; topology_yaml?: string }) =>
    apiFetch<{ application: Application; api_key: string; warning: string }>('/apps', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updatePermissions: (appId: string, serviceId: string, mode: AppService['mode'], allowed_actions: string[]) =>
    apiFetch<AppService>(`/apps/${encodeURIComponent(appId)}/services/${encodeURIComponent(serviceId)}/permissions`, {
      method: 'PUT',
      body: JSON.stringify({ mode, allowed_actions }),
    }),
};
