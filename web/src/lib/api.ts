import { lang, t } from './i18n.svelte'

// Types et appels de l'API du service (mêmes noms de champs que le JSON Go).

export type Window = '1h' | '24h' | '7d'
export const WINDOWS: Window[] = ['1h', '24h', '7d']

/** Intervalle choisi sur le graphique : [from, to[, secondes Unix. */
export interface TimeRange {
  from: number
  to: number
}

/** Ce que la page affiche : une période glissante (en direct) ou un intervalle figé. */
export type Period = Window | TimeRange

export function isRange(p: Period): p is TimeRange {
  return typeof p !== 'string'
}

/** Paramètres de requête de l'API pour une période. */
export function periodQuery(p: Period): string {
  return isRange(p) ? `from=${Math.floor(p.from)}&to=${Math.ceil(p.to)}` : `window=${p}`
}

export type StatusClass = '1xx' | '2xx' | '3xx' | '4xx' | '5xx' | 'abandon'
export type ClassCounts = Record<StatusClass, number>

export type VerdictKind = 'known' | 'success' | 'public' | 'blocked' | 'probing' | 'other'

export interface Offender {
  ip: string
  country: string
  org: string
  asn: number
  looksLike: string
  count: number
  urls: string[]
}

export interface Kpis {
  avgRps: number
  peakRps: number
  bytesIn: number
  bytesOut: number
  byClass: ClassCounts
  /** -1 : CrowdSec injoignable */
  activeBans: number
}

export interface Series {
  stepSeconds: number
  t: number[]
  byClass: Record<StatusClass, number[]>
}

export interface Ban {
  scenario: string
  until: string
  /** Posé à la main (cscli ou cette page). */
  manual: boolean
}

export interface IpRow {
  ip: string
  name: string
  looksLike: string
  country: string
  org: string
  asn: number
  byClass: ClassCounts
  successes: number
  bytesIn: number
  bytesOut: number
  verdict: VerdictKind
  ban: Ban | null
}

export interface UrlRow {
  host: string
  path: string
  byClass: ClassCounts
  /** WebSockets acceptés (journalisés avec le statut 0, donc aussi dans abandon) */
  sockets: number
}

export interface ServiceRow {
  router: string
  requests: number
  err5xx: number
  p95Ms: number
}

export interface SensitiveHit {
  ip: string
  host: string
  path: string
  status: number
  count: number
  last: string
}

/** Alerte CrowdSec locale (scénario, WAF ou ban manuel). */
export type AlertKind = 'ban' | 'waf' | 'alert'
export interface CsAlert {
  id: number
  at: string
  /** IP, ou plage pour une alerte sur une plage. */
  ip: string
  country: string
  org: string
  scenario: string
  kind: AlertKind
  manual: boolean
  /** Règle WAF : « Detect access to .env files ». */
  detail?: string
  host?: string
  paths?: string[]
  /** Chemins visés au-delà des 3 premiers. */
  more?: number
  events: number
}

export interface CsBan {
  ip: string
  country: string
  org: string
  scenario: string
  manual: boolean
  until: string
}

export interface CsScenario {
  scenario: string
  kind: AlertKind
  detail?: string
  alerts: number
  ips: number
  last: string
  hosts: string[]
}

export interface CrowdSec {
  /** Compte machine configuré : alertes lisibles. */
  enabled: boolean
  /** Dernière lecture des alertes en échec. */
  error?: string
  bans: CsBan[]
  blocklist: { community: number; lists: number; at: string } | null
  counts: { bans: number; waf: number; other: number; ips: number; manual: number }
  scenarios: CsScenario[]
  recent: CsAlert[]
}

export interface Summary {
  /** '' pour un intervalle choisi (voir range). */
  window: Window | ''
  /** Intervalle effectif, arrondi aux minutes (aux heures au-delà de 48 h). */
  range?: TimeRange
  generatedAt: string
  lastLine: string
  verdict: { red: boolean; totalIPs?: number; totalSuccesses?: number; offenders: Offender[] }
  kpis: Kpis
  series: Series
  ips: IpRow[]
  urls: UrlRow[]
  services: ServiceRow[]
  sensitive: SensitiveHit[]
  counts: { ips: number; namedIps: number; unknownIps: number; urls: number }
  warnings: string[]
  /** relecture des archives en cours : les chiffres sont incomplets */
  loading: boolean
  crowdsec: CrowdSec | null
}

export interface RawHit {
  time: string
  method: string
  host: string
  path: string
  status: number
}

export interface IpDetail extends IpRow {
  urls: UrlRow[]
  routers: Record<string, number>
  rules: Record<string, number>
  userAgents: string[]
  rawPaths: RawHit[]
  block: {
    /** Compte machine CrowdSec configuré : la page peut bannir. */
    enabled: boolean
    /** Pourquoi cette IP ne peut pas être bannie (IP nommée, privée…). */
    refused?: string
  }
  /** Historique CrowdSec de l'IP (7 jours), le plus récent d'abord. */
  alerts: CsAlert[]
}

export type BanDuration = '4h' | '24h' | '7d' | '30d'
export const BAN_DURATIONS: BanDuration[] = ['4h', '24h', '7d', '30d']

export function banDurationLabel(d: BanDuration): string {
  return t(`duration.${d}`)
}

/** Une des dernières requêtes lues (bloc « en direct »). */
export interface Hit {
  /** Croissant ; repart de 1 quand le service redémarre. */
  seq: number
  time: string
  ip: string
  name: string
  country: string
  method: string
  host: string
  path: string
  status: number
  ms: number
  /** WebSocket accepté, journalisé sans statut. */
  socket: boolean
  /** Succès d'une IP inconnue hors URL anonymes. */
  red: boolean
}

export interface LiveTick {
  t: number
  byClass: ClassCounts
  bytesIn: number
  bytesOut: number
  lastLine: string
  red: Record<Window, boolean>
  loading: boolean
  /** Requêtes lues depuis le tick précédent. */
  hits?: Hit[]
}

/** En-têtes de lecture : la langue choisie dans la page, pour les messages du service. */
function readHeaders(): Record<string, string> {
  return { Accept: 'application/json', 'Accept-Language': lang() }
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { headers: readHeaders(), cache: 'no-store' })
  if (!res.ok) throw new Error(t('api.httpError', { status: res.status }))
  return (await res.json()) as T
}

export function fetchSummary(p: Period): Promise<Summary> {
  return getJson<Summary>(`/api/summary?${periodQuery(p)}`)
}

/** null si l'IP n'a aucune requête sur la période. */
export async function fetchIp(ip: string, p: Period): Promise<IpDetail | null> {
  const res = await fetch(`/api/ip/${encodeURIComponent(ip)}?${periodQuery(p)}`, {
    headers: readHeaders(),
    cache: 'no-store',
  })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(t('api.httpError', { status: res.status }))
  return (await res.json()) as IpDetail
}

/** Écriture (ban, levée) : l'en-tête dédié prouve que la requête vient de la page. */
async function write(method: 'POST' | 'DELETE', url: string, body?: unknown): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: { ...readHeaders(), 'Content-Type': 'application/json', 'X-Traefik-Dashboard': '1' },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  })
  if (!res.ok) {
    let msg = t('api.httpError', { status: res.status })
    try {
      const j = (await res.json()) as { error?: string }
      if (j.error) msg = j.error
    } catch {
      // corps illisible : message générique
    }
    throw new Error(msg)
  }
}

export function banIp(ip: string, duration: BanDuration, reason: string): Promise<void> {
  return write('POST', `/api/ip/${encodeURIComponent(ip)}/ban`, { duration, reason })
}

export function unbanIp(ip: string): Promise<void> {
  return write('DELETE', `/api/ip/${encodeURIComponent(ip)}/ban`)
}
