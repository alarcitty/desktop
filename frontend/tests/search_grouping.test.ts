import { describe, expect, it } from 'vitest';
import {
  computeTitleGroupKey,
  stripSeasonSuffix,
  titleBasePart,
  titleGroupVariants
} from '../src/api/anime';
import { normalizeTitleText } from '../src/api/providers/shared';

describe('stripSeasonSuffix', () => {
  it('strips english season markers', () => {
    expect(stripSeasonSuffix('Jujutsu Kaisen 2nd Season')).toBe('Jujutsu Kaisen');
    expect(stripSeasonSuffix('Kaguya-sama wa Kokurasetai Season 3')).toBe('Kaguya-sama wa Kokurasetai');
    expect(stripSeasonSuffix('Attack on Titan: The Final Season')).toBe('Attack on Titan');
    expect(stripSeasonSuffix('Bleach: Thousand-Year Blood War Part 2')).toBe(
      'Bleach: Thousand-Year Blood War'
    );
    expect(stripSeasonSuffix('One Piece TV 2')).toBe('One Piece');
  });

  it('strips russian season markers', () => {
    expect(stripSeasonSuffix('Магическая битва 2 сезон')).toBe('Магическая битва');
    expect(stripSeasonSuffix('Ван-Пис Сезон 3')).toBe('Ван-Пис');
    expect(stripSeasonSuffix('Наруто: Часть 2')).toBe('Наруто');
  });

  it('strips trailing roman numerals', () => {
    expect(stripSeasonSuffix('Overlord IV')).toBe('Overlord');
    expect(stripSeasonSuffix('Mushoku Tensei II')).toBe('Mushoku Tensei');
  });

  it('keeps titles without season markers intact', () => {
    expect(stripSeasonSuffix('One Piece')).toBe('One Piece');
    expect(stripSeasonSuffix('Re:Zero kara Hajimeru Isekai Seikatsu')).toBe(
      'Re:Zero kara Hajimeru Isekai Seikatsu'
    );
    expect(stripSeasonSuffix('Fate/stay night')).toBe('Fate/stay night');
  });
});

describe('titleBasePart', () => {
  it('splits colon subtitles with a space', () => {
    expect(titleBasePart('One Piece: Egghead')).toBe('One Piece');
    expect(titleBasePart('Kaguya-sama wa Kokurasetai: Ultra Romantic')).toBe(
      'Kaguya-sama wa Kokurasetai'
    );
  });

  it('does not split glued colons like Re:Zero', () => {
    expect(titleBasePart('Re:Zero kara Hajimeru Isekai Seikatsu')).toBe(
      'Re:Zero kara Hajimeru Isekai Seikatsu'
    );
  });

  it('strips season marker before splitting subtitle', () => {
    expect(titleBasePart('Jujutsu Kaisen 2nd Season: Hidden Inventory')).toBe('Jujutsu Kaisen');
  });
});

describe('computeTitleGroupKey', () => {
  const roots = new Set(
    [
      'One Piece',
      'Jujutsu Kaisen',
      'Kaguya-sama wa Kokurasetai',
      'Attack on Titan',
      'Fate/stay night'
    ].map((t) => normalizeTitleText(t))
  );

  it('collapses season entries onto the root key', () => {
    const root = computeTitleGroupKey('One Piece', roots);
    expect(computeTitleGroupKey('One Piece: Egghead', roots)).toBe(root);
    expect(computeTitleGroupKey('Jujutsu Kaisen 2nd Season', roots)).toBe(
      computeTitleGroupKey('Jujutsu Kaisen', roots)
    );
    expect(computeTitleGroupKey('Attack on Titan: The Final Season', roots)).toBe(
      computeTitleGroupKey('Attack on Titan', roots)
    );
    expect(computeTitleGroupKey('Kaguya-sama wa Kokurasetai: Ultra Romantic', roots)).toBe(
      computeTitleGroupKey('Kaguya-sama wa Kokurasetai', roots)
    );
  });

  it('collapses roman-numeral sequels', () => {
    expect(computeTitleGroupKey('Overlord IV', roots)).toBe(computeTitleGroupKey('Overlord', roots));
  });

  it('keeps distinct franchises separate', () => {
    expect(computeTitleGroupKey('One Piece', roots)).not.toBe(
      computeTitleGroupKey('Naruto', roots)
    );
    expect(computeTitleGroupKey('Re:Zero kara Hajimeru Isekai Seikatsu', roots)).not.toBe(
      computeTitleGroupKey('One Piece', roots)
    );
  });

  it('does not merge subtitle into an absent root', () => {
    const key = computeTitleGroupKey('Fullmetal Alchemist: Brotherhood', roots);
    expect(key).toBe(normalizeTitleText('Fullmetal Alchemist Brotherhood').replace(/[^a-z0-9а-яё]+/gi, ' ').trim());
  });

  it('merges subtitle when the root is present among candidates', () => {
    const withFma = new Set([...roots, normalizeTitleText('Fullmetal Alchemist')]);
    expect(computeTitleGroupKey('Fullmetal Alchemist: Brotherhood', withFma)).toBe(
      computeTitleGroupKey('Fullmetal Alchemist', withFma)
    );
  });

  it('returns empty key for empty titles', () => {
    expect(computeTitleGroupKey('', roots)).toBe('');
    expect(computeTitleGroupKey('!!!', roots)).toBe('');
  });
});

describe('titleGroupVariants', () => {
  it('produces full, stripped and base variants', () => {
    const variants = titleGroupVariants('Jujutsu Kaisen 2nd Season: Hidden Inventory');
    expect(variants).toContain(normalizeTitleText('Jujutsu Kaisen 2nd Season: Hidden Inventory'));
    expect(variants).toContain(normalizeTitleText('Jujutsu Kaisen'));
  });

  it('dedupes identical variants and drops empties', () => {
    expect(titleGroupVariants('One Piece')).toEqual([normalizeTitleText('One Piece')]);
    expect(titleGroupVariants('')).toEqual([]);
  });

  it('lets a season title match the root registration', () => {
    const registered = new Map<string, string>();
    for (const v of titleGroupVariants('One Piece')) registered.set(v, 'group:one-piece');
    const hit = titleGroupVariants('One Piece: Egghead').find((v) => registered.has(v));
    expect(hit).toBeDefined();
    expect(registered.get(hit!)).toBe('group:one-piece');
  });
});