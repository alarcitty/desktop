import { describe, expect, it } from 'vitest';
import {
  computeAdjacentHeroIndices,
  getBufferedAheadSec,
  getHeroMinBufferAheadSec,
  hasEnoughHeroPreviewBuffer,
  type BufferedRangesLike
} from '../src/pages/home/hero/heroBuffer';

function makeBuffered(ranges: Array<[number, number]>): BufferedRangesLike {
  return {
    length: ranges.length,
    start: (i) => ranges[i][0],
    end: (i) => ranges[i][1]
  };
}

describe('Hero opening preview buffer & neighbor warming helpers', () => {
  it('computes continuous buffered seconds ahead of position including minor keyframe offset', () => {
    const buf = makeBuffered([
      [0, 6],
      [40.1, 46.5]
    ]);
    expect(getBufferedAheadSec(buf, 40)).toBeCloseTo(6.5, 2);
    expect(getBufferedAheadSec(buf, 44)).toBeCloseTo(2.5, 2);
    expect(getBufferedAheadSec(buf, 20)).toBe(0);
    expect(getBufferedAheadSec(makeBuffered([]), 10)).toBe(0);
  });

  it('requires seek to reach startSec and sufficient forward buffer before revealing', () => {
    // Still at 0 while target starts at 35s
    expect(
      hasEnoughHeroPreviewBuffer({
        currentTime: 0,
        targetStartSec: 35,
        targetEndSec: 65,
        readyState: 4,
        buffered: makeBuffered([[0, 10]]),
        canPlayThrough: true
      })
    ).toBe(false);

    // At 35s, only 1.1s buffered and readyState 3 -> must NOT reveal yet (prevents mid-clip stall)
    expect(
      hasEnoughHeroPreviewBuffer({
        currentTime: 35,
        targetStartSec: 35,
        targetEndSec: 65,
        readyState: 3,
        buffered: makeBuffered([[34.8, 36.1]]),
        canPlayThrough: false
      })
    ).toBe(false);

    // At 35s, 4.5s buffered ahead -> ready to reveal cleanly
    expect(
      hasEnoughHeroPreviewBuffer({
        currentTime: 35,
        targetStartSec: 35,
        targetEndSec: 65,
        readyState: 3,
        buffered: makeBuffered([[34.8, 39.5]]),
        canPlayThrough: false
      })
    ).toBe(true);

    // canPlayThrough / readyState 4 at target position -> ready
    expect(
      hasEnoughHeroPreviewBuffer({
        currentTime: 35,
        targetStartSec: 35,
        targetEndSec: 65,
        readyState: 4,
        buffered: makeBuffered([[34.8, 36.0]]),
        canPlayThrough: false
      })
    ).toBe(true);
  });

  it('scales minimum buffer threshold cleanly for short and malformed clips', () => {
    expect(getHeroMinBufferAheadSec(10, 40)).toBeCloseTo(4.2, 2);
    expect(getHeroMinBufferAheadSec(10, 14)).toBeCloseTo(1.8, 2);
    expect(getHeroMinBufferAheadSec(10, 10)).toBe(2.2);
  });

  it('orders adjacent spotlight slides cleanly without duplicating active index', () => {
    expect(computeAdjacentHeroIndices(0, 5)).toEqual([1, 4, 2, 3]);
    expect(computeAdjacentHeroIndices(2, 5)).toEqual([3, 1, 4, 0]);
    expect(computeAdjacentHeroIndices(0, 1)).toEqual([]);
    expect(computeAdjacentHeroIndices(0, 0)).toEqual([]);
  });
});
