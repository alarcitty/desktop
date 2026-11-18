import { describe, expect, it } from 'vitest';
import { encodeMilaShareLink, parseMilaShareLink } from '../src/app/router.svelte';

describe('encodeMilaShareLink', () => {
  it('encodes shiki id links', () => {
    expect(encodeMilaShareLink({ shikiId: 52991 })).toBe('mila://anime/52991');
    expect(encodeMilaShareLink({ shikiId: 52991, episodeOrdinal: 3 })).toBe(
      'mila://anime/52991?ep=3'
    );
  });

  it('falls back to alias when no shiki id', () => {
    expect(encodeMilaShareLink({ alias: 'frieren' })).toBe('mila://anime/alias/frieren');
    expect(encodeMilaShareLink({ alias: 'one piece', episodeOrdinal: 1100 })).toBe(
      'mila://anime/alias/one%20piece?ep=1100'
    );
  });

  it('ignores invalid ids and episode numbers', () => {
    expect(encodeMilaShareLink({ shikiId: 0, alias: 'x' })).toBe('mila://anime/alias/x');
    expect(encodeMilaShareLink({ shikiId: 5, episodeOrdinal: -2 })).toBe('mila://anime/5');
    expect(encodeMilaShareLink({})).toBe('mila://anime');
  });
});

describe('parseMilaShareLink', () => {
  it('parses canonical mila links', () => {
    expect(parseMilaShareLink('mila://anime/52991')).toMatchObject({ shikiId: 52991 });
    expect(parseMilaShareLink('mila://anime/52991?ep=3')).toMatchObject({
      shikiId: 52991,
      episodeOrdinal: 3
    });
    expect(parseMilaShareLink('MILA://ANIME/52991')).toMatchObject({ shikiId: 52991 });
  });

  it('parses alias links with encoding', () => {
    expect(parseMilaShareLink('mila://anime/alias/one%20piece?ep=1100')).toMatchObject({
      alias: 'one piece',
      episodeOrdinal: 1100
    });
  });

  it('parses compact codes and shikimori urls', () => {
    expect(parseMilaShareLink('anime:52991#3')).toMatchObject({ shikiId: 52991, episodeOrdinal: 3 });
    expect(parseMilaShareLink('https://shikimori.io/animes/52991-sousou-no-frieren')).toMatchObject({
      shikiId: 52991
    });
  });

  it('rejects garbage', () => {
    expect(parseMilaShareLink('')).toBeNull();
    expect(parseMilaShareLink('mila://room/ABCD')).toBeNull();
    expect(parseMilaShareLink('mila://anime/0')).toBeNull();
    expect(parseMilaShareLink('https://example.com')).toBeNull();
  });

  it('roundtrips encode -> parse', () => {
    const link = encodeMilaShareLink({ shikiId: 21, episodeOrdinal: 7 });
    expect(parseMilaShareLink(link)).toMatchObject({ shikiId: 21, episodeOrdinal: 7 });
    const aliasLink = encodeMilaShareLink({ alias: 'steins gate', episodeOrdinal: 2 });
    expect(parseMilaShareLink(aliasLink)).toMatchObject({
      alias: 'steins gate',
      episodeOrdinal: 2
    });
  });
});