import themesConfig from '../themes/themes.json';

export type ThemeMode = 'dark' | 'light' | 'custom';
export type LogoTone = 'light' | 'dark';

export interface ThemeMeta {
  id: string;
  mode: string;
  logoTone: LogoTone;
}

export interface ThemePreset {
  meta: ThemeMeta;
  app: Record<string, string>;
  text: Record<string, string>;
  accent: Record<string, string>;
  logo: Record<string, string>;
  header: Record<string, string>;
  buttons: Record<string, string>;
  inputs: Record<string, string>;
  popup: Record<string, string>;
  setup: Record<string, string>;
  cards: Record<string, string>;
  progress: Record<string, string>;
  toggle: Record<string, string>;
  status: Record<string, string>;
  media: Record<string, string>;
}

export type ThemeTokenOverrides = Record<string, string>;

export const THEME_STORAGE_KEY = 'mila.setup.theme';
export const HUE_STORAGE_KEY = 'mila.setup.customHue';
export const OVERRIDES_STORAGE_KEY = 'mila.theme.overrides';
export const DEFAULT_CUSTOM_HUE = 215;

export const SUPPORTED_THEMES: ThemeMode[] = ['dark', 'light', 'custom'];

export function isThemeMode(value: unknown): value is ThemeMode {
  return value === 'dark' || value === 'light' || value === 'custom';
}

export function clampHue(raw: number): number {
  if (!Number.isFinite(raw)) return DEFAULT_CUSTOM_HUE;
  return Math.round(Math.max(0, Math.min(360, raw)));
}

/** Converts HSL (h in 0..360, s/l in 0..100) into an "r, g, b" tuple string for rgba() CSS rules. */
export function hslToRgbTuple(h: number, s: number, l: number): string {
  const hueNorm = (((h % 360) + 360) % 360) / 360;
  const sat = Math.max(0, Math.min(100, s)) / 100;
  const light = Math.max(0, Math.min(100, l)) / 100;

  if (sat === 0) {
    const v = Math.round(light * 255);
    return `${v}, ${v}, ${v}`;
  }

  const hue2rgb = (p: number, q: number, t: number): number => {
    let tt = t;
    if (tt < 0) tt += 1;
    if (tt > 1) tt -= 1;
    if (tt < 1 / 6) return p + (q - p) * 6 * tt;
    if (tt < 1 / 2) return q;
    if (tt < 2 / 3) return p + (q - p) * (2 / 3 - tt) * 6;
    return p;
  };

  const q = light < 0.5 ? light * (1 + sat) : light + sat - light * sat;
  const p = 2 * light - q;
  const r = Math.round(hue2rgb(p, q, hueNorm + 1 / 3) * 255);
  const g = Math.round(hue2rgb(p, q, hueNorm) * 255);
  const b = Math.round(hue2rgb(p, q, hueNorm - 1 / 3) * 255);
  return `${r}, ${g}, ${b}`;
}

function resolveTemplateValue(value: string, hue: number, canvasRgb: string): string {
  return value
    .replaceAll('{hue}', String(hue))
    .replaceAll('{canvasRgb}', canvasRgb);
}

/** Resolves a complete palette from themes.json, substituting custom hue and applying per-element overrides. */
export function resolveThemePalette(
  mode: ThemeMode,
  hue: number,
  overrides: ThemeTokenOverrides = {}
): ThemePreset {
  const safeHue = clampHue(hue);
  const presets = themesConfig.presets as Record<ThemeMode, ThemePreset>;
  const source = presets[mode] ?? presets.dark;
  const customCanvasRgb = hslToRgbTuple(safeHue, 12, 5);

  const out: Record<string, unknown> = {
    meta: { ...source.meta }
  };

  for (const [group, entries] of Object.entries(source)) {
    if (group === 'meta' || !entries || typeof entries !== 'object') continue;
    const resolvedGroup: Record<string, string> = {};
    for (const [token, rawVal] of Object.entries(entries as Record<string, string>)) {
      const dotPath = `${group}.${token}`;
      const overridden = overrides[dotPath];
      const baseStr = typeof overridden === 'string' && overridden.trim() ? overridden.trim() : rawVal;
      resolvedGroup[token] = resolveTemplateValue(baseStr, safeHue, customCanvasRgb);
    }
    out[group] = resolvedGroup;
  }

  return out as unknown as ThemePreset;
}

/** Flattens a resolved palette into "--mila-<group>-<token>" CSS custom properties. */
export function paletteToCssVars(palette: ThemePreset): Record<string, string> {
  const vars: Record<string, string> = {};
  for (const [group, entries] of Object.entries(palette)) {
    if (group === 'meta' || !entries || typeof entries !== 'object') continue;
    for (const [token, value] of Object.entries(entries as Record<string, string>)) {
      vars[`--mila-${group}-${token}`] = value;
    }
  }
  return vars;
}

function readInitialMode(): ThemeMode {
  if (typeof window === 'undefined') return 'dark';
  try {
    const saved = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (isThemeMode(saved)) return saved;
  } catch {
    // Ignore restricted storage contexts
  }
  return 'dark';
}

function readInitialHue(): number {
  if (typeof window === 'undefined') return DEFAULT_CUSTOM_HUE;
  try {
    const raw = window.localStorage.getItem(HUE_STORAGE_KEY);
    if (raw !== null && raw.trim() !== '') {
      return clampHue(Number(raw));
    }
  } catch {
    // Ignore restricted storage contexts
  }
  return DEFAULT_CUSTOM_HUE;
}

function readInitialOverrides(): ThemeTokenOverrides {
  if (typeof window === 'undefined') return {};
  try {
    const raw = window.localStorage.getItem(OVERRIDES_STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const clean: ThemeTokenOverrides = {};
      for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
        if (typeof v === 'string' && v.trim()) {
          clean[k] = v.trim();
        }
      }
      return clean;
    }
  } catch {
    // Ignore malformed override JSON
  }
  return {};
}

export const theme = $state<{
  mode: ThemeMode;
  hue: number;
  overrides: ThemeTokenOverrides;
}>({
  mode: readInitialMode(),
  hue: readInitialHue(),
  overrides: readInitialOverrides()
});

export function getResolvedTheme(): ThemePreset {
  return resolveThemePalette(theme.mode, theme.hue, theme.overrides);
}

export function applyThemeToDocument(
  mode: ThemeMode = theme.mode,
  hue: number = theme.hue,
  overrides: ThemeTokenOverrides = theme.overrides
): ThemePreset {
  const palette = resolveThemePalette(mode, hue, overrides);
  if (typeof document !== 'undefined' && document.documentElement) {
    const root = document.documentElement;
    root.dataset.theme = mode;
    root.dataset.themeMode = palette.meta.mode;
    root.dataset.logoTone = palette.meta.logoTone;
    const vars = paletteToCssVars(palette);
    for (const [prop, val] of Object.entries(vars)) {
      root.style.setProperty(prop, val);
    }
  }
  return palette;
}

function persistStorage(key: string, val: string): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(key, val);
  } catch {
    // Ignore storage write errors
  }
}

let hueApplyRaf = 0;
let pendingHueApply: number | null = null;
let huePersistTimer: number | undefined;

export function setThemeMode(mode: ThemeMode): void {
  theme.mode = mode;
  persistStorage(THEME_STORAGE_KEY, mode);
  applyThemeToDocument(mode, theme.hue, theme.overrides);
}

export function setCustomHue(hue: number): void {
  const safe = clampHue(hue);
  theme.hue = safe;
  if (typeof window !== 'undefined' && typeof window.setTimeout === 'function' && typeof window.clearTimeout === 'function') {
    window.clearTimeout(huePersistTimer);
    huePersistTimer = window.setTimeout(() => {
      huePersistTimer = undefined;
      persistStorage(HUE_STORAGE_KEY, String(theme.hue));
    }, 180);
  } else {
    persistStorage(HUE_STORAGE_KEY, String(safe));
  }
  if (theme.mode === 'custom') {
    pendingHueApply = safe;
    if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
      if (!hueApplyRaf) {
        hueApplyRaf = window.requestAnimationFrame(() => {
          hueApplyRaf = 0;
          const h = pendingHueApply;
          pendingHueApply = null;
          if (h !== null && theme.mode === 'custom') {
            applyThemeToDocument('custom', h, theme.overrides);
          }
        });
      }
    } else {
      applyThemeToDocument('custom', safe, theme.overrides);
    }
  }
}

export function setThemeOverride(dotPath: string, value: string): void {
  const next = { ...theme.overrides };
  if (!value.trim()) {
    delete next[dotPath];
  } else {
    next[dotPath] = value.trim();
  }
  theme.overrides = next;
  persistStorage(OVERRIDES_STORAGE_KEY, JSON.stringify(next));
  applyThemeToDocument(theme.mode, theme.hue, next);
}

export function clearThemeOverrides(): void {
  theme.overrides = {};
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.removeItem(OVERRIDES_STORAGE_KEY);
    } catch {
      // Ignore storage remove errors
    }
  }
  applyThemeToDocument(theme.mode, theme.hue, {});
}

// Apply immediately on module load so first paint has exact theme CSS variables.
applyThemeToDocument(theme.mode, theme.hue, theme.overrides);
