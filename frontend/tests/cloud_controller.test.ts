import { afterEach, describe, expect, it, vi } from 'vitest';
import { cloudApi } from '../src/api/cloud';

afterEach(() => {
  delete (globalThis as { window?: unknown }).window;
});

describe('cloudApi controller', () => {
  it('calls Wails CloudPing and unwraps {data} envelope', async () => {
    const cloudPing = vi.fn().mockResolvedValue(
      JSON.stringify({
        data: {
          ok: true,
          service: 'mila-cloud',
          version: '0.1.0',
          uptime_sec: 42,
          time: '2026-11-15T18:50:00Z'
        }
      })
    );
    (globalThis as { window?: unknown }).window = {
      go: { mila: { Mila: { CloudPing: cloudPing } } }
    };

    const res = await cloudApi.ping();
    expect(cloudPing).toHaveBeenCalledTimes(1);
    expect(res).toEqual({
      ok: true,
      service: 'mila-cloud',
      version: '0.1.0',
      uptime_sec: 42,
      time: '2026-11-15T18:50:00Z'
    });
  });

  it('forwards GET and POST requests through CloudRequest', async () => {
    const cloudRequest = vi.fn().mockResolvedValue(JSON.stringify({ data: { created: true } }));
    (globalThis as { window?: unknown }).window = {
      go: { mila: { Mila: { CloudRequest: cloudRequest } } }
    };

    const out = await cloudApi.post<{ created: boolean }>('/api/v1/rooms', { title: 'Watch Party' });
    expect(cloudRequest).toHaveBeenCalledWith('POST', '/api/v1/rooms', JSON.stringify({ title: 'Watch Party' }));
    expect(out.created).toBe(true);
  });

  it('throws cleanly when Wails bridge is missing or returns an error envelope', async () => {
    await expect(cloudApi.ping()).rejects.toThrow('Cloud controller bridge is unavailable');

    (globalThis as { window?: unknown }).window = {
      go: { mila: { Mila: { CloudRequest: vi.fn().mockResolvedValue(JSON.stringify({ error: 'cloud request failed' })) } } }
    };
    await expect(cloudApi.get('/api/v1/rooms')).rejects.toThrow('cloud request failed');
  });

  it('handles CloudLoginWithGoogle, CloudGetSession and CloudLogout envelopes', async () => {
    const sampleSession = {
      expires_at: 1799999999,
      user: {
        id: 1,
        email: 'mila@gmail.com',
        name: 'Mila',
        avatar_url: 'https://lh3.googleusercontent.com/a/avatar'
      }
    };
    const CloudLoginWithGoogle = vi.fn().mockResolvedValue(JSON.stringify({ data: sampleSession }));
    const CloudGetSession = vi.fn().mockResolvedValue(JSON.stringify({ data: sampleSession }));
    const CloudLogout = vi.fn().mockResolvedValue(JSON.stringify({ data: { ok: true } }));

    (globalThis as { window?: unknown }).window = {
      go: { mila: { Mila: { CloudLoginWithGoogle, CloudGetSession, CloudLogout } } }
    };

    const logged = await cloudApi.loginWithGoogle();
    expect(CloudLoginWithGoogle).toHaveBeenCalledTimes(1);
    expect(logged.user.email).toBe('mila@gmail.com');

    const loaded = await cloudApi.getSession();
    expect(CloudGetSession).toHaveBeenCalledTimes(1);
    expect(loaded?.user.id).toBe(1);

    await cloudApi.logout();
    expect(CloudLogout).toHaveBeenCalledTimes(1);
  });
});
