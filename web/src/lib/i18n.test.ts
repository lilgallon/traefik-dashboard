import { afterEach, describe, expect, it } from 'vitest'
import { detect, setLang, t, tn } from './i18n.svelte'
import { en } from './locales/en'
import { fr } from './locales/fr'

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()
const forms = (e: string | { one: string; other: string }) => (typeof e === 'string' ? [e] : [e.one, e.other])

afterEach(() => setLang('en', false))

describe('dictionaries', () => {
  it('have the same keys, shapes and placeholders', () => {
    expect(Object.keys(fr).sort()).toEqual(Object.keys(en).sort())
    for (const k of Object.keys(en) as (keyof typeof en)[]) {
      const a = forms(en[k])
      const b = forms(fr[k])
      expect(b.length, k).toBe(a.length)
      a.forEach((s, i) => expect(placeholders(b[i]), k).toEqual(placeholders(s)))
      b.forEach((s) => expect(s.trim(), k).not.toBe(''))
    }
  })
})

describe('t / tn', () => {
  it('fills placeholders and follows the language', () => {
    expect(t('feed.noUnknown', { n: 12 })).toBe('No unknown IP among the last 12 requests.')
    setLang('fr', false)
    expect(t('feed.noUnknown', { n: 12 })).toBe('Aucune IP inconnue parmi les 12 dernières requêtes.')
  })
  it('picks the plural form of each language', () => {
    expect(tn('kpi.bansSub', 0)).toBe('current bans')
    expect(tn('kpi.bansSub', 1)).toBe('current ban')
    setLang('fr', false)
    expect(tn('kpi.bansSub', 0)).toBe('ban en cours')
    expect(tn('kpi.bansSub', 2)).toBe('bans en cours')
  })
  it('detects the browser language, English by default', () => {
    expect(detect(['fr-FR', 'en'])).toBe('fr')
    expect(detect(['de-DE', 'fr'])).toBe('fr')
    expect(detect(['de-DE'])).toBe('en')
    expect(detect(undefined)).toBe('en')
  })
})
