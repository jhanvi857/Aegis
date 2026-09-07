import { apiFetch } from './client';
import type { PredictionOutput } from '../types/telemetry';

export const predictionApi = {
  getLatestPrediction: () => apiFetch<PredictionOutput>('/prediction/latest'),
  getPredictionHistory: () => apiFetch<PredictionOutput[]>('/prediction/history'),
};
