import { apiFetch } from './client';
import type { RecoveryAction, RecoveryHistoryItem } from '../types/telemetry';

export const recoveryApi = {
  getRecommendations: () => apiFetch<RecoveryAction[]>('/recovery/recommendations'),
  executeAction: (actionId: string) =>
    apiFetch<{ success: boolean; actionId: string }>('/recovery/execute', {
      method: 'POST',
      body: JSON.stringify({ actionId }),
    }),
  getHistory: () => apiFetch<RecoveryHistoryItem[]>('/recovery/history'),
};
