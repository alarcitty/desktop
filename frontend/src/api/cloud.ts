import { getWailsMilaBridge } from './bridge';

export interface CloudPingResult {
  ok: boolean;
  service: string;
  version: string;
  uptime_sec: number;
  time: string;
}

export interface CloudUser {
  id: number;
  public_id?: string;
  username?: string;
  email: string;
  name: string;
  avatar_url: string;
  created_at?: number;
  last_login_at?: number;
}

export interface CloudSession {
  user: CloudUser;
  expires_at: number;
}

interface DataEnvelope<T> {
  data?: T;
  error?: string;
}

function unwrapEnvelope<T>(raw: string): T {
  const parsed = JSON.parse(raw) as DataEnvelope<T>;
  if (parsed.error) {
    throw new Error(parsed.error);
  }
  return parsed.data as T;
}

async function request<T>(method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE', path: string, body?: unknown): Promise<T> {
  const bridge = getWailsMilaBridge();
  if (!bridge?.CloudRequest) {
    throw new Error('Cloud controller bridge is unavailable');
  }
  const payload = body === undefined ? '' : JSON.stringify(body);
  const raw = await bridge.CloudRequest(method, path, payload);
  return unwrapEnvelope<T>(raw);
}

export const cloudApi = {
  async ping(): Promise<CloudPingResult> {
    const bridge = getWailsMilaBridge();
    if (!bridge?.CloudPing) {
      throw new Error('Cloud controller bridge is unavailable');
    }
    const raw = await bridge.CloudPing();
    return unwrapEnvelope<CloudPingResult>(raw);
  },

  get<T>(path: string): Promise<T> {
    return request<T>('GET', path);
  },

  post<T>(path: string, body?: unknown): Promise<T> {
    return request<T>('POST', path, body);
  },

  put<T>(path: string, body?: unknown): Promise<T> {
    return request<T>('PUT', path, body);
  },

  patch<T>(path: string, body?: unknown): Promise<T> {
    return request<T>('PATCH', path, body);
  },

  del<T>(path: string): Promise<T> {
    return request<T>('DELETE', path);
  },

  async loginWithGoogle(): Promise<CloudSession> {
    const bridge = getWailsMilaBridge();
    if (!bridge?.CloudLoginWithGoogle) {
      throw new Error('Cloud login bridge is unavailable');
    }
    const raw = await bridge.CloudLoginWithGoogle();
    return unwrapEnvelope<CloudSession>(raw);
  },

  async getSession(): Promise<CloudSession | null> {
    const bridge = getWailsMilaBridge();
    if (!bridge?.CloudGetSession) {
      return null;
    }
    const raw = await bridge.CloudGetSession();
    return unwrapEnvelope<CloudSession | null>(raw);
  },

  async logout(): Promise<void> {
    const bridge = getWailsMilaBridge();
    if (!bridge?.CloudLogout) {
      return;
    }
    const raw = await bridge.CloudLogout();
    unwrapEnvelope<{ ok: boolean }>(raw);
  }
};
