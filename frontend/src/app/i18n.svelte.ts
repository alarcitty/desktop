import en from '../i18n/locales/en.json';
import ru from '../i18n/locales/ru.json';
import zh from '../i18n/locales/zh.json';
import uk from '../i18n/locales/uk.json';

export type LocaleCode = 'en' | 'ru' | 'zh' | 'uk';

export const LOCALE_STORAGE_KEY = 'mila.setup.language';

export const SUPPORTED_LOCALES: Array<{ code: LocaleCode; nativeLabel: string }> = [
  { code: 'en', nativeLabel: 'English' },
  { code: 'ru', nativeLabel: 'Русский' },
  { code: 'zh', nativeLabel: '中文' },
  { code: 'uk', nativeLabel: 'Українська' }
];

type LocaleDict = typeof en;

const DICTIONARIES: Record<LocaleCode, LocaleDict> = {
  en,
  ru,
  zh,
  uk
};

export function isLocaleCode(value: unknown): value is LocaleCode {
  return value === 'en' || value === 'ru' || value === 'zh' || value === 'uk';
}

function readInitialLocale(): LocaleCode {
  if (typeof window === 'undefined') return 'en';
  try {
    const saved = window.localStorage.getItem(LOCALE_STORAGE_KEY);
    if (isLocaleCode(saved)) return saved;
  } catch {
    // Ignore storage access failures
  }
  return 'en';
}

export const i18n = $state<{ locale: LocaleCode }>({
  locale: readInitialLocale()
});

function syncDocumentLang(code: LocaleCode): void {
  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.lang = code;
  }
}

syncDocumentLang(i18n.locale);

export function setLocale(code: LocaleCode): void {
  i18n.locale = code;
  syncDocumentLang(code);
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, code);
    } catch {
      // Ignore storage write errors
    }
  }
}

function lookupPath(dict: unknown, key: string): string | null {
  const parts = key.split('.');
  let cur: unknown = dict;
  for (const part of parts) {
    if (!cur || typeof cur !== 'object' || !(part in (cur as Record<string, unknown>))) {
      return null;
    }
    cur = (cur as Record<string, unknown>)[part];
  }
  return typeof cur === 'string' ? cur : null;
}

export function t(key: string, vars?: Record<string, string | number>): string {
  const activeDict = DICTIONARIES[i18n.locale] ?? DICTIONARIES.en;
  let template = lookupPath(activeDict, key) ?? lookupPath(DICTIONARIES.en, key) ?? key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      template = template.replaceAll(`{${k}}`, String(v));
    }
  }
  return template;
}
