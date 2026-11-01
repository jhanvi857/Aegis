export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8000/api';

export class ApiError extends Error {
  status: number;
  statusText: string;
  data?: any;

  constructor(status: number, statusText: string, message: string, data?: any) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.statusText = statusText;
    this.data = data;
  }
}

export async function apiFetch<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('aegis_auth_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers: {
      ...headers,
      ...(options?.headers || {}),
    },
  });
  if (!res.ok) {
    let errorDetail = res.statusText;
    let data: any = null;
    try {
      const text = await res.text();
      try {
        data = JSON.parse(text);
        errorDetail = data.error || data.message || errorDetail;
      } catch {
        if (text && text.trim()) {
          errorDetail = text.trim();
        }
      }
    } catch {
      // Body reading failed
    }
    throw new ApiError(res.status, res.statusText, errorDetail || `HTTP error ${res.status}: ${res.statusText}`, data);
  }
  return await res.json();
}
