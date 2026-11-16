import { afterEach, describe, expect, it } from 'vitest';
import themesConfig from '../src/themes/themes.json';
import {
  clearThemeOverrides,
  DEFAULT_CUSTOM_HUE,
  HUE_STORAGE_KEY,
  OVERRIDES_STORAGE_KEY,
  paletteToCssVars,
  resolveThemePalette,
  setCustomHue,
  setThemeMode,
  setThemeOverride,
  theme,
  THEME_STORAGE_KEY
} from '../src/app/theme.svelte';

function collectTokenKeys(preset: Record<string, unknown>): string[] {
  const out: string[] = [];
  for (const group of Object.keys(preset).sort()) {
    if (group === 'meta') continue;
    const obj = preset[group] as Record<string, unknown>;
    for (const token of Object.keys(obj).sort()) {
      expect(typeof obj[token]).toBe('string');
      expect((obj[token] as string).trim().length).toBeGreaterThan(0);
      out.push(`${group}.${token}`);
    }
  }
  return out;
}

afterEach(() => {
  clearThemeOverrides();
  setCustomHue(DEFAULT_CUSTOM_HUE);
  setThemeMode('dark');
  delete (globalThis as { window?: unknown }).window;
  delete (globalThis as { document?: unknown }).document;
});

describe('centralized theme JSON and runtime switching', () => {
  it('keeps identical token structure across dark, light, and custom presets in themes.json', () => {
    const { dark, light, custom } = themesConfig.presets;
    const baseKeys = collectTokenKeys(dark as unknown as Record<string, unknown>);
    expect(collectTokenKeys(light as unknown as Record<string, unknown>)).toEqual(baseKeys);
    expect(collectTokenKeys(custom as unknown as Record<string, unknown>)).toEqual(baseKeys);

    expect(dark.meta.logoTone).toBe('light');
    expect(light.meta.logoTone).toBe('dark');
    expect(custom.meta.logoTone).toBe('light');
    expect(dark.logo.color).toBe('#ffffff');
    expect(light.logo.color).toBe('#111114');
  });

  it('resolves custom accent hue and dark canvas RGB without unresolved template placeholders', () => {
    const palette = resolveThemePalette('custom', 280);
    expect(palette.meta.logoTone).toBe('light');
    expect(palette.app.canvas).toBe('hsl(280, 12%, 5%)');
    expect(palette.buttons.mainBg).toBe('hsl(280, 82%, 76%)');

    const cssVars = paletteToCssVars(palette);
    for (const val of Object.values(cssVars)) {
      expect(val).not.toContain('{hue}');
      expect(val).not.toContain('{canvasRgb}');
    }
    expect(cssVars['--mila-app-canvasRgb']).toMatch(/^\d+,\s*\d+,\s*\d+$/);
  });

  it('switches light/custom themes live on documentElement and applies per-element overrides', () => {
    const stored = new Map<string, string>();
    const cssProps = new Map<string, string>();
    const dataset: Record<string, string> = {};

    (globalThis as { window?: unknown }).window = {
      localStorage: {
        getItem: (key: string) => stored.get(key) ?? null,
        setItem: (key: string, val: string) => {
          stored.set(key, val);
        },
        removeItem: (key: string) => {
          stored.delete(key);
        }
      }
    };
    (globalThis as { document?: unknown }).document = {
      documentElement: {
        dataset,
        style: {
          setProperty: (k: string, v: string) => {
            cssProps.set(k, v);
          }
        }
      }
    };

    setThemeMode('light');
    expect(theme.mode).toBe('light');
    expect(stored.get(THEME_STORAGE_KEY)).toBe('light');
    expect(dataset.theme).toBe('light');
    expect(dataset.logoTone).toBe('dark');
    expect(cssProps.get('--mila-logo-color')).toBe('#111114');
    expect(cssProps.get('--mila-app-canvas')).toBe('#f4f4f6');

    setCustomHue(145);
    setThemeMode('custom');
    expect(stored.get(THEME_STORAGE_KEY)).toBe('custom');
    expect(stored.get(HUE_STORAGE_KEY)).toBe('145');
    expect(dataset.theme).toBe('custom');
    expect(dataset.logoTone).toBe('light');
    expect(cssProps.get('--mila-app-canvas')).toBe('hsl(145, 12%, 5%)');
    expect(cssProps.get('--mila-accent-primary')).toBe('hsl(145, 82%, 74%)');

    setThemeOverride('buttons.mainBg', '#ff55aa');
    expect(JSON.parse(stored.get(OVERRIDES_STORAGE_KEY) || '{}')).toEqual({
      'buttons.mainBg': '#ff55aa'
    });
    expect(cssProps.get('--mila-buttons-mainBg')).toBe('#ff55aa');
  });
});
