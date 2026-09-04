import { apiFetch } from './client';
import type { ChaosInjection, FaultType } from '../types/telemetry';

export const chaosApi = {
  injectFault: (faultType: FaultType, targetServiceId: string, durationSec: number = 60) =>
    apiFetch<ChaosInjection>('/chaos/inject', {
      method: 'POST',
      body: JSON.stringify({ faultType, targetServiceId, durationSec }),
    }),
  stopFault: (id: string) =>
    apiFetch<{ success: boolean }>(`/chaos/stop/${id}`, { method: 'POST' }),
  getActiveFaults: () => apiFetch<ChaosInjection[]>('/chaos/active'),
};
