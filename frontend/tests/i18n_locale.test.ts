import { afterEach, describe, expect, it } from 'vitest';
import en from '../src/i18n/locales/en.json';
import ru from '../src/i18n/locales/ru.json';
import zh from '../src/i18n/locales/zh.json';
import uk from '../src/i18n/locales/uk.json';
import {
  i18n,
  LOCALE_STORAGE_KEY,
  setLocale,
  SUPPORTED_LOCALES,
  t
} from '../src/app/i18n.svelte';
import { translateGenreName } from '../src/api/anime';

function collectLeafPaths(obj: Record<string, unknown>, prefix = ''): string[] {
  const paths: string[] = [];
  for (const key of Object.keys(obj).sort()) {
    const val = obj[key];
    const nextPath = prefix ? `${prefix}.${key}` : key;
    if (val && typeof val === 'object' && !Array.isArray(val)) {
      paths.push(...collectLeafPaths(val as Record<string, unknown>, nextPath));
    } else {
      expect(typeof val, `Expected string at key ${nextPath}`).toBe('string');
      expect((val as string).trim().length, `Empty string at key ${nextPath}`).toBeGreaterThan(0);
      paths.push(nextPath);
    }
  }
  return paths;
}

afterEach(() => {
  setLocale('en');
  delete (globalThis as { window?: unknown }).window;
});

describe('i18n locales and translations', () => {
  it('keeps identical key trees across en, ru, zh, and uk dictionaries', () => {
    const basePaths = collectLeafPaths(en as unknown as Record<string, unknown>);
    expect(collectLeafPaths(ru as unknown as Record<string, unknown>)).toEqual(basePaths);
    expect(collectLeafPaths(zh as unknown as Record<string, unknown>)).toEqual(basePaths);
    expect(collectLeafPaths(uk as unknown as Record<string, unknown>)).toEqual(basePaths);
    expect(SUPPORTED_LOCALES.map((item) => item.code)).toEqual(['en', 'ru', 'zh', 'uk']);
  });

  it('switches locale reactively, formats variables, and persists to localStorage', () => {
    const stored = new Map<string, string>();
    (globalThis as { window?: unknown }).window = {
      localStorage: {
        getItem: (key: string) => stored.get(key) ?? null,
        setItem: (key: string, value: string) => {
          stored.set(key, value);
        }
      }
    };

    setLocale('en');
    expect(i18n.locale).toBe('en');
    expect(t('setup.welcome')).toBe('Welcome');
    expect(t('anime.watchEp', { n: 5 })).toBe('Watch Ep 5');

    setLocale('ru');
    expect(stored.get(LOCALE_STORAGE_KEY)).toBe('ru');
    expect(t('setup.welcome')).toBe('Добро пожаловать');
    expect(t('nav.home')).toBe('Главная');

    setLocale('zh');
    expect(stored.get(LOCALE_STORAGE_KEY)).toBe('zh');
    expect(t('setup.welcome')).toBe('欢迎使用');
    expect(t('nav.library')).toBe('片库');

    setLocale('uk');
    expect(stored.get(LOCALE_STORAGE_KEY)).toBe('uk');
    expect(t('setup.welcome')).toBe('Вітаємо');
    expect(t('nav.news')).toBe('Новини');
  });

  it('translates Russian AniLiberty genres to English', () => {
    expect(translateGenreName('драма')).toBe('Drama');
    expect(translateGenreName('Научная фантастика')).toBe('Sci-Fi');
    expect(translateGenreName('Комедия')).toBe('Comedy');
    expect(translateGenreName('Action')).toBe('Action');
  });
});
