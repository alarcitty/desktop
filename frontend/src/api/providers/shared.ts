export const SPECIAL_TITLE_RE =
  /(?:^|[^\p{L}\p{N}])(special(?:s)?|сп[еэ]ш[еэ]?л(?:ы)?|спецвыпуск(?:и)?|ova|ова|ona|она|recap|рекап)(?:$|[^\p{L}\p{N}])/iu;

const TITLE_STOP_WORDS = new Set([
  'the',
  'a',
  'an',
  'of',
  'in',
  'on',
  'to',
  'and',
  'for',
  'no',
  'wa',
  'ga',
  'ni',
  'wo',
  'season',
  'part',
  'tv',
  'сезон',
  'часть'
]);

const RU_ORDINALS: Record<string, number> = {
  первый: 1,
  второй: 2,
  третий: 3,
  четвертый: 4,
  четвёртый: 4,
  пятый: 5,
  шестой: 6,
  седьмой: 7,
  восьмой: 8
};

export async function fetchJsonWithTimeout<T>(
  url: string,
  init?: RequestInit,
  timeoutMs = 5000
): Promise<T> {
  const ctrl = new AbortController();
  const extSignal = init?.signal;
  if (extSignal?.aborted) {
    ctrl.abort();
  }
  const onAbort = () => ctrl.abort();
  extSignal?.addEventListener('abort', onAbort, { once: true });
  const timer = globalThis.setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { ...init, signal: ctrl.signal });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  } finally {
    globalThis.clearTimeout(timer);
    extSignal?.removeEventListener('abort', onAbort);
  }
}

export function normalizeTitleText(raw: string): string {
  return raw
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/ё/g, 'е')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();
}

export function titleTokens(raw: string): string[] {
  const all = normalizeTitleText(raw).split(' ').filter(Boolean);
  const meaningful = all.filter(
    (w) => !TITLE_STOP_WORDS.has(w) && (w.length >= 2 || /^\d+$/.test(w))
  );
  return meaningful.length > 0 ? meaningful : all;
}

export function extractSeasonNumber(rawTitles: string[]): number {
  for (const raw of rawTitles) {
    const s = raw.toLowerCase();
    const numSeason =
      s.match(/(?:season|сезон|tv[-\s]?|part|часть)\s*[-:]?\s*(\d+)/iu) ??
      s.match(/(\d+)\s*(?:st|nd|rd|th)?\s*(?:season|сезон|part|часть)/iu);
    if (numSeason) {
      const n = Number(numSeason[1]);
      if (Number.isFinite(n) && n >= 1) return n;
    }
    for (const [word, n] of Object.entries(RU_ORDINALS)) {
      if (s.includes(`${word} сезон`)) return n;
    }
  }
  return 1;
}

export function scoreTitleSimilarity(
  candTitles: string[],
  targetTitles: string[],
  candYear?: number | null,
  targetYear?: number | null
): number {
  const cleanCand = candTitles.map((v) => v.trim()).filter(Boolean);
  const cleanTarget = targetTitles.map((v) => v.trim()).filter(Boolean);
  if (cleanCand.length === 0 || cleanTarget.length === 0) {
    return -1;
  }

  const candIsSpecial = cleanCand.some((t) => SPECIAL_TITLE_RE.test(t));
  const targetIsSpecial = cleanTarget.some((t) => SPECIAL_TITLE_RE.test(t));
  if (candIsSpecial !== targetIsSpecial) {
    return -1;
  }

  const candSeason = extractSeasonNumber(cleanCand);
  const targetSeason = extractSeasonNumber(cleanTarget);
  if (candSeason !== targetSeason) {
    return -1;
  }

  let best = -1;
  for (const cTitle of cleanCand) {
    const cNorm = normalizeTitleText(cTitle);
    const cToks = titleTokens(cTitle);
    if (!cNorm || cToks.length === 0) continue;

    for (const tTitle of cleanTarget) {
      const tNorm = normalizeTitleText(tTitle);
      const tToks = titleTokens(tTitle);
      if (!tNorm || tToks.length === 0) continue;

      if (cNorm === tNorm) {
        let exactScore = 900;
        if (targetYear && candYear && Math.abs(targetYear - candYear) <= 1) {
          exactScore += 40;
        }
        best = Math.max(best, exactScore);
        continue;
      }

      const tSet = new Set(tToks);
      let matches = 0;
      for (const tok of cToks) {
        if (tSet.has(tok)) matches += 1;
      }
      if (matches === 0) continue;

      const minLen = Math.min(cToks.length, tToks.length);
      const maxLen = Math.max(cToks.length, tToks.length);
      const containment = matches / minLen;
      const jaccard = matches / maxLen;

      if (containment >= 0.8 && jaccard >= 0.55) {
        let score = Math.round(500 * jaccard + 100 * containment);
        if (targetYear && candYear) {
          if (Math.abs(targetYear - candYear) <= 1) score += 40;
          else if (Math.abs(targetYear - candYear) >= 3) score -= 120;
        }
        best = Math.max(best, score);
      }
    }
  }

  return best;
}

export function dedupeSearchQueries(rawQueries: Array<string | undefined | null>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const q of rawQueries) {
    const trimmed = q?.trim() ?? '';
    const key = trimmed.toLowerCase();
    if (trimmed.length < 2 || seen.has(key)) continue;
    seen.add(key);
    out.push(trimmed);
  }
  return out;
}
