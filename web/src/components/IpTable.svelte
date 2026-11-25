<script lang="ts">
  import type { ClassCounts, IpRow, Summary } from '../lib/api'
  import { countryName, formatBytes, formatCount, pluriel } from '../lib/format'
  import { groupIps, type IpGroup } from '../lib/model'
  import Icon from './Icon.svelte'
  import VerdictBadge from './VerdictBadge.svelte'

  let { summary, busy, onselect }: { summary: Summary | null; busy: boolean; onselect: (ip: string) => void } = $props()

  type Filter = 'all' | 'named' | 'unknown'
  let filter = $state<Filter>('all')
  let expanded = $state<Record<string, boolean>>({})
  let showAll = $state(false)
  const PAGE = 15

  const groups = $derived(groupIps(summary?.ips ?? []))
  const unknownShown = $derived(showAll ? groups.unknown : groups.unknown.slice(0, PAGE))
  const cols = ['2xx', '3xx', '4xx', '5xx'] as const

  function shortIp(ip: string): string {
    return ip.length > 28 ? ip.slice(0, 14) + '…' + ip.slice(-10) : ip
  }
  function activate(g: IpGroup) {
    if (g.rows.length === 1) onselect(g.rows[0].ip)
    else expanded[g.name] = !expanded[g.name]
  }
</script>

{#snippet numbers(c: ClassCounts, bytesIn: number, bytesOut: number)}
  {#each cols as k (k)}
    <td class="num" class:zero={c[k] === 0}>{formatCount(c[k])}</td>
  {/each}
  <td class="num" title="{formatBytes(bytesIn)} entrants, {formatBytes(bytesOut)} sortants">{formatBytes(bytesIn + bytesOut)}</td>
{/snippet}

{#snippet where(r: IpRow)}
  <span class="meta">
    {#if r.country}<span title={countryName(r.country)}>{r.country}</span>{/if}
    {#if r.country && r.org}<span aria-hidden="true">·</span>{/if}
    {#if r.org}<span>{r.org}</span>{/if}
    {#if !r.country && !r.org && r.ip !== 'autres'}<span>origine inconnue</span>{/if}
  </span>
{/snippet}

<section class="card" class:busy>
  <header class="card-head">
    <div>
      <h2 class="card-title">Adresses IP</h2>
      <p class="card-sub">
        {#if summary}
          {pluriel(summary.counts.namedIps, 'nommée', 'nommées')} · {pluriel(summary.counts.unknownIps, 'inconnue', 'inconnues')} · cliquer
          une ligne pour le détail
        {:else}
          &nbsp;
        {/if}
      </p>
    </div>
    <div class="segmented" role="group" aria-label="Filtrer les adresses">
      <button type="button" aria-pressed={filter === 'all'} onclick={() => (filter = 'all')}>Toutes</button>
      <button type="button" aria-pressed={filter === 'named'} onclick={() => (filter = 'named')}>Nommées</button>
      <button type="button" aria-pressed={filter === 'unknown'} onclick={() => (filter = 'unknown')}>Inconnues</button>
    </div>
  </header>

  {#if !summary}
    <div class="sk-rows">{#each Array(6) as _, i (i)}<span class="skeleton"></span>{/each}</div>
  {:else if summary.ips.length === 0}
    <p class="empty">Aucune requête sur la période.</p>
  {:else}
    <div class="table-scroll">
      <table class="tbl">
        <thead>
          <tr>
            <th scope="col">Qui</th>
            {#each cols as k (k)}<th scope="col" class="num"><i class="swatch" style:background="var(--c-{k})"></i>{k}</th>{/each}
            <th scope="col" class="num">Volume</th>
            <th scope="col" class="vcol">Verdict</th>
          </tr>
        </thead>

        {#if filter !== 'unknown'}
          <tbody>
            <tr class="section"><th colspan="7" scope="colgroup">Nommées <span>{groups.named.length}</span></th></tr>
            {#each groups.named as g (g.name)}
              {@const multi = g.rows.length > 1}
              <tr class="row" onclick={() => activate(g)}>
                <td>
                  <button type="button" class="who" aria-expanded={multi ? !!expanded[g.name] : undefined}
                    onclick={(e) => { e.stopPropagation(); activate(g) }}>
                    <span class="caret" class:open={expanded[g.name]} class:hidden={!multi}><Icon name="chevron" size={14} /></span>
                    <span class="id">
                      <strong>{g.name}</strong>
                      {#if multi}
                        <span class="meta">{g.rows.length} adresses</span>
                      {:else}
                        <span class="meta mono" title={g.rows[0].ip}>{shortIp(g.rows[0].ip)}</span>
                      {/if}
                    </span>
                  </button>
                </td>
                {@render numbers(g.byClass, g.bytesIn, g.bytesOut)}
                <td class="vcol"><VerdictBadge verdict="connue" /></td>
              </tr>
              {#if multi && expanded[g.name]}
                {#each g.rows as r (r.ip)}
                  <tr class="row sub" onclick={() => onselect(r.ip)}>
                    <td>
                      <button type="button" class="who" onclick={(e) => { e.stopPropagation(); onselect(r.ip) }}>
                        <span class="caret hidden"></span>
                        <span class="id"><span class="mono" title={r.ip}>{shortIp(r.ip)}</span>{@render where(r)}</span>
                      </button>
                    </td>
                    {@render numbers(r.byClass, r.bytesIn, r.bytesOut)}
                    <td class="vcol"></td>
                  </tr>
                {/each}
              {/if}
            {:else}
              <tr><td colspan="7" class="none">Aucune IP nommée sur la période.</td></tr>
            {/each}
          </tbody>
        {/if}

        {#if filter !== 'named'}
          <tbody>
            <tr class="section"><th colspan="7" scope="colgroup">Inconnues <span>{summary.counts.unknownIps}</span></th></tr>
            {#each unknownShown as r (r.ip)}
              <tr class="row" class:hot={r.verdict === 'succès'} onclick={() => r.ip !== 'autres' && onselect(r.ip)}>
                <td>
                  <button type="button" class="who" disabled={r.ip === 'autres'}
                    onclick={(e) => { e.stopPropagation(); onselect(r.ip) }}>
                    <span class="caret hidden"></span>
                    <span class="id">
                      {#if r.ip === 'autres'}
                        <strong>Autres IP</strong><span class="meta">regroupées : trafic exceptionnel</span>
                      {:else}
                        <span class="mono ip" title={r.ip}>{shortIp(r.ip)}</span>
                        <span class="meta-line">
                          {@render where(r)}
                          {#if r.looksLike}<span class="chip warn">ressemble à {r.looksLike}</span>{/if}
                          <span class="inline-verdict"><VerdictBadge verdict={r.verdict} /></span>
                          {#if r.ban}<span class="chip ban" title="Ban CrowdSec en cours"><Icon name="ban" size={11} />bannie · {r.ban.scenario.replace(/^crowdsecurity\//, '')}</span>{/if}
                        </span>
                      {/if}
                    </span>
                  </button>
                </td>
                {@render numbers(r.byClass, r.bytesIn, r.bytesOut)}
                <td class="vcol"><VerdictBadge verdict={r.verdict} /></td>
              </tr>
            {:else}
              <tr><td colspan="7" class="none">Aucune IP inconnue sur la période.</td></tr>
            {/each}
          </tbody>
        {/if}
      </table>
    </div>
    {#if filter !== 'named' && groups.unknown.length > PAGE}
      <button type="button" class="more" onclick={() => (showAll = !showAll)}>
        {showAll ? 'Réduire' : `Afficher les ${groups.unknown.length - PAGE} autres`}
        {#if showAll && summary.counts.unknownIps > groups.unknown.length}
          <span class="muted">({groups.unknown.length} affichées sur {formatCount(summary.counts.unknownIps)})</span>
        {/if}
      </button>
    {/if}
  {/if}
</section>

<style>
  .card {
    transition: opacity 0.2s;
  }
  .busy {
    opacity: 0.55;
  }
  .tbl {
    min-width: 640px;
  }
  .section th {
    position: static;
    padding: 7px 16px;
    background: var(--surface);
    color: var(--text-2);
    font-size: 11.5px;
    border-bottom: 1px solid var(--border);
  }
  tbody + tbody .section th {
    border-top: 1px solid var(--border-strong);
  }
  .section span {
    margin-left: 4px;
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--surface-3);
    font-weight: 600;
    letter-spacing: 0;
  }
  .row {
    cursor: pointer;
    transition: background-color 0.12s;
  }
  .row:hover {
    background: var(--surface-2);
  }
  .row.sub {
    background: var(--surface-2);
  }
  .row.sub:hover {
    background: var(--surface-3);
  }
  .row.hot {
    background: var(--bad-bg);
    box-shadow: inset 3px 0 0 var(--bad-solid);
  }
  .who {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    cursor: pointer;
  }
  .who:disabled {
    cursor: default;
  }
  .caret {
    flex: none;
    width: 14px;
    color: var(--text-3);
    transition: transform 0.15s;
  }
  .caret.open {
    transform: rotate(90deg);
  }
  .caret.hidden {
    visibility: hidden;
  }
  .id {
    display: grid;
    gap: 1px;
    min-width: 0;
  }
  .id strong {
    font-weight: 620;
  }
  .ip {
    font-weight: 550;
  }
  .meta,
  .meta-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 3px 5px;
    font-size: 12px;
    color: var(--text-3);
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 0 7px;
    border-radius: 999px;
    font-size: 11.5px;
    font-weight: 550;
    line-height: 18px;
    white-space: nowrap;
  }
  .chip.warn {
    color: var(--warn);
    background: var(--warn-bg);
    border: 1px solid var(--warn-border);
  }
  .chip.ban {
    color: var(--text-2);
    background: var(--surface-3);
  }
  .none {
    color: var(--text-3);
    text-align: center;
    padding: 18px 16px;
  }
  .inline-verdict {
    display: none;
  }
  /* Téléphone : le verdict passe sous l'IP, pour rester visible sans défiler. */
  @media (max-width: 720px) {
    .tbl {
      min-width: 520px;
    }
    .vcol {
      display: none;
    }
    .inline-verdict {
      display: inline-flex;
    }
    .who {
      min-width: 170px;
    }
  }
  .sk-rows {
    display: grid;
    gap: 10px;
    padding: 6px 16px 16px;
  }
  .sk-rows .skeleton {
    height: 30px;
  }
</style>
