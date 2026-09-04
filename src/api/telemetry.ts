import { apiFetch } from './client';
import type { Microservice, ServiceMetric, LiveAlert } from '../types/telemetry';

export const telemetryApi = {
  getServices: () => apiFetch<Microservice[]>('/telemetry/services'),
  getServiceById: (id: string) => apiFetch<Microservice>(`/telemetry/services/${id}`),
  getMetrics: (id: string, rangeMinutes: number = 10) => 
    apiFetch<ServiceMetric[]>(`/telemetry/services/${id}/metrics?range=${rangeMinutes}`),
  getAlerts: () => apiFetch<LiveAlert[]>('/telemetry/alerts'),
};
