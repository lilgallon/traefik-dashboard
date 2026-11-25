import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Hit, LiveTick } from './api'
import { createLive } from './live.svelte'

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static CLOSED = 2
  readyState = 0
  closed = false
  listeners = new Map<string, ((ev: { data: string }) => void)[]>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, fn: (ev: { data: string }) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  close() {
    this.closed = true
    this.readyState = 2
  }
  open() {
    this.readyState = 1
    this.onopen?.()
  }
  emit(type: string, data: unknown) {
    for (const fn of this.listeners.get(type) ?? []) fn({ data: typeof data === 'string' ? data : JSON.stringify(data) })
  }
  fail(closed = false) {
    if (closed) this.readyState = 2
    this.onerror?.()
  }
}

const tick = (t: number): LiveTick => ({
  t,
  byClass: { '1xx': 0, '2xx': 1, '3xx': 0, '4xx': 0, '5xx': 0, abandon: 0 },
  bytesIn: 1,
  bytesOut: 2,
  lastLine: '2026-11-10T12:00:00Z',
  red: { '1h': false, '24h': false, '7d': false },
  loading: false,
})

const hit = (seq: number): Hit => ({
  seq,
  time: '2026-11-10T12:00:00Z',
  ip: '198.51.100.1',
  name: '',
  country: 'FR',
  method: 'GET',
  host: 'home.example.com',
  path: '/',
  status: 200,
  ms: 3,
  socket: false,
  red: false,
})

beforeEach(() => {
  FakeEventSource.instances = []
  vi.useFakeTimers()
})
afterEach(() => vi.useRealTimers())

const impl = FakeEventSource as unknown as typeof EventSource

describe('createLive', () => {
  it('accumule les ticks reçus', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    expect(es.url).toBe('/api/stream')
    expect(live.connected).toBe(false)
    es.open()
    expect(live.connected).toBe(true)
    es.emit('tick', tick(1))
    es.emit('tick', tick(2))
    es.emit('tick', tick(3))
    expect(live.ticks.length).toBe(3)
    expect(live.ticks.map((t) => t.t)).toEqual([1, 2, 3])
    expect(live.last?.t).toBe(3)
    live.close()
    expect(es.closed).toBe(true)
  })

  it('plafonne à 3 600 ticks', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    es.open()
    for (let i = 1; i <= 3700; i++) es.emit('tick', tick(i))
    expect(live.ticks.length).toBe(3600)
    expect(live.ticks[0].t).toBe(101)
    expect(live.ticks.at(-1)?.t).toBe(3700)
    live.close()
  })

  it('ignore un tick illisible ou en double', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    es.open()
    es.emit('tick', '{pas du json')
    es.emit('tick', tick(5))
    es.emit('tick', tick(5))
    es.emit('tick', tick(4))
    expect(live.ticks.map((t) => t.t)).toEqual([5])
    live.close()
  })

  it('passe à déconnecté sur erreur, puis se reconnecte', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    es.open()
    es.fail() // le navigateur réessaie seul
    expect(live.connected).toBe(false)
    es.open()
    expect(live.connected).toBe(true)

    es.fail(true) // connexion fermée pour de bon : on en recrée une
    expect(live.connected).toBe(false)
    vi.advanceTimersByTime(3000)
    expect(FakeEventSource.instances.length).toBe(2)
    FakeEventSource.instances[1].open()
    expect(live.connected).toBe(true)
    live.close()
  })

  it("ne se reconnecte pas après close(), et vit sans EventSource", () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    FakeEventSource.instances[0].fail(true)
    live.close()
    vi.advanceTimersByTime(10000)
    expect(FakeEventSource.instances.length).toBe(1)

    const none = createLive('/api/stream', { EventSourceImpl: undefined })
    expect(none.connected).toBe(false)
    expect(none.ticks).toEqual([])
    none.close()
  })

  it('garde les dernières requêtes, la plus récente en tête', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    es.open()
    expect(live.hits).toEqual([])
    es.emit('hits', [hit(1), hit(2), hit(3)])
    expect(live.hits.map((h) => h.seq)).toEqual([3, 2, 1])
    // Un tick apporte les suivantes ; celles déjà connues sont écartées.
    es.emit('tick', { ...tick(1), hits: [hit(3), hit(4), hit(5)] })
    expect(live.hits.map((h) => h.seq)).toEqual([5, 4, 3, 2, 1])
    es.emit('tick', tick(2))
    es.emit('tick', { ...tick(3), hits: 'nimporte quoi' })
    expect(live.hits.length).toBe(5)
    live.close()
  })

  it('plafonne à 200 requêtes et repart de l\'instantané à la reconnexion', () => {
    const live = createLive('/api/stream', { EventSourceImpl: impl })
    const es = FakeEventSource.instances[0]
    es.open()
    es.emit('hits', Array.from({ length: 190 }, (_, i) => hit(i + 1)))
    es.emit('tick', { ...tick(1), hits: Array.from({ length: 30 }, (_, i) => hit(191 + i)) })
    expect(live.hits.length).toBe(200)
    expect(live.hits[0].seq).toBe(220)
    expect(live.hits.at(-1)?.seq).toBe(21)
    // Le service a redémarré : la numérotation repart de 1.
    es.emit('hits', [hit(1), hit(2)])
    expect(live.hits.map((h) => h.seq)).toEqual([2, 1])
    es.emit('tick', { ...tick(2), hits: [hit(3)] })
    expect(live.hits.map((h) => h.seq)).toEqual([3, 2, 1])
    live.close()
  })
})
