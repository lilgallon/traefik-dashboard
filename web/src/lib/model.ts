// Ce que la page calcule à partir du résumé et du flux en direct.

import { isRange, type AlertKind, type ClassCounts, type Hit, type IpRow, type Kpis, type LiveTick, type Period, type StatusClass, type Summary, type TimeRange, type Window } from './api'
import { formatDayTime } from './format'

export const WINDOW_SECONDS: Record<Window, number> = { '1h': 3600, '24h': 86400, '7d': 604800 }
export const WINDOW_LABEL: Record<Window, string> = { '1h': '1 h', '24h': '24 h', '7d': '7 j' }
export const WINDOW_PHRASE: Record<Window, string> = {
  '1h': 'sur la dernière heure',
  '24h': 'sur les dernières 24 h',
  '7d': 'sur les 7 derniers jours',
}

const two = (n: number) => String(n).padStart(2, '0')
const hm = (d: Date) => `${two(d.getHours())}:${two(d.getMinutes())}`
const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()

/** « de 14:02 à 14:37 » le même jour, « du 20/11 22:00 au 21/11 03:00 » sinon. */
export function rangeLabel(r: TimeRange): string {
  const a = new Date(r.from * 1000)
  const b = new Date(r.to * 1000)
  if (sameDay(a, new Date(b.getTime() - 1))) return `le ${formatDayTime(a).slice(0, 5)} de ${hm(a)} à ${hm(b)}`
  return `du ${formatDayTime(a)} au ${formatDayTime(b)}`
}

/** « sur la dernière heure », ou « entre … » pour un intervalle. */
export function periodPhrase(p: Period): string {
  return isRange(p) ? rangeLabel(p) : WINDOW_PHRASE[p]
}

/** Clé d'une période, pour savoir si le résumé affiché y correspond. */
export function periodKey(p: Period): string {
  return isRange(p) ? `${p.from}-${p.to}` : p
}

export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** Ordre d'empilement, du bas vers le haut : ce qui va bien en bas, les erreurs en haut. */
export const STACK: { key: StatusClass; label: string; cssVar: string }[] = [
  { key: '2xx', label: '2xx', cssVar: '--c-2xx' },
  { key: '3xx', label: '3xx', cssVar: '--c-3xx' },
  { key: 'abandon', label: 'abandons', cssVar: '--c-abandon' },
  { key: '4xx', label: '4xx', cssVar: '--c-4xx' },
  { key: '5xx', label: '5xx', cssVar: '--c-5xx' },
]

export const STALE_AFTER_MS = 15 * 60 * 1000
const OFFLINE_AFTER_MS = 30 * 1000

export function total(c: ClassCounts): number {
  return c['1xx'] + c['2xx'] + c['3xx'] + c['4xx'] + c['5xx'] + c.abandon
}

export function emptyCounts(): ClassCounts {
  return { '1xx': 0, '2xx': 0, '3xx': 0, '4xx': 0, '5xx': 0, abandon: 0 }
}

export interface IpGroup {
  name: string
  rows: IpRow[]
  byClass: ClassCounts
  bytesIn: number
  bytesOut: number
}

/** Les IP nommées sont regroupées sous leur nom (« Maison » = IPv4 + plusieurs IPv6). */
export function groupIps(rows: IpRow[]): { named: IpGroup[]; unknown: IpRow[] } {
  const groups = new Map<string, IpGroup>()
  const unknown: IpRow[] = []
  for (const r of rows) {
    if (r.verdict !== 'connue' || !r.name) {
      unknown.push(r)
      continue
    }
    let g = groups.get(r.name)
    if (!g) {
      g = { name: r.name, rows: [], byClass: emptyCounts(), bytesIn: 0, bytesOut: 0 }
      groups.set(r.name, g)
    }
    g.rows.push(r)
    for (const k of Object.keys(g.byClass) as StatusClass[]) g.byClass[k] += r.byClass[k] ?? 0
    g.bytesIn += r.bytesIn
    g.bytesOut += r.bytesOut
  }
  const named = [...groups.values()].sort((a, b) => total(b.byClass) - total(a.byClass) || a.name.localeCompare(b.name, 'fr'))
  return { named, unknown }
}

/** Premier tick que le résumé ne contient pas encore. */
function firstNewTick(s: Summary): number {
  if (s.window === '1h' && s.series.t.length > 0) return s.series.t[s.series.t.length - 1] + 1
  return Math.floor(Date.parse(s.generatedAt) / 1000)
}

/** Indicateurs en direct : ceux du résumé, plus les ticks arrivés depuis. */
export function liveKpis(s: Summary, ticks: LiveTick[]): Kpis {
  if (s.window === '') return s.kpis // intervalle figé : rien à ajouter
  const from = firstNewTick(s)
  const byClass = { ...s.kpis.byClass }
  let bytesIn = s.kpis.bytesIn
  let bytesOut = s.kpis.bytesOut
  let peak = s.kpis.peakRps
  for (let i = ticks.length - 1; i >= 0 && ticks[i].t >= from; i--) {
    const t = ticks[i]
    for (const k of Object.keys(byClass) as StatusClass[]) byClass[k] += t.byClass[k] ?? 0
    bytesIn += t.bytesIn
    bytesOut += t.bytesOut
    peak = Math.max(peak, total(t.byClass))
  }
  return { ...s.kpis, byClass, bytesIn, bytesOut, peakRps: peak, avgRps: total(byClass) / WINDOW_SECONDS[s.window] }
}

export interface ChartData {
  t: number[]
  /** Valeurs par famille, dans l'ordre de STACK. */
  raw: { key: StatusClass; values: number[] }[]
  /** Cumul : stacked[i] = somme des familles 0…i. */
  stacked: number[][]
}

function valuesOf(key: StatusClass, c: ClassCounts): number {
  // 1xx : Traefik n'en journalise pas en pratique ; rangé avec les abandons.
  return key === 'abandon' ? c.abandon + c['1xx'] : c[key]
}

/** Données du graphique. Sur 1 h, la série est prolongée seconde par seconde par les ticks. */
export function chartData(s: Summary, ticks: LiveTick[]): ChartData {
  let t = s.series.t
  let raw = STACK.map(({ key }) => ({
    key,
    values: key === 'abandon' ? s.series.byClass.abandon.map((v, i) => v + (s.series.byClass['1xx'][i] ?? 0)) : s.series.byClass[key],
  }))

  if (s.window === '1h' && t.length > 0) {
    const last = t[t.length - 1]
    const fresh = ticks.filter((k) => k.t > last && k.t - last <= 3600)
    if (fresh.length > 0) {
      const end = fresh[fresh.length - 1].t
      const byT = new Map(fresh.map((k) => [k.t, k]))
      t = t.slice()
      raw = raw.map((r) => ({ key: r.key, values: r.values.slice() }))
      for (let x = last + 1; x <= end; x++) {
        t.push(x)
        const k = byT.get(x)
        for (const r of raw) r.values.push(k ? valuesOf(r.key, k.byClass) : 0)
      }
      const extra = t.length - s.series.t.length
      t = t.slice(extra)
      raw = raw.map((r) => ({ key: r.key, values: r.values.slice(extra) }))
    }
  }

  const stacked: number[][] = []
  raw.forEach((r, i) => {
    stacked.push(i === 0 ? r.values.slice() : r.values.map((v, j) => v + stacked[i - 1][j]))
  })
  return { t, raw, stacked }
}

export type DataState = 'loading' | 'backfill' | 'live' | 'stale' | 'offline'

/**
 * Ce que valent les données affichées. Sert à ne jamais montrer un faux « tout va
 * bien » : pendant la relecture, sur un trou de données ou hors ligne, pas de vert.
 */
export function dataState(s: Summary | null, error: string | null, now: Date): DataState {
  if (!s) return error ? 'offline' : 'loading'
  if (error && now.getTime() - Date.parse(s.generatedAt) > OFFLINE_AFTER_MS) return 'offline'
  if (s.loading) return 'backfill'
  const last = Date.parse(s.lastLine)
  if (Number.isNaN(last) || new Date(last).getUTCFullYear() < 2000 || now.getTime() - last > STALE_AFTER_MS) return 'stale'
  return 'live'
}

/** Bloc YAML prêt à coller dans names.yaml. */
export function ruleSnippet(
  d: { ip: string; asn: number; org: string; routers: Record<string, number> },
  kind: 'ip' | 'asn',
): string {
  if (kind === 'ip') {
    const ip = d.ip.includes(':') ? `"${d.ip}"` : d.ip
    return `  - name: À nommer\n    ip: [${ip}]\n`
  }
  const routers = Object.entries(d.routers)
    .filter(([r]) => !r.startsWith('('))
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([r]) => r)
  let out = `  - name: À nommer\n    asn: [${d.asn}]${d.org ? `  # ${d.org}` : ''}\n`
  if (routers.length > 0) out += `    routers: [${routers.join(', ')}]\n`
  return out
}

/** Famille de statut d'une requête du bloc « en direct » : un WebSocket accepté compte comme un succès. */
export function hitClass(h: Hit): StatusClass {
  if (h.socket) return '2xx'
  if (h.status >= 500) return '5xx'
  if (h.status >= 400) return '4xx'
  if (h.status >= 300) return '3xx'
  if (h.status >= 200) return '2xx'
  if (h.status >= 100) return '1xx'
  return 'abandon'
}

/** Ce que la colonne « statut » affiche. */
export function hitStatus(h: Hit): string {
  if (h.socket) return 'WS'
  return h.status > 0 ? String(h.status) : 'abandon'
}

/** Les `limit` requêtes les plus récentes, IP nommées exclues si `unknownOnly`. */
export function visibleHits(hits: Hit[], unknownOnly: boolean, limit: number): Hit[] {
  const out: Hit[] = []
  for (const h of hits) {
    if (unknownOnly && h.name) continue
    out.push(h)
    if (out.length >= limit) break
  }
  return out
}

/** « crowdsecurity/http-probing » → « http-probing » : l'auteur habituel est sous-entendu. */
export function scenarioLabel(s: string): string {
  return s.replace(/^crowdsecurity\//, '')
}

export const ALERT_KIND: Record<AlertKind, { label: string; tone: 'bad' | 'warn' | 'quiet'; title: string }> = {
  ban: { label: 'ban', tone: 'bad', title: 'Scénario déclenché : IP bannie (toutes ses requêtes refusées)' },
  waf: { label: 'WAF', tone: 'warn', title: 'Requête bloquée par le WAF (AppSec), sans ban' },
  alerte: { label: 'signalée', tone: 'quiet', title: 'Scénario déclenché sans décision (IP déjà bannie, par exemple)' },
}
