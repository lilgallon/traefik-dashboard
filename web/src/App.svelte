<script lang="ts">
  import { onMount } from 'svelte'
  import CrowdSecPanel from './components/CrowdSecPanel.svelte'
  import Freshness from './components/Freshness.svelte'
  import Icon from './components/Icon.svelte'
  import IpDetail from './components/IpDetail.svelte'
  import IpTable from './components/IpTable.svelte'
  import KpiRow from './components/KpiRow.svelte'
  import LiveFeed from './components/LiveFeed.svelte'
  import PeriodPicker from './components/PeriodPicker.svelte'
  import ServiceTable from './components/ServiceTable.svelte'
  import TrafficChart from './components/TrafficChart.svelte'
  import UrlTable from './components/UrlTable.svelte'
  import VerdictBanner from './components/VerdictBanner.svelte'
  import Warnings from './components/Warnings.svelte'
  import { fetchSummary, WINDOWS, type Period, type Summary, type TimeRange, type Window } from './lib/api'
  import { createLive } from './lib/live.svelte'
  import { chartData, dataState, liveKpis, periodKey, rangeLabel, total } from './lib/model'

  const REFRESH_MS = 5000

  type ThemePref = 'auto' | 'light' | 'dark'

  // Préférences : mémorisées si le navigateur le permet, jamais indispensables.
  function load<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
    try {
      const v = localStorage.getItem(key) as T | null
      return v && allowed.includes(v) ? v : fallback
    } catch {
      return fallback
    }
  }
  function save(key: string, value: string) {
    try {
      localStorage.setItem(key, value)
    } catch {
      // navigation privée, stockage bloqué : sans importance
    }
  }
  function ipFromHash(): string | null {
    const m = /^#ip=(.+)$/.exec(location.hash)
    if (!m) return null
    const ip = decodeURIComponent(m[1])
    return /^[0-9a-fA-F:.]{3,45}$/.test(ip) ? ip : null
  }

  let win = $state<Window>(load('td-window', WINDOWS, '24h'))
  let themePref = $state<ThemePref>(load('td-theme', ['auto', 'light', 'dark'] as const, 'auto'))
  let systemDark = $state(false)
  // Intervalle choisi sur le graphique : la page se fige dessus (plus de direct).
  let range = $state.raw<TimeRange | null>(null)
  const period = $derived<Period>(range ?? win)
  let summary = $state.raw<Summary | null>(null)
  let summaryKey = $state('')
  let error = $state<string | null>(null)
  let now = $state(new Date())
  let selected = $state<string | null>(ipFromHash())

  const live = createLive('/api/stream')

  const theme = $derived(themePref === 'auto' ? (systemDark ? 'dark' : 'light') : themePref)
  $effect(() => {
    if (themePref === 'auto') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = themePref
  })

  let seq = 0
  let lastForced = 0
  async function refresh() {
    const id = ++seq
    const p = period
    try {
      const s = await fetchSummary(p)
      if (id !== seq) return // une réponse plus récente est déjà en route
      summary = s
      summaryKey = periodKey(p)
      error = null
    } catch (e) {
      if (id === seq) error = e instanceof Error ? e.message : String(e)
    }
  }

  function setWindow(w: Window) {
    if (w === win && !range) return
    win = w
    range = null
    save('td-window', w)
    void refresh()
  }

  /** Zoom sur un intervalle du graphique (on peut zoomer encore dans le zoom). */
  function setRange(r: TimeRange) {
    range = r
    void refresh()
  }

  function backToLive() {
    if (!range) return
    range = null
    void refresh()
  }

  function cycleTheme() {
    themePref = themePref === 'auto' ? 'light' : themePref === 'light' ? 'dark' : 'auto'
    save('td-theme', themePref)
  }

  function select(ip: string | null) {
    selected = ip
    history.replaceState(null, '', ip ? `#ip=${encodeURIComponent(ip)}` : location.pathname + location.search)
  }

  onMount(() => {
    void refresh()
    // Un intervalle figé ne change pas : rafraîchir ne sert qu'au direct.
    const poll = setInterval(() => {
      if (!document.hidden && !range) void refresh()
    }, REFRESH_MS)
    const clock = setInterval(() => (now = new Date()), 1000)
    const onvisible = () => {
      if (!document.hidden && !range) void refresh()
    }
    const onkey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && range && !selected) backToLive()
    }
    window.addEventListener('keydown', onkey)
    document.addEventListener('visibilitychange', onvisible)
    const onhash = () => (selected = ipFromHash())
    window.addEventListener('hashchange', onhash)
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark = mq.matches
    const onscheme = (e: MediaQueryListEvent) => (systemDark = e.matches)
    mq.addEventListener('change', onscheme)
    return () => {
      clearInterval(poll)
      clearInterval(clock)
      document.removeEventListener('visibilitychange', onvisible)
      window.removeEventListener('hashchange', onhash)
      window.removeEventListener('keydown', onkey)
      mq.removeEventListener('change', onscheme)
      live.close()
    }
  })

  // Le flux annonce le verdict chaque seconde : s'il diffère du résumé affiché, on
  // recharge tout de suite au lieu d'attendre le prochain passage.
  $effect(() => {
    const t = live.last
    if (!t || !summary || range || summary.window !== win) return
    if ((t.red[win] !== summary.verdict.red || t.loading !== summary.loading) && Date.now() - lastForced > 1500) {
      lastForced = Date.now()
      void refresh()
    }
  })

  const current = $derived(summary && summaryKey === periodKey(period) ? summary : null)
  const busy = $derived(summary !== null && current === null)
  const shown = $derived(current ?? summary)
  const mode = $derived(dataState(shown, error, now))
  const kpis = $derived(shown ? liveKpis(shown, live.ticks) : null)
  const chart = $derived(shown ? chartData(shown, shown.range ? [] : live.ticks) : null)
  // Intervalle effectif (arrondi par le service) ; celui demandé en attendant.
  const shownRange = $derived(range ? (current?.range ?? range) : null)
  const currentRps = $derived.by(() => {
    if (range || !live.connected || live.ticks.length === 0) return null
    const recent = live.ticks.slice(-10)
    return recent.reduce((n, t) => n + total(t.byClass), 0) / recent.length
  })
  const lastLine = $derived.by(() => {
    const a = shown?.lastLine ?? ''
    const b = live.last?.lastLine ?? ''
    return Date.parse(b) > Date.parse(a) || !a ? b || a : a
  })

  const themeLabel: Record<ThemePref, string> = { auto: 'Thème : automatique', light: 'Thème : clair', dark: 'Thème : sombre' }
  const themeIcon = $derived(themePref === 'auto' ? 'auto' : themePref === 'light' ? 'sun' : 'moon')

  $effect(() => {
    document.title = shown?.verdict.red && mode !== 'backfill' ? '🔴 Traefik dashboard' : 'Traefik dashboard'
  })
</script>

<div class="page">
  <header class="top">
    <div class="brand">
      <span class="logo" aria-hidden="true"><Icon name="pulse" size={18} /></span>
      <div>
        <h1>Traefik</h1>
        <p>Trafic entrant</p>
      </div>
    </div>
    <div class="fresh"><Freshness {mode} connected={live.connected} {lastLine} {now} /></div>
    <div class="controls">
      <PeriodPicker value={range ? null : win} onchange={setWindow} />
      <button type="button" class="theme" onclick={cycleTheme} aria-label={themeLabel[themePref]} title={themeLabel[themePref]}>
        <Icon name={themeIcon} size={17} />
      </button>
    </div>
  </header>

  <main>
    <Warnings items={shown?.warnings ?? []} />
    {#if shownRange}
      <div class="range" role="status">
        <Icon name="clock" size={16} />
        <p>Intervalle figé <b>{rangeLabel(shownRange)}</b> · plus de mise à jour en direct</p>
        <button type="button" onclick={backToLive}>Revenir au direct <kbd>Échap</kbd></button>
      </div>
    {/if}
    <VerdictBanner {mode} summary={shown} period={shownRange ?? win} {now} onselect={select} />
    <KpiRow {kpis} {currentRps} {busy} />
    <TrafficChart
      data={chart}
      step={shown?.series.stepSeconds ?? 60}
      live={!range}
      {theme}
      {busy}
      onrange={setRange}
    />
    {#if !range}
      <LiveFeed hits={live.hits} connected={live.connected} onselect={select} />
    {/if}
    <IpTable summary={shown} {busy} onselect={select} />
    <CrowdSecPanel summary={shown} period={shownRange ?? win} {now} {busy} onselect={select} onchange={() => void refresh()} />
    <div class="split">
      <UrlTable summary={shown} {busy} />
      <ServiceTable summary={shown} {busy} />
    </div>
  </main>

  <footer>
    <p>
      Heures affichées dans le fuseau du navigateur · tableaux rafraîchis toutes les 5 s (sauf sur un intervalle figé) · noms et URL anonymes dans
      <span class="mono">names.yaml</span>
    </p>
  </footer>
</div>

{#if selected}
  {#key selected}
    <IpDetail ip={selected} {period} {now} onclose={() => select(null)} />
  {/key}
{/if}

<style>
  .page {
    max-width: 1280px;
    margin: 0 auto;
    padding: 18px 24px 40px;
  }
  .top {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 12px 20px;
    margin-bottom: 16px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 11px;
  }
  .logo {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: 9px;
    background: var(--text);
    color: var(--bg);
  }
  h1 {
    font-size: 17px;
    font-weight: 700;
    letter-spacing: -0.02em;
    line-height: 1.2;
  }
  .brand p {
    font-size: 12.5px;
    color: var(--text-3);
  }
  .fresh {
    justify-self: end;
    min-width: 0;
  }
  .controls {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .theme {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: 9px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-2);
    cursor: pointer;
  }
  .theme:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
  main {
    display: grid;
    gap: 12px;
    min-width: 0;
  }
  .range {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    border-radius: var(--radius);
    border: 1px solid var(--border-strong);
    background: var(--surface);
    font-size: 13.5px;
    color: var(--text-2);
  }
  .range p {
    flex: 1;
    min-width: 0;
  }
  .range b {
    color: var(--text);
    font-weight: 600;
  }
  .range button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
  }
  .range button:hover {
    border-color: var(--border-strong);
  }
  .range kbd {
    font: 11px var(--font);
    color: var(--text-3);
  }
  .split {
    display: grid;
    grid-template-columns: minmax(0, 1.75fr) minmax(0, 1fr);
    gap: 12px;
    align-items: start;
  }
  footer {
    margin-top: 20px;
    font-size: 12px;
    color: var(--text-3);
    text-align: center;
  }

  @media (max-width: 960px) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 720px) {
    .page {
      padding: 12px 12px 32px;
    }
    .top {
      grid-template-columns: 1fr auto;
      margin-bottom: 12px;
    }
    .fresh {
      grid-column: 1 / -1;
      grid-row: 2;
      justify-self: start;
    }
    main {
      gap: 8px;
    }
    .brand p {
      display: none;
    }
    .split {
      gap: 8px;
    }
    .range {
      flex-wrap: wrap;
    }
    .range kbd {
      display: none;
    }
  }
</style>
