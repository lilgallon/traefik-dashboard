// Numbers, sizes, durations and dates, in the interface language.

import { formatInt, lang, t } from './i18n.svelte'

/** Narrow no-break space: between a number and its unit. */
const NNBSP = ' '

function decimal(n: number, digits: number): string {
  const s = n.toFixed(digits).replace(/\.?0+$/, '')
  return lang() === 'fr' ? s.replace('.', ',') : s
}

export function formatCount(n: number): string {
  return formatInt(n)
}

const UNITS = { en: ['B', 'kB', 'MB', 'GB', 'TB'], fr: ['o', 'Ko', 'Mo', 'Go', 'To'] }

/** Base 1000: 1536 → "1.5 kB" (en), "1,5 Ko" (fr). */
export function formatBytes(n: number): string {
  const units = UNITS[lang()]
  if (!Number.isFinite(n) || n <= 0) return `0${NNBSP}${units[0]}`
  let v = n
  let i = 0
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000
    i++
  }
  const text = i === 0 || v >= 100 ? Math.round(v).toString() : decimal(v, 1)
  return `${text}${NNBSP}${units[i]}`
}

/** Requests per second: 1.3 · 38 · 0.04. */
export function formatRps(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0'
  if (n < 0.01) return `< ${decimal(0.01, 2)}`
  if (n < 0.1) return decimal(n, 2)
  if (n < 10) return decimal(n, 1)
  return formatCount(n)
}

export function formatMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  if (ms < 1) return `< 1${NNBSP}ms`
  if (ms < 1000) return `${Math.round(ms)}${NNBSP}ms`
  return `${decimal(ms / 1000, 1)}${NNBSP}s`
}

/** Duration of a request: like formatMs, then "4 min", "2 h 05" for a long connection. */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 60_000) return formatMs(ms)
  const min = Math.floor(ms / 60_000)
  if (min < 60) return `${min}${NNBSP}min`
  return `${Math.floor(min / 60)}${NNBSP}h${NNBSP}${String(min % 60).padStart(2, '0')}`
}

function validDate(t: string): number | null {
  const ms = Date.parse(t)
  if (!t || Number.isNaN(ms) || new Date(ms).getUTCFullYear() < 2000) return null
  return ms
}

/** "12 s ago", "3 min ago". Missing or invalid date: "never". */
export function ago(when: string, now: Date): string {
  const ms = validDate(when)
  if (ms === null) return t('time.never')
  const s = Math.floor((now.getTime() - ms) / 1000)
  if (s < 2) return t('time.justNow')
  if (s < 60) return t('time.agoS', { n: s })
  if (s < 3600) return t('time.agoMin', { n: Math.floor(s / 60) })
  if (s < 86400) {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return m === 0 ? t('time.agoH', { h }) : t('time.agoHM', { h, m: String(m).padStart(2, '0') })
  }
  return t('time.agoD', { n: Math.floor(s / 86400) })
}

/** Time left: "in 3 h 05 min", "in 12 min". Past: "expired"; missing: "—". */
export function remaining(when: string, now: Date): string {
  const ms = validDate(when)
  if (ms === null) return '—'
  const s = Math.floor((ms - now.getTime()) / 1000)
  if (s <= 0) return t('time.expired')
  if (s < 60) return t('time.inS', { n: s })
  if (s < 3600) return t('time.inMin', { n: Math.floor(s / 60) })
  if (s < 2 * 86400) {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return m === 0 ? t('time.inH', { h }) : t('time.inHM', { h, m: String(m).padStart(2, '0') })
  }
  return t('time.inD', { n: Math.floor(s / 86400) })
}

const two = (n: number) => String(n).padStart(2, '0')

/** Browser local time, HH:MM:SS. */
export function formatClock(d: Date): string {
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`
}

/** Local time, HH:MM. */
export function formatHm(d: Date): string {
  return `${two(d.getHours())}:${two(d.getMinutes())}`
}

const months = new Map<string, Intl.DateTimeFormat>()

/** Local day: "Nov 10" (en), "10/11" (fr). */
export function formatDay(d: Date): string {
  if (lang() === 'fr') return `${two(d.getDate())}/${two(d.getMonth() + 1)}`
  let f = months.get('en')
  if (!f) {
    f = new Intl.DateTimeFormat('en', { month: 'short' })
    months.set('en', f)
  }
  return `${f.format(d)} ${d.getDate()}`
}

/** Local day and time: "Nov 10 09:05" (en), "10/11 09:05" (fr). */
export function formatDayTime(d: Date): string {
  return `${formatDay(d)} ${formatHm(d)}`
}

const regions = new Map<string, Intl.DisplayNames>()

/** "FR" → "France". Unknown code: returned as is. */
export function countryName(code: string): string {
  if (!code) return ''
  try {
    const l = lang()
    let r = regions.get(l)
    if (!r) {
      r = new Intl.DisplayNames([l], { type: 'region' })
      regions.set(l, r)
    }
    return r.of(code) ?? code
  } catch {
    return code
  }
}

export function percent(part: number, whole: number): string {
  const sign = lang() === 'fr' ? `${NNBSP}%` : '%'
  if (whole <= 0 || part <= 0) return `0${sign}`
  const p = (part / whole) * 100
  if (p < 0.1) return `< ${decimal(0.1, 1)}${sign}`
  return `${decimal(p, p < 10 ? 1 : 0)}${sign}`
}
