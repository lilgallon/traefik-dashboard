<script lang="ts">
  import type { Summary } from '../lib/api'
  import { formatCount, formatMs } from '../lib/format'

  let { summary, busy }: { summary: Summary | null; busy: boolean } = $props()

  const max = $derived(Math.max(1, ...(summary?.services ?? []).map((s) => s.requests)))
</script>

<section class="card" class:busy>
  <header class="card-head">
    <div>
      <h2 class="card-title">Services</h2>
      <p class="card-sub">routers Traefik · temps de réponse au 95ᵉ centile</p>
    </div>
  </header>
  {#if !summary}
    <div class="sk-rows">{#each Array(5) as _, i (i)}<span class="skeleton"></span>{/each}</div>
  {:else if summary.services.length === 0}
    <p class="empty">Aucune requête sur la période.</p>
  {:else}
    <div class="table-scroll">
      <table class="tbl">
        <thead>
          <tr>
            <th scope="col">Router</th>
            <th scope="col" class="num">Requêtes</th>
            <th scope="col" class="num">5xx</th>
            <th scope="col" class="num">p95</th>
          </tr>
        </thead>
        <tbody>
          {#each summary.services as s (s.router)}
            <tr>
              <td>
                <span class="name" class:none={s.router.startsWith('(')}>{s.router === '(aucun)' ? 'aucun router' : s.router}</span>
                <span class="meter" aria-hidden="true"><i style:width="{Math.max(1.5, (s.requests / max) * 100)}%"></i></span>
              </td>
              <td class="num strong">{formatCount(s.requests)}</td>
              <td class="num" class:zero={s.err5xx === 0} class:err={s.err5xx > 0}>{formatCount(s.err5xx)}</td>
              <td class="num">{formatMs(s.p95Ms)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<style>
  .card {
    transition: opacity 0.2s;
  }
  .busy {
    opacity: 0.55;
  }
  .name {
    display: block;
    font-weight: 550;
  }
  .name.none {
    font-weight: 400;
    color: var(--text-2);
    font-style: italic;
  }
  .meter {
    display: block;
    height: 4px;
    margin-top: 5px;
    border-radius: 999px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .meter i {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    opacity: 0.75;
    transition: width 0.4s ease;
  }
  .strong {
    font-weight: 620;
  }
  .err {
    color: var(--bad);
    font-weight: 650;
  }
  .sk-rows {
    display: grid;
    gap: 10px;
    padding: 6px 16px 16px;
  }
  .sk-rows .skeleton {
    height: 34px;
  }
</style>
