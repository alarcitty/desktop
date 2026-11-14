import { ensureShikimoriArtwork, truncateDescription } from './anime';

const SHIKI_TOPICS_URL = 'https://shikimori.io/api/topics?forum=news&limit=14';
const ANILIBRIA_LATEST_URL = 'https://anilibria.top/api/v1/anime/releases/latest?limit=12';

export type NewsKind = 'news' | 'release';

export interface NewsItem {
  id: string;
  kind: NewsKind;
  tag: string;
  title: string;
  excerpt: string;
  paragraphs: string[];
  quote?: string;
  publishedAt: string;
  publishedMs: number;
  relativeTime: string;
  image: string;
  poster: string;
  isLandscape: boolean;
  shikiId?: number;
  releaseAlias?: string;
  episodeOrdinal?: number;
  score?: number;
  commentsCount?: number;
  externalUrl?: string;
  genres?: string[];
  hashtags: string[];
}

export interface NewsFeed {
  items: NewsItem[];
  fetchedAt: number;
}

interface RawShikiTopic {
  id: number;
  topic_title: string;
  body?: string;
  html_body?: string;
  created_at: string;
  comments_count?: number;
  linked_type?: string | null;
  linked?: {
    id: number;
    name?: string;
    russian?: string;
    url?: string;
    kind?: string;
    score?: string | number;
    episodes?: number;
    image?: {
      original?: string;
      preview?: string;
    };
  } | null;
}

interface RawLibLatestRelease {
  id: number;
  alias: string;
  year?: number;
  type?: { value?: string; description?: string };
  season?: { description?: string };
  name?: { main?: string; english?: string };
  shikimori?: { id?: number; url?: string; rating?: number };
  poster?: { src?: string; preview?: string; optimized?: { src?: string; preview?: string } };
  fresh_at?: string;
  updated_at?: string;
  description?: string;
  comments_count?: number;
  genres?: Array<{ name?: string }>;
  latest_episode?: {
    ordinal?: number;
    updated_at?: string;
    preview?: {
      src?: string;
      preview?: string;
      optimized?: { src?: string; preview?: string };
    };
  } | null;
}

let cachedFeed: NewsFeed | null = null;
let inflightPromise: Promise<NewsFeed> | null = null;

function resolveShikiAsset(raw?: string | null): string {
  if (!raw || raw.includes('missing_')) return '';
  if (raw.startsWith('http://') || raw.startsWith('https://')) return raw;
  if (raw.startsWith('//')) return `https:${raw}`;
  if (raw.startsWith('/')) return `https://shikimori.io${raw}`;
  return raw;
}

function resolveLibAsset(raw?: string | null): string {
  if (!raw) return '';
  if (raw.startsWith('http://') || raw.startsWith('https://')) return raw;
  if (raw.startsWith('/')) return `https://anilibria.top${raw}`;
  return raw;
}

async function fetchJson<T>(url: string, timeoutMs = 6500): Promise<T> {
  const ctrl = new AbortController();
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { signal: ctrl.signal });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  } finally {
    window.clearTimeout(timer);
  }
}

function extractAnimeIdsFromTopic(topic: RawShikiTopic): number[] {
  const ids: number[] = [];
  if (topic.linked_type === 'Anime' && topic.linked?.id) {
    ids.push(topic.linked.id);
  }
  const body = topic.body ?? '';
  const regex = /\[anime=(\d+)\]/g;
  let match: RegExpExecArray | null;
  while ((match = regex.exec(body)) !== null) {
    const num = Number(match[1]);
    if (Number.isFinite(num) && num > 0 && !ids.includes(num)) {
      ids.push(num);
    }
  }
  return ids;
}

function extractInlineMedia(topic: RawShikiTopic): string {
  const body = `${topic.body ?? ''}\n${topic.html_body ?? ''}`;
  const imgMatch = /\[img\](https?:\/\/[^[\]\s]+)\[\/img\]/i.exec(body);
  if (imgMatch?.[1]) return imgMatch[1];

  const htmlImg = /<img[^>]+src=["']([^"']+)["']/i.exec(topic.html_body ?? '');
  if (htmlImg?.[1] && !htmlImg[1].includes('missing_') && !htmlImg[1].includes('smiles')) {
    return resolveShikiAsset(htmlImg[1]);
  }

  const ytMatch =
    /(?:youtube\.com\/(?:watch\?v=|embed\/|shorts\/)|youtu\.be\/|\[video=youtube;)([a-zA-Z0-9_-]{11})/i.exec(body);
  if (ytMatch?.[1]) {
    return `https://i.ytimg.com/vi/${ytMatch[1]}/hqdefault.jpg`;
  }
  return '';
}

function cleanTopicBBCode(raw: string): { paragraphs: string[]; quote?: string } {
  let quoteText = '';
  const withoutQuotes = raw
    .replace(/\r\n/g, '\n')
    .replace(/\[replies=[^\]]*\]/gi, '')
    .replace(/\[quote\]([\s\S]*?)\[\/quote\]/gi, (_, inner: string) => {
      if (!quoteText) {
        quoteText = inner;
      }
      return '\n';
    });

  const stripTags = (text: string): string =>
    text
      .replace(/\[url=([^\]]+)\]([\s\S]*?)\[\/url\]/gi, '$2')
      .replace(/\[url\]([\s\S]*?)\[\/url\]/gi, '')
      .replace(/\[(?:anime|manga|person|character|club|studio|user)=\d+\]([\s\S]*?)\[\/(?:anime|manga|person|character|club|studio|user)\]/gi, '$1')
      .replace(/\[img\][\s\S]*?\[\/img\]/gi, '')
      .replace(/\[video=[^\]]*\][\s\S]*?\[\/video\]/gi, '')
      .replace(/\[br\]/gi, '\n')
      .replace(/\[\/?[a-zA-Z0-9_*]+(?:=[^\]]*)?\]/g, '')
      .replace(/▼\/\/\s*Играйте прямо сейчас:\s*/gi, '')
      .replace(/[ \t]{2,}/g, ' ')
      .trim();

  const paragraphs = stripTags(withoutQuotes)
    .split(/\n{2,}/)
    .map((p) => p.replace(/\n+/g, ' ').trim())
    .filter(Boolean);

  const cleanedQuote = quoteText ? stripTags(quoteText).replace(/\n+/g, ' ').trim() : undefined;
  return {
    paragraphs: paragraphs.length > 0 ? paragraphs : cleanedQuote ? [cleanedQuote] : [],
    quote: paragraphs.length > 0 && cleanedQuote ? cleanedQuote : undefined
  };
}

export function formatRelativeTime(isoDate: string): string {
  const ms = Date.parse(isoDate);
  if (!Number.isFinite(ms)) return '';
  const diffSec = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  if (diffSec < 3600) {
    const mins = Math.max(1, Math.floor(diffSec / 60));
    return `${mins}m ago`;
  }
  if (diffSec < 86_400) {
    return `${Math.floor(diffSec / 3600)}h ago`;
  }
  const days = Math.floor(diffSec / 86_400);
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days}d ago`;
  return new Date(ms).toLocaleDateString('en-US', { day: 'numeric', month: 'short' });
}

function detectNewsTag(title: string, linkedKind?: string): string {
  const lower = title.toLowerCase();
  if (lower.includes('трейлер') || lower.includes('тизер') || lower.includes('trailer')) return 'TRAILER';
  if (lower.includes('анонс') || lower.includes('сезон')) return 'ANNOUNCEMENT';
  if (lower.includes('постер') || lower.includes('визуал')) return 'KEY VISUAL';
  if (lower.includes('финал') || lower.includes('заверш')) return 'FINALE';
  if (linkedKind === 'manga') return 'MANGA';
  return 'NEWS';
}

function buildHashtags(parts: {
  tag: string;
  year?: number;
  episodes?: number;
  episodeOrdinal?: number;
  genres?: string[];
}): string[] {
  const tags: string[] = [];
  const pushTag = (raw: string) => {
    const trimmed = raw.trim();
    const label = trimmed ? trimmed.charAt(0).toUpperCase() + trimmed.slice(1).toLowerCase() : '';
    if (label && !tags.some((t) => t.toLowerCase() === label.toLowerCase())) {
      tags.push(label);
    }
  };
  pushTag(parts.tag);
  if (parts.year && parts.year > 1990) tags.push(String(parts.year));
  if (parts.episodeOrdinal) {
    tags.push(`Ep ${parts.episodeOrdinal}`);
  } else if (parts.episodes && parts.episodes > 0) {
    tags.push(`${parts.episodes} ep`);
  }
  for (const g of (parts.genres ?? []).slice(0, 3)) {
    pushTag(g);
  }
  return tags;
}

export async function loadNewsFeed(force = false): Promise<NewsFeed> {
  if (!force && cachedFeed && Date.now() - cachedFeed.fetchedAt < 120_000) {
    return cachedFeed;
  }
  if (!force && inflightPromise) {
    return inflightPromise;
  }

  inflightPromise = (async () => {
    const [topicsRes, releasesRes] = await Promise.allSettled([
      fetchJson<RawShikiTopic[]>(SHIKI_TOPICS_URL, 7000),
      fetchJson<RawLibLatestRelease[]>(ANILIBRIA_LATEST_URL, 7000)
    ]);

    const rawTopics = topicsRes.status === 'fulfilled' && Array.isArray(topicsRes.value) ? topicsRes.value : [];
    const rawReleases = releasesRes.status === 'fulfilled' && Array.isArray(releasesRes.value) ? releasesRes.value : [];

    // Collect all referenced Shikimori Anime IDs to enrich news cards with HD screenshots in one GraphQL batch.
    const shikiIds = new Set<number>();
    const topicAnimeMap = new Map<number, number[]>();
    for (const t of rawTopics) {
      const ids = extractAnimeIdsFromTopic(t);
      topicAnimeMap.set(t.id, ids);
      if (ids[0]) shikiIds.add(ids[0]);
    }
    for (const r of rawReleases) {
      if (r.shikimori?.id) shikiIds.add(r.shikimori.id);
    }

    const artworkMap =
      shikiIds.size > 0
        ? await ensureShikimoriArtwork(Array.from(shikiIds)).catch(() => new Map())
        : new Map();

    const newsItems: NewsItem[] = rawTopics
      .filter((t) => Boolean(t.topic_title))
      .map((t) => {
        const animeIds = topicAnimeMap.get(t.id) ?? [];
        const primaryShikiId = animeIds[0];
        const art = primaryShikiId ? artworkMap.get(primaryShikiId) : undefined;
        const linkedPoster = resolveShikiAsset(t.linked?.image?.original || t.linked?.image?.preview);
        const inlineMedia = extractInlineMedia(t);
        const screenshot = art?.screenshots?.[0] ?? '';
        const poster = art?.posterOriginal || linkedPoster || '';
        const heroImage = screenshot || inlineMedia || poster;
        const isLandscape = Boolean(screenshot || inlineMedia);

        const parsed = cleanTopicBBCode(t.body ?? '');
        const summarySource = parsed.paragraphs.join(' ') || parsed.quote || t.topic_title;
        const rawScore = Number(t.linked?.score ?? art?.score ?? 0);
        const tag = detectNewsTag(t.topic_title, t.linked?.kind);
        const genres = art?.genres?.slice(0, 3) ?? [];

        return {
          id: `shiki-${t.id}`,
          kind: 'news',
          tag,
          title: t.topic_title.trim(),
          excerpt: truncateDescription(summarySource, 145),
          paragraphs: parsed.paragraphs.length > 0 ? parsed.paragraphs : [t.topic_title],
          quote: parsed.quote,
          publishedAt: t.created_at,
          publishedMs: Date.parse(t.created_at) || 0,
          relativeTime: formatRelativeTime(t.created_at),
          image: heroImage,
          poster: poster || inlineMedia,
          isLandscape,
          shikiId: primaryShikiId,
          score: Number.isFinite(rawScore) && rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
          commentsCount: t.comments_count ?? 0,
          externalUrl: `https://shikimori.io/forum/news/${t.id}`,
          genres,
          hashtags: buildHashtags({
            tag,
            year: art?.year,
            episodes: art?.episodes || t.linked?.episodes || undefined,
            genres
          })
        };
      });

    const releaseItems: NewsItem[] = rawReleases.map((r) => {
      const shikiId = r.shikimori?.id;
      const art = shikiId ? artworkMap.get(shikiId) : undefined;
      const epPreview = resolveLibAsset(
        r.latest_episode?.preview?.optimized?.src ||
          r.latest_episode?.preview?.src ||
          r.latest_episode?.preview?.preview
      );
      const libPoster = resolveLibAsset(
        r.poster?.optimized?.src || r.poster?.src || r.poster?.preview
      );
      const screenshot = art?.screenshots?.[0] ?? '';
      const heroImage = epPreview || screenshot || art?.posterOriginal || libPoster;
      const poster = art?.posterOriginal || libPoster;
      const epOrdinal = r.latest_episode?.ordinal;
      const publishedAt = r.fresh_at || r.latest_episode?.updated_at || r.updated_at || '';
      const rawScore = r.shikimori?.rating || art?.score || 0;
      const desc: string = String(r.description ?? art?.description ?? '').replace(/\r\n/g, '\n').trim();
      const paragraphs = desc
        .split(/\n{2,}/)
        .map((p: string) => p.replace(/\n+/g, ' ').trim())
        .filter(Boolean);

      const genres = (r.genres ?? [])
        .map((g) => g.name ?? '')
        .filter(Boolean)
        .slice(0, 3);
      const tag = epOrdinal ? `EPISODE ${epOrdinal}` : r.type?.description || 'RELEASE';

      return {
        id: `lib-${r.id}`,
        kind: 'release',
        tag,
        title: r.name?.main || r.name?.english || 'Anime Release',
        excerpt: truncateDescription(desc || r.name?.english || 'New episode available in 1080p HLS.', 135),
        paragraphs: paragraphs.length > 0 ? paragraphs : [r.name?.english || r.name?.main || ''],
        publishedAt,
        publishedMs: Date.parse(publishedAt) || 0,
        relativeTime: formatRelativeTime(publishedAt),
        image: heroImage,
        poster,
        isLandscape: Boolean(epPreview || screenshot),
        shikiId,
        releaseAlias: r.alias,
        episodeOrdinal: epOrdinal,
        score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
        commentsCount: r.comments_count ?? 0,
        externalUrl: r.shikimori?.url,
        genres,
        hashtags: buildHashtags({
          tag: epOrdinal ? 'episode' : 'release',
          year: r.year || art?.year,
          episodeOrdinal: epOrdinal,
          episodes: art?.episodes,
          genres
        })
      };
    });

    // Interleave Shikimori editorial news and AniLiberty fresh episode drops evenly so 'All' stays balanced.
    const combined: NewsItem[] = [];
    const maxLen = Math.max(newsItems.length, releaseItems.length);
    for (let i = 0; i < maxLen; i++) {
      if (i < newsItems.length) combined.push(newsItems[i]);
      if (i < releaseItems.length) combined.push(releaseItems[i]);
    }
    cachedFeed = { items: combined, fetchedAt: Date.now() };
    return cachedFeed;
  })();

  try {
    return await inflightPromise;
  } finally {
    inflightPromise = null;
  }
}

export function getCachedNewsFeed(): NewsFeed | null {
  return cachedFeed;
}
