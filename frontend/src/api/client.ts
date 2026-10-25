export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8000/api';

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
    throw new Error(`HTTP error ${res.status}: ${res.statusText}`);
  }
  return await res.json();
}
