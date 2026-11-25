import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { periodQuery, type ClassCounts, type Hit, type IpRow, type LiveTick, type Summary } from './api'
import { chartData, dataState, groupIps, hitClass, hitStatus, visibleHits, liveKpis, periodKey, periodPhrase, rangeLabel, ruleSnippet, scenarioLabel, total } from './model'
import { setLang } from './i18n.svelte'

// Les libellés attendus ici sont en français.
beforeAll(() => setLang('fr', false))
afterAll(() => setLang('en', false))

const counts = (c: Partial<ClassCounts> = {}): ClassCounts => ({ '1xx': 0, '2xx': 0, '3xx': 0, '4xx': 0, '5xx': 0, abandon: 0, ...c })

const row = (ip: string, name: string, c: Partial<ClassCounts>, extra: Partial<IpRow> = {}): IpRow => ({
  ip, name, looksLike: '', country: 'FR', org: 'Free', asn: 1, byClass: counts(c), successes: 0,
  bytesIn: 10, bytesOut: 100, verdict: name ? 'known' : 'blocked', ban: null, ...extra,
})

const tick = (t: number, c: Partial<ClassCounts> = {}): LiveTick => ({
  t, byClass: counts(c), bytesIn: 5, bytesOut: 50, lastLine: '', red: { '1h': false, '24h': false, '7d': false }, loading: false,
})

function summary(over: Partial<Summary> = {}): Summary {
  return {
    window: '24h', generatedAt: '2026-11-10T12:00:00.4Z', lastLine: '2026-11-10T11:59:58Z',
    verdict: { red: false, offenders: [] },
    kpis: { avgRps: 1, peakRps: 3, bytesIn: 1000, bytesOut: 2000, byClass: counts({ '2xx': 86400 }), activeBans: 0 },
    series: { stepSeconds: 60, t: [0, 60], byClass: { '1xx': [0, 0], '2xx': [1, 2], '3xx': [0, 0], '4xx': [0, 1], '5xx': [0, 0], abandon: [0, 0] } },
    ips: [], urls: [], services: [], sensitive: [], counts: { ips: 0, namedIps: 0, unknownIps: 0, urls: 0 },
    warnings: [], loading: false, crowdsec: null, ...over,
  }
}

describe('groupIps', () => {
  it('regroupe les IP nommées par nom, les inconnues restent à part', () => {
    const { named, unknown } = groupIps([
      row('203.0.113.10', 'Maison', { '2xx': 100 }),
      row('2a01::1', 'Maison', { '2xx': 5, '4xx': 1 }),
      row('34.1.1.1', 'Partenaire GCP 2', { '2xx': 1000 }),
      row('198.51.100.1', '', { '4xx': 3 }),
    ])
    expect(named.map((g) => g.name)).toEqual(['Partenaire GCP 2', 'Maison'])
    const maison = named[1]
    expect(maison.rows.length).toBe(2)
    expect(maison.byClass['2xx']).toBe(105)
    expect(maison.byClass['4xx']).toBe(1)
    expect(maison.bytesOut).toBe(200)
    expect(unknown.map((r) => r.ip)).toEqual(['198.51.100.1'])
  })
  it('accepte une liste vide', () => {
    expect(groupIps([])).toEqual({ named: [], unknown: [] })
  })
})

describe('liveKpis', () => {
  it('ajoute les ticks postérieurs au résumé', () => {
    const s = summary()
    const g = Date.parse(s.generatedAt) / 1000
    const k = liveKpis(s, [tick(Math.floor(g) - 5, { '2xx': 9 }), tick(Math.floor(g), { '2xx': 2 }), tick(Math.floor(g) + 1, { '4xx': 1 })])
    expect(k.byClass['2xx']).toBe(86402)
    expect(k.byClass['4xx']).toBe(1)
    expect(k.bytesIn).toBe(1010)
    expect(k.bytesOut).toBe(2100)
    expect(k.avgRps).toBeCloseTo(86403 / 86400, 6)
    expect(k.peakRps).toBe(3)
  })
  it('relève le pic si une seconde en direct le dépasse', () => {
    const s = summary()
    const g = Math.floor(Date.parse(s.generatedAt) / 1000)
    expect(liveKpis(s, [tick(g + 1, { '2xx': 7, '4xx': 2 })]).peakRps).toBe(9)
  })
  it('sur 1 h, part de la fin de la série', () => {
    const s = summary({ window: '1h', series: { stepSeconds: 1, t: [100, 101], byClass: { '1xx': [0, 0], '2xx': [1, 1], '3xx': [0, 0], '4xx': [0, 0], '5xx': [0, 0], abandon: [0, 0] } } })
    const k = liveKpis(s, [tick(101, { '2xx': 5 }), tick(102, { '2xx': 1 })])
    expect(k.byClass['2xx']).toBe(86401)
  })
})

describe('chartData', () => {
  it('empile dans l’ordre 2xx, 3xx, abandons, 4xx, 5xx', () => {
    const d = chartData(summary(), [])
    expect(d.t).toEqual([0, 60])
    expect(d.raw.map((r) => r.key)).toEqual(['2xx', '3xx', 'abandon', '4xx', '5xx'])
    // cumul : dernière série = total
    expect(d.stacked[4]).toEqual([1, 3])
    expect(d.stacked[0]).toEqual([1, 2])
  })
  it('sur 1 h, prolonge la série avec les ticks et garde 3 600 points', () => {
    const t = Array.from({ length: 3600 }, (_, i) => 1000 + i)
    const zeros = t.map(() => 0)
    const s = summary({ window: '1h', series: { stepSeconds: 1, t, byClass: { '1xx': zeros, '2xx': t.map(() => 1), '3xx': zeros, '4xx': zeros, '5xx': zeros, abandon: zeros } } })
    const d = chartData(s, [tick(4599, { '2xx': 9 }), tick(4600, { '2xx': 4, '1xx': 1 }), tick(4602, { '5xx': 2 })])
    expect(d.t.length).toBe(3600)
    expect(d.t.at(-1)).toBe(4602)
    expect(d.t[0]).toBe(1003)
    // tick 4599 déjà dans la série : ignoré ; 4601 manquant : comblé par zéro
    expect(d.raw[0].values.slice(-3)).toEqual([4, 0, 0])
    expect(d.raw[4].values.at(-1)).toBe(2)
    // 1xx (jamais journalisé en pratique) rangé avec les abandons
    expect(d.raw[2].values.at(-3)).toBe(1)
  })
  it('ignore les ticks hors 1 h', () => {
    const d = chartData(summary(), [tick(999999, { '2xx': 5 })])
    expect(d.t).toEqual([0, 60])
  })
})

describe('dataState', () => {
  const now = new Date('2026-11-10T12:00:00Z')
  it('distingue chargement, relecture, retard et direct', () => {
    expect(dataState(null, null, now)).toBe('loading')
    expect(dataState(summary({ loading: true }), null, now)).toBe('backfill')
    expect(dataState(summary(), null, now)).toBe('live')
    expect(dataState(summary({ lastLine: '2026-11-10T11:40:00Z' }), null, now)).toBe('stale')
    expect(dataState(summary({ lastLine: '0001-01-01T00:00:00Z' }), null, now)).toBe('stale')
  })
  it('signale un tableau de bord injoignable', () => {
    expect(dataState(null, 'Failed to fetch', now)).toBe('offline')
    // un résumé trop vieux ne vaut plus rien
    expect(dataState(summary({ generatedAt: '2026-11-10T11:58:00Z' }), 'Failed to fetch', now)).toBe('offline')
    // une erreur passagère ne fait pas clignoter la page
    expect(dataState(summary({ generatedAt: '2026-11-10T11:59:50Z' }), 'Failed to fetch', now)).toBe('live')
  })
})

describe('ruleSnippet', () => {
  it('propose une règle par IP', () => {
    expect(ruleSnippet({ ip: '198.51.100.1', asn: 7738, org: 'V tal', routers: { homeassistant: 3 } }, 'ip')).toBe(
      '  - name: À nommer\n    ip: [198.51.100.1]\n',
    )
  })
  it('met une IPv6 entre guillemets', () => {
    expect(ruleSnippet({ ip: '2001:db8:e487::1', asn: 0, org: '', routers: {} }, 'ip')).toContain('ip: ["2001:db8:e487::1"]')
  })
  it('propose une règle par opérateur, restreinte aux routers vus', () => {
    expect(ruleSnippet({ ip: '198.51.100.1', asn: 7738, org: 'V tal', routers: { gotify: 1, homeassistant: 3, '(none)': 2 } }, 'asn')).toBe(
      '  - name: À nommer\n    asn: [7738]  # V tal\n    routers: [homeassistant, gotify]\n',
    )
  })
})

describe('total', () => {
  it('somme les familles', () => {
    expect(total(counts({ '2xx': 2, abandon: 1 }))).toBe(3)
  })
})

describe('requêtes en direct', () => {
  const hit = (seq: number, over: Partial<Hit> = {}): Hit => ({
    seq, time: '2026-11-10T12:00:00Z', ip: '198.51.100.1', name: '', country: 'FR', method: 'GET',
    host: 'home.example.com', path: '/', status: 200, ms: 3, socket: false, red: false, ...over,
  })

  it('classe par statut, WebSocket accepté compris', () => {
    expect(hitClass(hit(1))).toBe('2xx')
    expect(hitClass(hit(1, { status: 302 }))).toBe('3xx')
    expect(hitClass(hit(1, { status: 403 }))).toBe('4xx')
    expect(hitClass(hit(1, { status: 502 }))).toBe('5xx')
    expect(hitClass(hit(1, { status: 0 }))).toBe('abandon')
    expect(hitClass(hit(1, { status: 0, socket: true }))).toBe('2xx')
    expect(hitStatus(hit(1, { status: 404 }))).toBe('404')
    expect(hitStatus(hit(1, { status: 0 }))).toBe('abandon')
    expect(hitStatus(hit(1, { status: 0, socket: true }))).toBe('WS')
  })

  it('filtre les inconnues et limite le nombre de lignes', () => {
    const hits = [hit(5, { name: 'Maison' }), hit(4), hit(3, { name: 'Travail 1' }), hit(2), hit(1)]
    expect(visibleHits(hits, false, 3).map((h) => h.seq)).toEqual([5, 4, 3])
    expect(visibleHits(hits, true, 2).map((h) => h.seq)).toEqual([4, 2])
    expect(visibleHits(hits, true, 10).map((h) => h.seq)).toEqual([4, 2, 1])
    expect(visibleHits([], true, 10)).toEqual([])
  })
})

describe('scenarioLabel', () => {
  it('retire l’auteur habituel', () => {
    expect(scenarioLabel('crowdsecurity/http-probing')).toBe('http-probing')
    expect(scenarioLabel('LePresidente/http-generic-403-bf')).toBe('LePresidente/http-generic-403-bf')
    expect(scenarioLabel('manuel (tableau de bord) : scan')).toBe('manuel (tableau de bord) : scan')
  })
})

describe('périodes', () => {
  it('distingue fenêtre et intervalle', () => {
    expect(periodKey('24h')).toBe('24h')
    expect(periodKey({ from: 10, to: 20 })).toBe('10-20')
    expect(periodQuery('1h')).toBe('window=1h')
    expect(periodQuery({ from: 10.4, to: 19.2 })).toBe('from=10&to=20')
  })
  it('décrit un intervalle', () => {
    const a = new Date(2026, 10, 21, 14, 2).getTime() / 1000
    expect(rangeLabel({ from: a, to: a + 35 * 60 })).toBe('le 21/11 de 14:02 à 14:37')
    // Jusqu'à minuit pile : toujours le même jour.
    const m = new Date(2026, 10, 21, 23, 0).getTime() / 1000
    expect(rangeLabel({ from: m, to: m + 3600 })).toBe('le 21/11 de 23:00 à 00:00')
    expect(rangeLabel({ from: m, to: m + 4 * 3600 })).toBe('du 21/11 23:00 au 22/11 03:00')
    expect(periodPhrase('7d')).toBe('sur les 7 derniers jours')
  })
})
