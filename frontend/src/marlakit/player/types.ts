export interface SkipRange {
  start?: number | null;
  stop?: number | null;
}

export interface HlsSources {
  hls_1080?: string | null;
  hls_720?: string | null;
  hls_480?: string | null;
  mp4_1080?: string | null;
  mp4_720?: string | null;
  mp4_480?: string | null;
  embed_url?: string | null;
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

export type VoiceProviderId = 'aniliberty' | 'animevost' | 'anixart' | string;

export interface VoiceTrackOption {
  id: string;
  label: string;
  sublabel?: string;
  releaseAlias?: string;
  provider?: VoiceProviderId;
  providerId?: number | string;
  sourceId?: number | string;
  kind?: 'hls' | 'mp4' | 'embed';
  isSub?: boolean;
}

export interface AudioTrackOption {
  index: number;
  label: string;
  lang?: string;
}
