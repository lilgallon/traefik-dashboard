<script lang="ts">
  import { untrack } from 'svelte'
  import uPlot from 'uplot'
  import type { TimeRange } from '../lib/api'
  import { formatClock, formatDay, formatDayTime, formatRps } from '../lib/format'
  import { lang, t } from '../lib/i18n.svelte'
  import { classLabel, STACK, type ChartData } from '../lib/model'

  let {
    data,
    step,
    live,
    theme,
    busy,
    onrange,
  }: {
    data: ChartData | null
    /** Pas de la série, en secondes. */
    step: number
    /** Période glissante (prolongée par le flux) plutôt qu'intervalle figé. */
    live: boolean
    theme: string
    busy: boolean
    /** Intervalle sélectionné à la souris (ou au doigt) sur le graphique. */
    onrange: (r: TimeRange) => void
  } = $props()

  // En deçà, un glisser est tenu pour un simple clic.
  const MIN_DRAG_PX = 6

  let host = $state<HTMLDivElement>()
  let plot: uPlot | undefined
  let hover = $state<{ idx: number; left: number } | null>(null)
  let width = $state(0)

  const granularity = $derived(
    (step <= 1 ? t('chart.perSecond') : step <= 60 ? t('chart.perMinute') : t('chart.perHour')) + (live ? t('chart.live') : ''),
  )
  const empty = $derived(!!data && data.stacked.length > 0 && data.stacked[data.stacked.length - 1].every((v) => v === 0))

  // uPlot dessine les séries dans l'ordre : la plus haute du cumul d'abord, les
  // suivantes par-dessus.
  function toPlot(d: ChartData): uPlot.AlignedData {
    return [d.t, ...d.stacked.slice().reverse()] as uPlot.AlignedData
  }

  const two = (n: number) => String(n).padStart(2, '0')
  function tickLabel(sec: number): string {
    const d = new Date(sec * 1000)
    if (d.getHours() === 0 && d.getMinutes() === 0) return formatDay(d)
    return `${two(d.getHours())}:${two(d.getMinutes())}`
  }

  function build(el: HTMLDivElement, d: ChartData) {
    const css = getComputedStyle(el)
    const v = (name: string) => css.getPropertyValue(name).trim()
    const axisColor = v('--text-3')
    const font = `11px ${v('--font')}`
    const opts: uPlot.Options = {
      width: el.clientWidth,
      height: el.clientHeight,
      padding: [8, 8, 0, 0],
      legend: { show: false },
      // Glisser sélectionne un intervalle ; setScale: false, c'est la page qui recharge.
      cursor: { y: false, points: { show: false }, drag: { x: true, y: false, setScale: false } },
      scales: {
        x: { time: true },
        y: { range: (_u, _min, max) => [0, max > 0 ? max * 1.08 : 1] },
      },
      axes: [
        {
          stroke: axisColor,
          font,
          grid: { show: false },
          ticks: { show: false },
          values: (_u, splits) => splits.map(tickLabel),
          gap: 6,
          size: 28,
        },
        {
          stroke: axisColor,
          font,
          size: 46,
          gap: 6,
          grid: { stroke: v('--grid'), width: 1 },
          ticks: { show: false },
          values: (_u, splits) => splits.map((s) => formatRps(s)),
        },
      ],
      series: [
        {},
        ...STACK.slice()
          .reverse()
          .map((s) => {
            const color = v(s.cssVar)
            return {
              label: classLabel(s.key),
              stroke: color,
              width: 1,
              fill: color + 'd9',
              paths: uPlot.paths.stepped!({ align: 1 }),
              points: { show: false },
            } satisfies uPlot.Series
          }),
      ],
      hooks: {
        setSelect: [
          (u) => {
            const { left, width } = u.select
            if (width < MIN_DRAG_PX) return
            const from = u.posToVal(left, 'x')
            // Le dernier point couvre [t, t + pas[ : la sélection va jusqu'à sa fin.
            const to = u.posToVal(left + width, 'x')
            u.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false)
            if (Number.isFinite(from) && Number.isFinite(to) && to > from) onrange({ from: Math.floor(from), to: Math.ceil(to) })
          },
        ],
        setCursor: [
          (u) => {
            const idx = u.cursor.idx
            hover = idx == null || (u.cursor.left ?? -1) < 0 ? null : { idx, left: (u.cursor.left ?? 0) + u.over.offsetLeft }
          },
        ],
      },
    }
    return new uPlot(opts, toPlot(d), el)
  }

  // (Re)création : au premier rendu, et quand le thème (couleurs lues dans le CSS) ou
  // la langue (libellés des axes) change.
  const hasData = $derived(data !== null)
  $effect(() => {
    void theme
    void lang()
    const el = host
    if (!el || !hasData) return
    // Les données suivantes passent par setData (effet ci-dessous) : les lire ici
    // sans s'y abonner, sinon le graphique serait recréé à chaque seconde.
    const initial = untrack(() => data)
    if (!initial) return
    const u = build(el, initial)
    plot = u
    const ro = new ResizeObserver(() => {
      width = el.clientWidth
      if (el.clientWidth > 0) u.setSize({ width: el.clientWidth, height: el.clientHeight })
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
      u.destroy()
      plot = undefined
      hover = null
    }
  })

  $effect(() => {
    if (data && plot) plot.setData(toPlot(data))
  })

  const tip = $derived.by(() => {
    if (!hover || !data || hover.idx >= data.t.length) return null
    const i = hover.idx
    const rows = STACK.map((s, k) => ({ ...s, value: data.raw[k].values[i] ?? 0 })).reverse()
    const sum = rows.reduce((n, r) => n + r.value, 0)
    const d = new Date(data.t[i] * 1000)
    return {
      when: step <= 1 ? formatClock(d) : formatDayTime(d),
      rows,
      sum,
      flip: width > 0 && hover.left > width / 2,
      left: hover.left,
    }
  })
</script>

<section class="card chart" class:busy>
  <header class="card-head">
    <div>
      <h2 class="card-title">{t('kpi.rps')}</h2>
      <p class="card-sub">{t('chart.byClass')} · {granularity} · <span class="hint">{t('chart.dragHint')}</span></p>
    </div>
    <ul class="legend" aria-label={t('chart.legend')}>
      {#each STACK as s (s.key)}
        <li><i class="swatch" style:background="var({s.cssVar})"></i>{classLabel(s.key)}</li>
      {/each}
    </ul>
  </header>
  <div class="plot-wrap">
    {#if data}
      <div class="plot" bind:this={host} role="img" aria-label={t('chart.aria')}></div>
      {#if empty}<p class="none">{t('chart.empty')}</p>{/if}
      {#if tip}
        <div class="tip" class:flip={tip.flip} style:left="{tip.left}px" aria-hidden="true">
          <p class="when">{tip.when}</p>
          {#each tip.rows as r (r.key)}
            <p class:nil={r.value === 0}><i class="swatch" style:background="var({r.cssVar})"></i>{classLabel(r.key)}<b>{formatRps(r.value)}</b></p>
          {/each}
          <p class="sum">{t('chart.total')}<b>{formatRps(tip.sum)}</b></p>
        </div>
      {/if}
    {:else}
      <span class="skeleton sk"></span>
    {/if}
  </div>
</section>

<style>
  .chart {
    transition: opacity 0.2s;
  }
  .busy {
    opacity: 0.55;
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    list-style: none;
    margin: 0;
    padding: 0;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .plot-wrap {
    position: relative;
    padding: 0 8px 10px;
  }
  .plot,
  .sk {
    height: 250px;
    width: 100%;
  }
  .plot {
    cursor: crosshair;
    touch-action: pan-y;
  }
  .plot :global(.u-select) {
    background: color-mix(in srgb, var(--text) 12%, transparent);
    border-left: 1px solid var(--border-strong);
    border-right: 1px solid var(--border-strong);
  }
  .hint {
    color: var(--text-3);
  }
  .none {
    position: absolute;
    inset: 0 0 30px 46px;
    display: grid;
    place-items: center;
    color: var(--text-3);
    font-size: 13px;
    pointer-events: none;
  }
  .tip {
    position: absolute;
    top: 6px;
    z-index: 2;
    min-width: 132px;
    padding: 8px 10px;
    transform: translateX(12px);
    border-radius: var(--radius-sm);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.14);
    font-size: 12.5px;
    pointer-events: none;
  }
  .tip.flip {
    transform: translateX(calc(-100% - 12px));
  }
  .tip p {
    display: flex;
    align-items: center;
    gap: 2px;
    white-space: nowrap;
    color: var(--text-2);
  }
  .tip b {
    margin-left: auto;
    padding-left: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .tip .when {
    margin-bottom: 4px;
    font-weight: 600;
    color: var(--text);
  }
  .tip .nil {
    opacity: 0.45;
  }
  .tip .sum {
    margin-top: 4px;
    padding-top: 4px;
    border-top: 1px solid var(--border);
  }
  @media (max-width: 720px) {
    .plot,
    .sk {
      height: 200px;
    }
    .plot-wrap {
      padding: 0 4px 8px;
    }
  }
</style>
