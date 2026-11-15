export interface BufferedRangesLike {
  length: number;
  start(index: number): number;
  end(index: number): number;
}

export interface HeroBufferReadinessInput {
  currentTime: number;
  targetStartSec: number;
  targetEndSec: number;
  readyState: number;
  buffered: BufferedRangesLike;
  canPlayThrough: boolean;
}

/**
 * Returns how many seconds are continuously buffered ahead of `positionSec`.
 */
export function getBufferedAheadSec(buffered: BufferedRangesLike, positionSec: number): number {
  if (!buffered || buffered.length === 0) return 0;
  const pos = Number.isFinite(positionSec) ? Math.max(0, positionSec) : 0;
  for (let i = 0; i < buffered.length; i += 1) {
    const start = buffered.start(i);
    const end = buffered.end(i);
    // Allow minor keyframe alignment offset up to 0.25s around target start position.
    if (pos >= start - 0.25 && pos <= end) {
      return Math.max(0, end - pos);
    }
  }
  return 0;
}

/**
 * Target forward buffer required before fading in the hero opening preview
 * so playback never freezes mid-clip right after revealing.
 */
export function getHeroMinBufferAheadSec(startSec: number, endSec: number): number {
  const clipLen = endSec - startSec;
  if (!Number.isFinite(clipLen) || clipLen <= 0) return 2.2;
  return Math.min(4.2, Math.max(1.8, clipLen * 0.25));
}

/**
 * Returns true once the video has reached its opening start offset and has
 * sufficient forward buffer (or HAVE_ENOUGH_DATA) for smooth playback.
 */
export function hasEnoughHeroPreviewBuffer(input: HeroBufferReadinessInput): boolean {
  const targetStart = Math.max(0, input.targetStartSec);
  if (!Number.isFinite(input.currentTime) || input.currentTime < targetStart - 0.35) {
    return false;
  }
  if (input.canPlayThrough || input.readyState >= 4) {
    return true;
  }
  if (input.readyState < 3) {
    return false;
  }
  const ahead = getBufferedAheadSec(
    input.buffered,
    Math.max(input.currentTime, targetStart)
  );
  return ahead >= getHeroMinBufferAheadSec(targetStart, input.targetEndSec);
}

/**
 * Orders neighbor carousel indices (+1, -1, +2, ...) for progressive warming.
 */
export function computeAdjacentHeroIndices(activeIndex: number, total: number): number[] {
  if (total <= 1) return [];
  const safeActive = ((activeIndex % total) + total) % total;
  const out: number[] = [];
  const seen = new Set<number>([safeActive]);
  for (let dist = 1; dist < total; dist += 1) {
    const nextIdx = (safeActive + dist) % total;
    if (!seen.has(nextIdx)) {
      seen.add(nextIdx);
      out.push(nextIdx);
    }
    const prevIdx = (safeActive - dist + total) % total;
    if (!seen.has(prevIdx)) {
      seen.add(prevIdx);
      out.push(prevIdx);
    }
  }
  return out;
}
