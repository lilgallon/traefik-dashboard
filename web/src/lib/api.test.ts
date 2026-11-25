import { afterEach, describe, expect, it, vi } from 'vitest'
import { banIp, fetchIp, fetchSummary, unbanIp } from './api'

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn(async (_url: string) => new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => vi.unstubAllGlobals())

describe('fetchSummary', () => {
  it('demande la période et renvoie le résumé', async () => {
    const fn = mockFetch(200, { window: '7d', verdict: { red: false, offenders: [] } })
    const s = await fetchSummary('7d')
    expect(s.window).toBe('7d')
    expect(fn.mock.calls[0][0]).toBe('/api/summary?window=7d')
  })
  it('lève une erreur lisible sur une réponse en erreur', async () => {
    mockFetch(503, { error: 'indisponible' })
    await expect(fetchSummary('24h')).rejects.toThrow(/503/)
  })
  it('laisse passer une erreur réseau', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    await expect(fetchSummary('24h')).rejects.toThrow('Failed to fetch')
  })
})

describe('fetchIp', () => {
  it('renvoie le détail', async () => {
    const fn = mockFetch(200, { ip: '2001:db8::1', verdict: 'succès' })
    const d = await fetchIp('2001:db8::1', '1h')
    expect(d?.ip).toBe('2001:db8::1')
    expect(fn.mock.calls[0][0]).toBe('/api/ip/2001%3Adb8%3A%3A1?window=1h')
  })
  it('renvoie null sur 404 (IP sortie de la période)', async () => {
    mockFetch(404, { error: 'aucune requête' })
    expect(await fetchIp('198.51.100.1', '1h')).toBeNull()
  })
  it('lève une erreur sur les autres statuts', async () => {
    mockFetch(500, {})
    await expect(fetchIp('198.51.100.1', '1h')).rejects.toThrow(/500/)
  })
  it('laisse passer une erreur réseau', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('réseau') }))
    await expect(fetchIp('198.51.100.1', '1h')).rejects.toThrow('réseau')
  })
})

describe('banIp / unbanIp', () => {
  it('pose un ban avec l’en-tête de la page', async () => {
    const fn = vi.fn(async (_url: string, _init?: RequestInit) => new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fn)
    await banIp('198.51.100.1', '7d', 'scan')
    const [url, init] = fn.mock.calls[0]
    expect(url).toBe('/api/ip/198.51.100.1/ban')
    expect(init?.method).toBe('POST')
    expect((init?.headers as Record<string, string>)['X-Traefik-Dashboard']).toBe('1')
    expect(JSON.parse(init?.body as string)).toEqual({ duration: '7d', reason: 'scan' })
  })
  it('lève le ban', async () => {
    const fn = vi.fn(async (_url: string, _init?: RequestInit) => new Response('{"deleted":1}', { status: 200 }))
    vi.stubGlobal('fetch', fn)
    await unbanIp('2001:db8::1')
    expect(fn.mock.calls[0][0]).toBe('/api/ip/2001%3Adb8%3A%3A1/ban')
    expect(fn.mock.calls[0][1]?.method).toBe('DELETE')
  })
  it('remonte le message d’erreur du service', async () => {
    mockFetch(409, { error: 'IP nommée « Maison »' })
    await expect(banIp('203.0.113.10', '4h', '')).rejects.toThrow('IP nommée « Maison »')
  })
})
