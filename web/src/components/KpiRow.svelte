<script lang="ts">
  import type { Kpis } from '../lib/api'
  import { formatBytes, formatCount, formatRps, percent } from '../lib/format'
  import { STACK, total } from '../lib/model'
  import ClassBar from './ClassBar.svelte'
  import Icon from './Icon.svelte'

  let { kpis, currentRps, busy }: { kpis: Kpis | null; currentRps: number | null; busy: boolean } = $props()

  const sum = $derived(kpis ? total(kpis.byClass) : 0)
  const parts = $derived(
    kpis
      ? STACK.map((s) => ({ ...s, n: s.key === 'abandon' ? kpis.byClass.abandon + kpis.byClass['1xx'] : kpis.byClass[s.key] }))
      : [],
  )
</script>

<section class="kpis" class:busy aria-label="Indicateurs de charge">
  <article class="card kpi">
    <h3><Icon name="activity" size={14} />Requêtes par seconde</h3>
    {#if kpis}
      <p class="value">{formatRps(kpis.avgRps)}<small>en moyenne</small></p>
      <p class="sub">
        pic <b>{formatRps(kpis.peakRps)}</b>
        {#if currentRps !== null}<span aria-hidden="true">·</span> en ce moment <b>{formatRps(currentRps)}</b>{/if}
      </p>
    {:else}
      <span class="skeleton sk-value"></span><span class="skeleton sk-sub"></span>
    {/if}
  </article>

  <article class="card kpi">
    <h3><Icon name="arrow-down" size={14} />Volume entrant</h3>
    {#if kpis}
      <p class="value">{formatBytes(kpis.bytesIn)}</p>
      <p class="sub">corps des requêtes reçues</p>
    {:else}
      <span class="skeleton sk-value"></span><span class="skeleton sk-sub"></span>
    {/if}
  </article>

  <article class="card kpi">
    <h3><Icon name="arrow-up" size={14} />Volume sortant</h3>
    {#if kpis}
      <p class="value">{formatBytes(kpis.bytesOut)}</p>
      <p class="sub">réponses envoyées</p>
    {:else}
      <span class="skeleton sk-value"></span><span class="skeleton sk-sub"></span>
    {/if}
  </article>

  <article class="card kpi statuses">
    <h3>Statuts</h3>
    {#if kpis}
      <p class="value">{formatCount(sum)}<small>requêtes</small></p>
      <ClassBar counts={kpis.byClass} height={8} />
      <dl>
        {#each parts as p (p.key)}
          <div class:nil={p.n === 0}>
            <dt><i class="swatch" style:background="var({p.cssVar})"></i>{p.label}</dt>
            <dd>{formatCount(p.n)} <span>{percent(p.n, sum)}</span></dd>
          </div>
        {/each}
      </dl>
    {:else}
      <span class="skeleton sk-value"></span><span class="skeleton sk-sub wide"></span>
    {/if}
  </article>

  <article class="card kpi">
    <h3><Icon name="shield" size={14} />Bans CrowdSec</h3>
    {#if kpis}
      {#if kpis.activeBans < 0}
        <p class="value unknown">—</p>
        <p class="sub">CrowdSec injoignable</p>
      {:else}
        <p class="value">{formatCount(kpis.activeBans)}</p>
        <p class="sub">{kpis.activeBans < 2 ? 'ban en cours' : 'bans en cours'} (4 h chacun)</p>
      {/if}
    {:else}
      <span class="skeleton sk-value"></span><span class="skeleton sk-sub"></span>
    {/if}
  </article>
</section>

<style>
  .kpis {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr 2.1fr 1fr;
    gap: 12px;
    transition: opacity 0.2s;
  }
  .busy {
    opacity: 0.55;
  }
  .kpi {
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    min-height: 104px;
  }
  h3 {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    font-weight: 550;
    color: var(--text-2);
  }
  h3 :global(svg) {
    color: var(--text-3);
  }
  .value {
    margin-top: 6px;
    font-size: 27px;
    font-weight: 680;
    letter-spacing: -0.025em;
    line-height: 1.15;
    white-space: nowrap;
  }
  .value small {
    margin-left: 6px;
    font-size: 12.5px;
    font-weight: 500;
    letter-spacing: 0;
    color: var(--text-3);
  }
  .value.unknown {
    color: var(--text-3);
  }
  .sub {
    margin-top: auto;
    padding-top: 6px;
    font-size: 12.5px;
    color: var(--text-3);
  }
  .sub b {
    font-weight: 600;
    color: var(--text-2);
  }
  .statuses :global(.bar) {
    margin-top: 10px;
  }
  dl {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 16px;
    margin: 9px 0 0;
    font-size: 12.5px;
  }
  dl div {
    display: flex;
    gap: 6px;
    align-items: baseline;
  }
  dt {
    color: var(--text-2);
    white-space: nowrap;
  }
  dd {
    margin: 0;
    font-weight: 600;
    white-space: nowrap;
  }
  dd span {
    font-weight: 400;
    color: var(--text-3);
  }
  .nil {
    opacity: 0.5;
  }
  .sk-value {
    width: 55%;
    height: 28px;
    margin-top: 8px;
  }
  .sk-sub {
    width: 75%;
    height: 12px;
    margin-top: auto;
  }
  .sk-sub.wide {
    width: 100%;
  }

  @media (max-width: 1100px) {
    .kpis {
      grid-template-columns: repeat(4, 1fr);
    }
    .statuses {
      grid-column: 1 / -1;
      order: 1;
    }
  }
  @media (max-width: 720px) {
    .kpis {
      grid-template-columns: 1fr 1fr;
      gap: 8px;
    }
    .kpi {
      padding: 12px;
      min-height: 92px;
    }
    .value {
      font-size: 22px;
    }
  }
</style>
