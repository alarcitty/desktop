import fs from 'node:fs';
import path from 'node:path';
import { performance } from 'node:perf_hooks';
import { parseAst } from 'rollup/parseAst';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLazyModuleLoader } from '../src/api/providers/lazy';
import { getLoadedHls, loadHls, resetHlsLoaderForTest } from '../src/app/hlsLoader';
import {
  extractSeasonNumber,
  fetchJsonWithTimeout,
  normalizeTitleText,
  scoreTitleSimilarity,
  titleTokens
} from '../src/api/providers/shared';

const FRONTEND_ROOT = path.resolve(import.meta.dirname, '..');
const SRC_ASSETS_DIR = path.join(FRONTEND_ROOT, 'src', 'assets');
const DIST_ASSETS_DIR = path.join(FRONTEND_ROOT, 'dist', 'assets');

function walkFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...walkFiles(full));
    } else {
      out.push(full);
    }
  }
  return out;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  resetHlsLoaderForTest();
});

describe('createLazyModuleLoader & hlsLoader', () => {
  it('deduplicates concurrent loads and caches synchronously afterward', async () => {
    let calls = 0;
    const loader = createLazyModuleLoader(async () => {
      calls += 1;
      await Promise.resolve();
      return { ok: true, token: 42 };
    });

    expect(loader.getLoaded()).toBeNull();
    const [a, b, c] = await Promise.all([loader.load(), loader.load(), loader.load()]);
    expect(calls).toBe(1);
    expect(a).toBe(b);
    expect(b).toBe(c);
    expect(loader.getLoaded()).toEqual({ ok: true, token: 42 });

    const d = await loader.load();
    expect(d).toBe(a);
    expect(calls).toBe(1);
  });

  it('clears pending state on failure so a subsequent load retries cleanly', async () => {
    let attempt = 0;
    const loader = createLazyModuleLoader(async () => {
      attempt += 1;
      if (attempt === 1) {
        throw new Error('chunk fetch failed');
      }
      return { recovered: true };
    });

    await expect(loader.load()).rejects.toThrow('chunk fetch failed');
    expect(loader.getLoaded()).toBeNull();

    const recovered = await loader.load();
    expect(recovered).toEqual({ recovered: true });
    expect(loader.getLoaded()).toEqual({ recovered: true });
    expect(attempt).toBe(2);
  });

  it('loads hls.js lazily, deduplicates concurrent requests, and resets cleanly', async () => {
    expect(getLoadedHls()).toBeNull();
    const [HlsA, HlsB] = await Promise.all([loadHls(), loadHls()]);
    expect(HlsA).toBe(HlsB);
    expect(typeof HlsA.isSupported).toBe('function');
    expect(getLoadedHls()).toBe(HlsA);

    resetHlsLoaderForTest();
    expect(getLoadedHls()).toBeNull();
  });
});

describe('provider shared helpers edge cases (abort, 429, empty inputs)', () => {
  it('propagates HTTP 429 errors without hanging', async () => {
    vi.stubGlobal('window', globalThis);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('rate limited', { status: 429 }))
    );

    await expect(fetchJsonWithTimeout('https://example.test/api', undefined, 500)).rejects.toThrow(
      'HTTP 429'
    );
  });

  it('cancels promptly via external AbortSignal', async () => {
    vi.stubGlobal('window', globalThis);
    const controller = new AbortController();
    controller.abort();

    const fetchMock = vi.fn((_input: unknown, init?: RequestInit) => {
      expect(init?.signal?.aborted).toBe(true);
      return Promise.reject(new DOMException('Aborted', 'AbortError'));
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(
      fetchJsonWithTimeout('https://example.test/api', { signal: controller.signal }, 5000)
    ).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('handles empty, nullish, and malformed titles safely', () => {
    expect(normalizeTitleText('')).toBe('');
    expect(titleTokens('   ')).toEqual([]);
    expect(extractSeasonNumber([])).toBe(1);
    expect(scoreTitleSimilarity([], ['Solo Leveling'], 2024, 2024)).toBe(-1);
    expect(scoreTitleSimilarity(['Solo Leveling'], [], 2024, 2024)).toBe(-1);
    expect(scoreTitleSimilarity(['Solo Leveling'], ['Solo Leveling'], 2024, 2024)).toBeGreaterThan(0);
    expect(scoreTitleSimilarity(['Solo Leveling Season 2'], ['Solo Leveling'], 2024, 2024)).toBe(-1);
    expect(scoreTitleSimilarity(['Solo Leveling OVA'], ['Solo Leveling'], 2024, 2024)).toBe(-1);
  });
});

describe('T-115 asset conversion and code-split verification', () => {
  it('has zero leftover PNGs in src/assets and valid WebP headers on all converted assets', () => {
    const files = walkFiles(SRC_ASSETS_DIR);
    const pngs = files.filter((f) => f.toLowerCase().endsWith('.png'));
    const webps = files.filter((f) => f.toLowerCase().endsWith('.webp'));
    const ttfs = files.filter((f) => f.toLowerCase().endsWith('.ttf'));

    expect(pngs).toEqual([]);
    expect(webps.length).toBe(13);
    expect(ttfs.length).toBe(6);
    expect(fs.existsSync(path.join(SRC_ASSETS_DIR, 'mila_header.svg'))).toBe(true);

    for (const webpPath of webps) {
      const buf = fs.readFileSync(webpPath);
      expect(buf.byteLength).toBeGreaterThan(1024);
      expect(buf.subarray(0, 4).toString('ascii')).toBe('RIFF');
      expect(buf.subarray(8, 12).toString('ascii')).toBe('WEBP');
    }
  });

  it('splits hls.js, AnimePage, and heavy providers out of the cold-start entry chunk', () => {
    expect(fs.existsSync(DIST_ASSETS_DIR)).toBe(true);
    const assets = fs.readdirSync(DIST_ASSETS_DIR);
    const jsFiles = assets.filter((f) => f.endsWith('.js'));

    const entryChunk = jsFiles.find((f) => f.startsWith('index-'));
    expect(entryChunk).toBeDefined();

    const entrySize = fs.statSync(path.join(DIST_ASSETS_DIR, entryChunk!)).size;
    // Before T-115 baseline was 981,658 bytes (~981.66 kB). Verify meaningful reduction.
    expect(entrySize).toBeLessThan(650_000);

    // Separate deferred chunks must exist for hls.js, AnimePage, and heavy provider modules.
    expect(jsFiles.some((f) => f.startsWith('hls'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('AnimePage-'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('animego-'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('animevost-'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('dreamcast-'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('kodik-'))).toBe(true);
    expect(jsFiles.some((f) => f.startsWith('torrent-'))).toBe(true);

    // Cold-start ESM AST parse budget check (<< 600ms) and ensure heavy chunks are deferred.
    const entrySource = fs.readFileSync(path.join(DIST_ASSETS_DIR, entryChunk!), 'utf8');
    const t0 = performance.now();
    parseAst(entrySource);
    const parseMs = performance.now() - t0;
    expect(parseMs).toBeLessThan(600);
  });
});
