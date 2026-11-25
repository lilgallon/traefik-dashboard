<script lang="ts">
  import type { Summary } from '../lib/api'
  import { formatCount, pluriel } from '../lib/format'
  import { total } from '../lib/model'
  import ClassBar from './ClassBar.svelte'

  let { summary, busy }: { summary: Summary | null; busy: boolean } = $props()

  let showAll = $state(false)
  const PAGE = 12
  const rows = $derived(showAll ? (summary?.urls ?? []) : (summary?.urls ?? []).slice(0, PAGE))
  const cols = ['2xx', '3xx', '4xx', '5xx'] as const
</script>

<section class="card" class:busy>
  <header class="card-head">
    <div>
      <h2 class="card-title">URL les plus appelées</h2>
      <p class="card-sub">
        {#if summary}{pluriel(summary.counts.urls, 'URL distincte', 'URL distinctes')} · identifiants remplacés par {'{id}'}{:else}&nbsp;{/if}
      </p>
    </div>
  </header>
  {#if !summary}
    <div class="sk-rows">{#each Array(6) as _, i (i)}<span class="skeleton"></span>{/each}</div>
  {:else if summary.urls.length === 0}
    <p class="empty">Aucune requête sur la période.</p>
  {:else}
    <div class="table-scroll">
      <table class="tbl">
        <thead>
          <tr>
            <th scope="col">Hôte et chemin</th>
            <th scope="col" class="num">Requêtes</th>
            <th scope="col" class="share">Répartition</th>
            {#each cols as k (k)}<th scope="col" class="num"><i class="swatch" style:background="var(--c-{k})"></i>{k}</th>{/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as u (u.host + u.path)}
            <tr>
              <td class="url"><span class="host">{u.host || '(sans hôte)'}</span><span class="mono path">{u.path}</span>{#if u.sockets > 0}<span class="ws">dont {formatCount(u.sockets)} WebSocket{u.sockets > 1 ? 's' : ''} accepté{u.sockets > 1 ? 's' : ''} (journalisé{u.sockets > 1 ? 's' : ''} sans statut)</span>{/if}</td>
              <td class="num strong">{formatCount(total(u.byClass))}</td>
              <td class="share"><ClassBar counts={u.byClass} /></td>
              {#each cols as k (k)}<td class="num" class:zero={u.byClass[k] === 0}>{formatCount(u.byClass[k])}</td>{/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if summary.urls.length > PAGE}
      <button type="button" class="more" onclick={() => (showAll = !showAll)}>
        {showAll ? 'Réduire' : `Afficher les ${summary.urls.length - PAGE} suivantes`}
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
    min-width: 560px;
  }
  .url {
    max-width: 420px;
    min-width: 200px;
  }
  .host {
    display: block;
    font-size: 12px;
    color: var(--text-3);
  }
  .path {
    display: block;
    font-weight: 500;
  }
  .ws {
    display: block;
    font-size: 12px;
    color: var(--text-3);
  }
  .strong {
    font-weight: 620;
  }
  .share {
    width: 110px;
    min-width: 90px;
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
