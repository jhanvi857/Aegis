import { apiFetch } from './client';
import type { RecoveryAction, RecoveryHistoryItem } from '../types/telemetry';

export const recoveryApi = {
  getRecommendations: () => apiFetch<RecoveryAction[]>('/recovery/recommendations'),
  getPending: () => apiFetch<RecoveryAction[]>('/recovery/pending'),
  approveAction: (actionId: string, targetServiceId?: string) =>
    apiFetch<{ approved: boolean; actionId: string; status: string }>('/recovery/approve', {
      method: 'POST',
      body: JSON.stringify({ actionId, targetServiceId }),
    }),
  rejectAction: (actionId: string, targetServiceId?: string) =>
    apiFetch<{ rejected: boolean; actionId: string; status: string }>('/recovery/reject', {
      method: 'POST',
      body: JSON.stringify({ actionId, targetServiceId }),
    }),
  executeAction: (actionType: string, targetServiceId?: string, actionTitleOrId?: string, approved?: boolean) =>
    apiFetch<{ success: boolean; actionId: string; approvalType?: string }>('/recovery/execute', {
      method: 'POST',
      body: JSON.stringify({ actionType, targetServiceId, actionId: actionTitleOrId, approved }),
    }),
  getHistory: () => apiFetch<RecoveryHistoryItem[]>('/recovery/history'),
};
