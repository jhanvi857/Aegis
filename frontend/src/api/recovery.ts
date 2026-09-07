import { apiFetch } from './client';
import type { RecoveryAction, RecoveryHistoryItem } from '../types/telemetry';

export const recoveryApi = {
  getRecommendations: () => apiFetch<RecoveryAction[]>('/recovery/recommendations'),
  executeAction: (actionType: string, targetServiceId?: string, actionTitleOrId?: string) =>
    apiFetch<{ success: boolean; actionId: string }>('/recovery/execute', {
      method: 'POST',
      body: JSON.stringify({ actionType, targetServiceId, actionId: actionTitleOrId }),
    }),
  getHistory: () => apiFetch<RecoveryHistoryItem[]>('/recovery/history'),
};
