export interface SkipRange {
  start?: number | null;
  stop?: number | null;
}

export interface HlsSources {
  hls_1080?: string | null;
  hls_720?: string | null;
  hls_480?: string | null;
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

export interface VoiceTrackOption {
  id: string;
  label: string;
  sublabel?: string;
  releaseAlias?: string;
}

export interface AudioTrackOption {
  index: number;
  label: string;
  lang?: string;
}
