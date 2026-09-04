// API Client configuration for Cloud Observability Console
const BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api';

export async function apiFetch<T>(endpoint: string, options?: RequestInit): Promise<T> {
  try {
    const res = await fetch(`${BASE_URL}${endpoint}`, {
      headers: {
        'Content-Type': 'application/json',
      },
      ...options,
    });
    if (!res.ok) {
      throw new Error(`HTTP error ${res.status}: ${res.statusText}`);
    }
    return await res.json();
  } catch (error) {
    // Graceful fallback to client simulation when real backend endpoint is offline
    throw error;
  }
}
