import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  callUnifiedContract,
  exportDiagnosticLogs,
  getBackendContractInfo,
  resetBackendApiCacheForTest,
  resolveBackendApiBase
} from '../src/api/bridge';
import {
  createDebouncedStorageWriter,
  flushBeforeClose,
  registerBeforeCloseFlush
} from '../src/app/lifecycle';
import { mountBindingsController } from '../src/app/bindings.svelte';

describe('T-020 close flush hooks & T-089 unified Go<->JS contract', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    resetBackendApiCacheForTest();
  });

  it('executes registered flush handlers synchronously even if one throws', () => {
    const flushed: string[] = [];
    const writer = createDebouncedStorageWriter<string>(1000, (v) => flushed.push(v));
    writer.schedule('ep:12:423.5');

    const unregGood = registerBeforeCloseFlush(() => writer.flush());
    const unregThrowing = registerBeforeCloseFlush(() => {
      throw new Error('storage quota exceeded');
    });

    try {
      expect(() => flushBeforeClose()).not.toThrow();
      expect(flushed).toEqual(['ep:12:423.5']);
      expect(writer.hasPending()).toBe(false);
    } finally {
      unregGood();
      unregThrowing();
    }
  });

  it('triggers close-flush on beforeunload, pagehide, and Wails mila:before-close event', () => {
    const windowListeners = new Map<string, Set<EventListenerOrEventListenerObject>>();
    const docListeners = new Map<string, Set<EventListenerOrEventListenerObject>>();
    const wailsEvents = new Map<string, () => void>();
    const unsubSpy = vi.fn();

    vi.stubGlobal('window', {
      addEventListener: (type: string, cb: EventListenerOrEventListenerObject) => {
        if (!windowListeners.has(type)) windowListeners.set(type, new Set());
        windowListeners.get(type)!.add(cb);
      },
      removeEventListener: (type: string, cb: EventListenerOrEventListenerObject) => {
        windowListeners.get(type)?.delete(cb);
      },
      matchMedia: () => ({ matches: false }),
      runtime: {
        EventsOn: (name: string, cb: () => void) => {
          wailsEvents.set(name, cb);
          return () => {
            wailsEvents.delete(name);
            unsubSpy();
          };
        }
      }
    });

    vi.stubGlobal('document', {
      addEventListener: (type: string, cb: EventListenerOrEventListenerObject) => {
        if (!docListeners.has(type)) docListeners.set(type, new Set());
        docListeners.get(type)!.add(cb);
      },
      removeEventListener: (type: string, cb: EventListenerOrEventListenerObject) => {
        docListeners.get(type)?.delete(cb);
      },
      body: { classList: { contains: () => false, toggle: () => false, remove: () => {} } },
      fullscreenElement: null
    });

    let flushCount = 0;
    const unregisterFlush = registerBeforeCloseFlush(() => {
      flushCount++;
    });

    const cleanup = mountBindingsController();
    try {
      expect(windowListeners.has('beforeunload')).toBe(true);
      expect(windowListeners.has('pagehide')).toBe(true);
      expect(wailsEvents.has('mila:before-close')).toBe(true);

      for (const cb of windowListeners.get('beforeunload') ?? []) {
        if (typeof cb === 'function') cb({} as Event);
      }
      for (const cb of windowListeners.get('pagehide') ?? []) {
        if (typeof cb === 'function') cb({} as Event);
      }
      wailsEvents.get('mila:before-close')?.();

      expect(flushCount).toBe(3);
    } finally {
      cleanup();
      unregisterFlush();
    }

    expect(unsubSpy).toHaveBeenCalledTimes(1);
    expect(windowListeners.get('beforeunload')?.size ?? 0).toBe(0);
    expect(windowListeners.get('pagehide')?.size ?? 0).toBe(0);
  });

  it('resolves same-origin /api/v1 first and falls back to localhost:8080 when proxyless', async () => {
    const sameOriginProbe = vi.fn(async (url: string) => {
      if (url === '/health') return { status: 'ok', mode: 'wails' };
      throw new Error('unexpected probe');
    });
    expect(await resolveBackendApiBase(sameOriginProbe)).toBe('/api/v1');

    resetBackendApiCacheForTest();
    const fallbackProbe = vi.fn(async (url: string) => {
      if (url === '/health') throw new Error('404');
      if (url === 'http://localhost:8080/health') return { status: 'ok', mode: 'http' };
      throw new Error('offline');
    });
    expect(await resolveBackendApiBase(fallbackProbe)).toBe('http://localhost:8080/api/v1');

    resetBackendApiCacheForTest();
    const offlineProbe = vi.fn(async () => {
      throw new Error('offline');
    });
    expect(await resolveBackendApiBase(offlineProbe)).toBeNull();
  });

  it('returns identical Go<->JS contract shapes across Wails IPC and HTTP vite dev modes', async () => {
    const httpResponse = {
      data: {
        status: 'ok',
        mode: 'http',
        apiBase: '/api/v1',
        extBase: '/ext',
        capabilities: ['anime', 'shikimori', 'image', 'diagnostics']
      }
    };

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string) => ({
        ok: true,
        status: 200,
        json: async () =>
          input.includes('/api/v1/diagnostics/logs')
            ? { data: { path: 'http.zip', files: 2, bytes: 512, exportedAt: '2026-11-15T14:00:00Z', redacted: true } }
            : httpResponse
      }))
    );

    // HTTP (vite dev) without window.go.mila.Mila
    vi.stubGlobal('window', {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis)
    });
    const httpContract = await getBackendContractInfo();
    expect(httpContract.mode).toBe('http');
    expect(httpContract.apiBase).toBe('/api/v1');
    const httpExport = await exportDiagnosticLogs('');
    expect(httpExport.redacted).toBe(true);
    expect(httpExport.files).toBe(2);

    // Wails (.exe) with window.go.mila.Mila returning { data } envelope
    const getContractSpy = vi.fn(async () =>
      JSON.stringify({
        data: {
          status: 'ok',
          mode: 'wails',
          apiBase: '/api/v1',
          extBase: '/ext',
          capabilities: ['anime', 'shikimori', 'image', 'providers', 'torrent', 'diagnostics']
        }
      })
    );
    const exportLogsSpy = vi.fn(async () =>
      JSON.stringify({
        data: { path: 'C:/cache/mila-diag.zip', files: 3, bytes: 1024, exportedAt: '2026-11-15T14:00:00Z', redacted: true }
      })
    );

    vi.stubGlobal('window', {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      go: {
        mila: {
          Mila: {
            GetContractInfo: getContractSpy,
            ExportDiagnosticsLogs: exportLogsSpy
          }
        }
      }
    });

    const wailsContract = await getBackendContractInfo();
    expect(getContractSpy).toHaveBeenCalledTimes(1);
    expect(wailsContract.mode).toBe('wails');
    expect(wailsContract.apiBase).toBe('/api/v1');

    const wailsExport = await exportDiagnosticLogs('C:/cache');
    expect(exportLogsSpy).toHaveBeenCalledWith('C:/cache');
    expect(wailsExport.redacted).toBe(true);
    expect(wailsExport.path).toBe('C:/cache/mila-diag.zip');

    // Wails IPC failure transparently falls back to HTTP without breaking caller
    const fallbackResult = await callUnifiedContract<{ ok: boolean }>({
      wailsCall: async () => {
        throw new Error('ipc closed');
      },
      httpPath: '/health'
    });
    expect((fallbackResult as unknown as { status: string }).status).toBe('ok');
  });
});
