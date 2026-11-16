export interface SkipRange {
  start?: number | null;
  stop?: number | null;
}

export interface TorrentRemuxConfig {
  hash: string;
  fileIndex: number;
  needsRemux: boolean;
  label?: string;
}

export interface HlsSources {
  hls_1080?: string | null;
  hls_720?: string | null;
  hls_480?: string | null;
  mp4_1080?: string | null;
  mp4_720?: string | null;
  mp4_480?: string | null;
  remux?: TorrentRemuxConfig | null;
}

export interface QualityVariant {
  label: string;
  height: number;
  width: number;
  bandwidth: number;
  url: string;
}

export interface QualityLevelOption {
  index: number; // -1 for Auto ABR
  label: string;
  height: number;
  bitrate: number;
}

export type VoiceProviderId =
  | 'aniliberty'
  | 'dreamcast'
  | 'animevost'
  | 'animego'
  | 'kodik'
  | 'hentaimama'
  | 'hentaigasm'
  | string;

export interface DubAudioSource {
  id: string;
  label: string;
  sources: HlsSources;
}

export interface VoiceTrackOption {
  id: string;
  label: string;
  sublabel?: string;
  releaseAlias?: string;
  provider?: VoiceProviderId;
  providerId?: number | string;
  sourceId?: number | string;
  kind?: 'hls' | 'mp4';
  isSub?: boolean;
}

export interface AudioTrackOption {
  index: number;
  label: string;
  lang?: string;
}

export interface SubtitleTrackOption {
  index: number;
  label: string;
  lang?: string;
  src: string;
}
