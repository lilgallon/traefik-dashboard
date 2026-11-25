// Interface language: English or French. The choice is remembered by the browser;
// without one, the browser's preferred languages decide (English by default).
// `t()` reads a reactive state, so everything that calls it while rendering is
// redrawn when the language changes.

import { en } from './locales/en'
import { fr } from './locales/fr'

export type Lang = 'en' | 'fr'
export const LANGS: readonly Lang[] = ['en', 'fr']

export type Key = keyof typeof en
type Plural = { one: string; other: string }
/** A dictionary: exactly the keys of en.ts, with the same shape (text or plural). */
export type Dict = { [K in Key]: (typeof en)[K] extends string ? string : Plural }

const DICTS: Record<Lang, Dict> = { en, fr }
const STORAGE = 'td-lang'

function isLang(v: unknown): v is Lang {
  return v === 'en' || v === 'fr'
}

/** First supported language of a preference list ("fr-FR", "en-US"…), or English. */
export function detect(prefs: readonly string[] | undefined): Lang {
  for (const p of prefs ?? []) {
    const code = p.toLowerCase().slice(0, 2)
    if (isLang(code)) return code
  }
  return 'en'
}

function initial(): Lang {
  try {
    const v = localStorage.getItem(STORAGE)
    if (isLang(v)) return v
  } catch {
    // storage unavailable: fall back on the browser languages
  }
  return detect(typeof navigator === 'undefined' ? undefined : navigator.languages)
}

let current = $state<Lang>(initial())

export function lang(): Lang {
  return current
}

/** Changes the language (and remembers it, unless `remember` is false). */
export function setLang(l: Lang, remember = true): void {
  current = l
  if (remember) {
    try {
      localStorage.setItem(STORAGE, l)
    } catch {
      // private browsing, blocked storage: the choice lasts for this page
    }
  }
  if (typeof document !== 'undefined') document.documentElement.lang = l
}

type Params = Record<string, string | number>

function fill(s: string, params?: Params): string {
  if (!params) return s
  return s.replace(/\{(\w+)\}/g, (m, k: string) => (k in params ? String(params[k]) : m))
}

function entry(key: Key): string | Plural {
  return DICTS[current][key] ?? DICTS.en[key]
}

/** Translated text; `{name}` placeholders are replaced by `params`. */
export function t(key: Key, params?: Params): string {
  const e = entry(key)
  return fill(typeof e === 'string' ? e : e.other, params)
}

const plurals = new Map<Lang, Intl.PluralRules>()

/** Plural form for `n`; `{n}` is the count formatted for the language. */
export function tn(key: Key, n: number, params?: Params): string {
  const e = entry(key)
  const p = { n: formatInt(n), ...params }
  if (typeof e === 'string') return fill(e, p)
  let rules = plurals.get(current)
  if (!rules) {
    rules = new Intl.PluralRules(current)
    plurals.set(current, rules)
  }
  return fill(rules.select(n) === 'one' ? e.one : e.other, p)
}

/** Thousands separator of the current language: 12,345 or 12 345 (narrow no-break space). */
export function formatInt(n: number): string {
  if (!Number.isFinite(n)) return '0'
  const sep = current === 'fr' ? ' ' : ','
  return Math.round(n)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, sep)
}
