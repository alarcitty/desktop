import { cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

export interface RevealBlurParams {
  duration?: number;
  delay?: number;
  x?: number;
  y?: number;
  scale?: number;
  blur?: number;
}

export function revealBlur(
  _node: Element,
  params: RevealBlurParams = {}
): TransitionConfig {
  const duration = params.duration ?? 240;
  const delay = params.delay ?? 0;
  const x = params.x ?? 0;
  const y = params.y ?? 0;
  const scale = params.scale ?? 1;
  const blur = params.blur ?? 6;
  return {
    duration,
    delay,
    easing: cubicOut,
    css: (t: number) => {
      const inv = 1 - t;
      const curX = inv * x;
      const curY = inv * y;
      const curScale = scale + (1 - scale) * t;
      const curBlur = inv * blur;
      return `opacity: ${t}; transform: translate3d(${curX}px, ${curY}px, 0) scale(${curScale}); filter: blur(${curBlur}px);`;
    }
  };
}
