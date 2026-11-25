import type { Hit, LiveTick } from './api'

const MAX_TICKS = 3600
const MAX_HITS = 200
const RECONNECT_MS = 3000

export interface Live {
  /** Une heure de ticks au plus, du plus ancien au plus récent. */
  readonly ticks: LiveTick[]
  readonly last: LiveTick | null
  /** Dernières requêtes lues, la plus récente d'abord (200 au plus). */
  readonly hits: Hit[]
  readonly connected: boolean
  close(): void
}

interface Options {
  EventSourceImpl?: typeof EventSource
}

/** Flux en direct (Server-Sent Events) : un tick par seconde, reconnexion automatique. */
export function createLive(url: string, options: Options = {}): Live {
  const Impl = 'EventSourceImpl' in options ? options.EventSourceImpl : globalThis.EventSource

  let ticks = $state.raw<LiveTick[]>([])
  let hits = $state.raw<Hit[]>([])
  let connected = $state(false)
  let source: EventSource | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let closed = false

  const valid = (list: unknown): Hit[] =>
    Array.isArray(list) ? (list as Hit[]).filter((h) => typeof h?.seq === 'number' && typeof h.ip === 'string') : []

  /** Ajoute les requêtes plus récentes que la dernière connue (les doublons sont écartés). */
  function addHits(list: unknown) {
    const top = hits.length > 0 ? hits[0].seq : 0
    const fresh = valid(list)
      .filter((h) => h.seq > top)
      .sort((a, b) => b.seq - a.seq)
    if (fresh.length > 0) hits = fresh.concat(hits).slice(0, MAX_HITS)
  }

  function connect() {
    if (closed || !Impl) return
    const es = new Impl(url)
    source = es
    es.onopen = () => {
      connected = true
    }
    // Instantané envoyé à chaque connexion : il remplace tout, ce qui couvre aussi le
    // redémarrage du service (la numérotation repart alors de 1).
    es.addEventListener('hits', (ev) => {
      try {
        hits = valid(JSON.parse((ev as MessageEvent<string>).data))
          .sort((a, b) => b.seq - a.seq)
          .slice(0, MAX_HITS)
      } catch {
        // instantané illisible : les ticks suivants rempliront la liste
      }
    })
    es.addEventListener('tick', (ev) => {
      let tick: LiveTick
      try {
        tick = JSON.parse((ev as MessageEvent<string>).data) as LiveTick
      } catch {
        return
      }
      addHits(tick?.hits)
      const lastT = ticks.length > 0 ? ticks[ticks.length - 1].t : -Infinity
      if (typeof tick?.t !== 'number' || tick.t <= lastT) return
      connected = true
      const next = ticks.length >= MAX_TICKS ? ticks.slice(ticks.length - MAX_TICKS + 1) : ticks.slice()
      next.push(tick)
      ticks = next
    })
    es.onerror = () => {
      connected = false
      // readyState CONNECTING : le navigateur réessaie seul. CLOSED : à nous de jouer.
      if (es.readyState === 2 && !closed) {
        es.close()
        clearTimeout(timer)
        timer = setTimeout(connect, RECONNECT_MS)
      }
    }
  }

  connect()

  return {
    get ticks() {
      return ticks
    },
    get last() {
      return ticks.length > 0 ? ticks[ticks.length - 1] : null
    },
    get hits() {
      return hits
    },
    get connected() {
      return connected
    },
    close() {
      closed = true
      connected = false
      clearTimeout(timer)
      source?.close()
    },
  }
}
