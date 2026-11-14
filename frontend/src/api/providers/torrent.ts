export interface TorrentSearchItem {
  source: 'nyaa' | 'anilibria' | string;
  title: string;
  infoHash: string;
  magnet: string;
  size: string;
  seeders: number;
  leechers: number;
  quality?: string;
  category?: string;
}

export interface TorrentFileItem {
  fileIndex: number;
  ordinal: number;
  name: string;
  path: string;
  length: number;
  needsRemux: boolean;
  contentType: string;
}

export interface TorrentReleaseInfo {
  infoHash: string;
  name: string;
  files: TorrentFileItem[];
}

export interface MediaAudioTrack {
  ordinal: number;
  streamIndex: number;
  codec: string;
  language?: string;
  title?: string;
  label: string;
  channels?: number;
}

export interface MediaSubtitleTrack {
  ordinal: number;
  streamIndex: number;
  codec: string;
  language?: string;
  title?: string;
  label: string;
}

export interface MediaProbeResult {
  duration: number;
  videoCodec?: string;
  width?: number;
  height?: number;
  audioTracks: MediaAudioTrack[];
  subtitles: MediaSubtitleTrack[];
}

export interface FFmpegStatus {
  available: boolean;
  downloading: boolean;
  progressPct: number;
  stage: string;
  error?: string;
}

interface WailsTorrentBindings {
  TorrentSearch?: (query: string, aniLibriaID: number) => Promise<string>;
  TorrentOpen?: (magnet: string) => Promise<string>;
  TorrentProbeFile?: (infoHash: string, fileIndex: number) => Promise<string>;
  GetFFmpegStatus?: () => Promise<string>;
}

function getWailsTorrentBridge(): WailsTorrentBindings | null {
  if (typeof window === 'undefined') return null;
  const w = window as unknown as {
    go?: { mila?: { Mila?: WailsTorrentBindings } };
  };
  return w.go?.mila?.Mila ?? null;
}

export async function searchTorrents(
  query: string,
  aniLibriaId = 0
): Promise<TorrentSearchItem[]> {
  const bridge = getWailsTorrentBridge();
  if (bridge?.TorrentSearch) {
    try {
      const raw = await bridge.TorrentSearch(query, aniLibriaId);
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) return parsed as TorrentSearchItem[];
    } catch {
      // Fall back to HTTP endpoint
    }
  }
  const params = new URLSearchParams();
  if (query.trim()) params.set('q', query.trim());
  if (aniLibriaId > 0) params.set('lib', String(aniLibriaId));
  const res = await fetch(`/torrent/search?${params.toString()}`);
  if (!res.ok) throw new Error(`Torrent search HTTP ${res.status}`);
  return (await res.json()) as TorrentSearchItem[];
}

export async function openTorrentMagnet(magnet: string): Promise<TorrentReleaseInfo> {
  const bridge = getWailsTorrentBridge();
  if (bridge?.TorrentOpen) {
    const raw = await bridge.TorrentOpen(magnet);
    const parsed = JSON.parse(raw) as TorrentReleaseInfo;
    if (parsed?.infoHash) return parsed;
  }
  const res = await fetch(`/torrent/open?magnet=${encodeURIComponent(magnet)}`);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()) as TorrentReleaseInfo;
}

export async function probeTorrentFile(
  infoHash: string,
  fileIndex: number
): Promise<MediaProbeResult> {
  const bridge = getWailsTorrentBridge();
  if (bridge?.TorrentProbeFile) {
    const raw = await bridge.TorrentProbeFile(infoHash, fileIndex);
    const parsed = JSON.parse(raw) as MediaProbeResult;
    if (parsed) return parsed;
  }
  const res = await fetch(
    `/media/probe?hash=${encodeURIComponent(infoHash)}&file=${encodeURIComponent(String(fileIndex))}`
  );
  if (!res.ok) throw new Error(`Probe HTTP ${res.status}`);
  return (await res.json()) as MediaProbeResult;
}

export async function getFFmpegStatus(): Promise<FFmpegStatus> {
  const bridge = getWailsTorrentBridge();
  if (bridge?.GetFFmpegStatus) {
    try {
      const raw = await bridge.GetFFmpegStatus();
      return JSON.parse(raw) as FFmpegStatus;
    } catch {
      // Fallback
    }
  }
  const res = await fetch('/media/ffmpeg-status');
  if (!res.ok) {
    return { available: false, downloading: false, progressPct: 0, stage: 'idle' };
  }
  return (await res.json()) as FFmpegStatus;
}

export function buildTorrentMediaUrl(
  hash: string,
  fileIndex: number,
  needsRemux: boolean,
  audioOrdinal = 0,
  startSec = 0
): string {
  if (!needsRemux) {
    return `/torrent/stream/${encodeURIComponent(hash)}/${encodeURIComponent(String(fileIndex))}`;
  }
  const params = new URLSearchParams();
  params.set('hash', hash);
  params.set('file', String(fileIndex));
  params.set('a', String(Math.max(0, audioOrdinal)));
  if (startSec > 0.25) {
    params.set('t', startSec.toFixed(2));
  }
  return `/media/stream.mp4?${params.toString()}`;
}

export function buildTorrentSubtitleUrl(
  hash: string,
  fileIndex: number,
  subOrdinal: number
): string {
  const params = new URLSearchParams();
  params.set('hash', hash);
  params.set('file', String(fileIndex));
  params.set('s', String(Math.max(0, subOrdinal)));
  return `/media/subtitles.vtt?${params.toString()}`;
}
