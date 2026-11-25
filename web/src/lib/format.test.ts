import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { ago, countryName, formatBytes, formatClock, formatCount, formatDayTime, formatDuration, formatMs, formatRps, percent, remaining } from './format'
import { setLang, tn } from './i18n.svelte'

// Ces tests décrivent le rendu français ; l'anglais a son bloc à la fin.
beforeAll(() => setLang('fr', false))
afterAll(() => setLang('en', false))

// Espace fine insécable : séparateur de milliers et espace avant l'unité.
const S = ' '

describe('formatBytes', () => {
  it('suit les unités françaises, en base 1000', () => {
    expect(formatBytes(0)).toBe(`0${S}o`)
    expect(formatBytes(999)).toBe(`999${S}o`)
    expect(formatBytes(1536)).toBe(`1,5${S}Ko`)
    expect(formatBytes(1.2e9)).toBe(`1,2${S}Go`)
    expect(formatBytes(125_067_560)).toBe(`125${S}Mo`)
    expect(formatBytes(22_337_540)).toBe(`22,3${S}Mo`)
    expect(formatBytes(3.4e12)).toBe(`3,4${S}To`)
  })
  it('ne plante pas sur une valeur absurde', () => {
    expect(formatBytes(-5)).toBe(`0${S}o`)
    expect(formatBytes(NaN)).toBe(`0${S}o`)
  })
})

describe('formatCount', () => {
  it('sépare les milliers', () => {
    expect(formatCount(110742)).toBe(`110${S}742`)
    expect(formatCount(0)).toBe('0')
    expect(formatCount(999)).toBe('999')
    expect(formatCount(1234567)).toBe(`1${S}234${S}567`)
  })
})

describe('formatRps', () => {
  it('garde une décimale, virgule française', () => {
    expect(formatRps(1.26)).toBe('1,3')
    expect(formatRps(0)).toBe('0')
    expect(formatRps(38)).toBe('38')
    expect(formatRps(12.04)).toBe('12')
  })
  it('ne réduit pas un petit débit à zéro', () => {
    expect(formatRps(0.04)).toBe('0,04')
    expect(formatRps(0.004)).toBe('< 0,01')
  })
})

describe('formatMs', () => {
  it('choisit ms ou s', () => {
    expect(formatMs(0)).toBe('—')
    expect(formatMs(0.4)).toBe(`< 1${S}ms`)
    expect(formatMs(17.38)).toBe(`17${S}ms`)
    expect(formatMs(2085.2)).toBe(`2,1${S}s`)
  })
})

describe('formatDuration', () => {
  it('passe aux minutes et aux heures pour une connexion longue', () => {
    expect(formatDuration(17.38)).toBe(`17${S}ms`)
    expect(formatDuration(59_000)).toBe(`59${S}s`)
    expect(formatDuration(4 * 60_000 + 30_000)).toBe(`4${S}min`)
    expect(formatDuration(2 * 3_600_000 + 5 * 60_000)).toBe(`2${S}h${S}05`)
    expect(formatDuration(0)).toBe('—')
  })
})

describe('ago', () => {
  const now = new Date('2026-11-10T12:00:00Z')
  it('parle français', () => {
    expect(ago('2026-11-10T11:59:59.500Z', now)).toBe("à l'instant")
    expect(ago('2026-11-10T11:59:48Z', now)).toBe('il y a 12 s')
    expect(ago('2026-11-10T11:57:00Z', now)).toBe('il y a 3 min')
    expect(ago('2026-11-10T09:30:00Z', now)).toBe('il y a 2 h 30')
    expect(ago('2026-11-10T10:00:00Z', now)).toBe('il y a 2 h')
    expect(ago('2026-11-07T12:00:00Z', now)).toBe('il y a 3 j')
  })
  it('tolère une date absente, invalide ou future', () => {
    expect(ago('', now)).toBe('jamais')
    expect(ago('0001-01-01T00:00:00Z', now)).toBe('jamais')
    expect(ago('n/a', now)).toBe('jamais')
    expect(ago('2026-11-10T12:00:03Z', now)).toBe("à l'instant")
  })
})

describe('divers', () => {
  it('nomme les pays', () => {
    expect(countryName('FR')).toBe('France')
    expect(countryName('BR')).toBe('Brésil')
    expect(countryName('')).toBe('')
    expect(countryName('ZZZ')).toBe('ZZZ')
  })
  it('accorde', () => {
    expect(tn('ips.unknownCount', 0)).toBe('0 inconnue')
    expect(tn('ips.unknownCount', 1)).toBe('1 inconnue')
    expect(tn('ips.unknownCount', 2)).toBe('2 inconnues')
    expect(tn('common.requests', 1500)).toBe('1\u202f500 requêtes')
  })
  it("affiche l'heure locale sur deux chiffres", () => {
    expect(formatClock(new Date(2026, 10, 10, 9, 5, 7))).toBe('09:05:07')
  })
})

describe('remaining', () => {
  const now = new Date('2026-11-11T12:00:00Z')
  it('formate le temps restant', () => {
    expect(remaining('2026-11-11T12:00:30Z', now)).toBe('dans 30 s')
    expect(remaining('2026-11-11T12:12:00Z', now)).toBe('dans 12 min')
    expect(remaining('2026-11-11T15:05:00Z', now)).toBe('dans 3 h 05')
    expect(remaining('2026-11-11T16:00:00Z', now)).toBe('dans 4 h')
    expect(remaining('2026-11-18T12:00:00Z', now)).toBe('dans 7 j')
  })
  it('signale un ban expiré ou sans fin connue', () => {
    expect(remaining('2026-11-11T11:00:00Z', now)).toBe('expiré')
    expect(remaining('0001-01-01T00:00:00Z', now)).toBe('—')
    expect(remaining('', now)).toBe('—')
  })
})

describe('en anglais', () => {
  beforeAll(() => setLang('en', false))
  afterAll(() => setLang('fr', false))
  it('formate nombres, volumes et pourcentages', () => {
    expect(formatCount(110742)).toBe('110,742')
    expect(formatBytes(1536)).toBe('1.5\u202fkB')
    expect(formatBytes(0)).toBe('0\u202fB')
    expect(formatRps(1.26)).toBe('1.3')
    expect(formatMs(2100)).toBe('2.1\u202fs')
    expect(percent(1, 3)).toBe('33%')
  })
  it('formate les dates relatives', () => {
    const now = new Date('2026-11-10T12:00:00Z')
    expect(ago('2026-11-10T11:59:59Z', now)).toBe('just now')
    expect(ago('2026-11-10T11:57:00Z', now)).toBe('3 min ago')
    expect(ago('2026-11-10T09:55:00Z', now)).toBe('2 h 05 min ago')
    expect(ago('', now)).toBe('never')
    expect(remaining('2026-11-10T12:00:30Z', now)).toBe('in 30 s')
    expect(remaining('2026-11-10T11:00:00Z', now)).toBe('expired')
  })
  it('nomme les pays et les jours', () => {
    expect(countryName('BR')).toBe('Brazil')
    expect(formatDayTime(new Date(2026, 10, 10, 9, 5))).toBe('Nov 10 09:05')
  })
  it('accorde', () => {
    expect(tn('common.requests', 1)).toBe('1 request')
    expect(tn('common.requests', 0)).toBe('0 requests')
    expect(tn('ips.addresses', 2)).toBe('2 addresses')
  })
})
