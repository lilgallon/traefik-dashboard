<script lang="ts">
  import type { Hit } from '../lib/api'
  import { countryName, formatClock, formatDuration } from '../lib/format'
  import { hitClass, hitStatus, visibleHits } from '../lib/model'
  import Icon from './Icon.svelte'

  // Les dernières requêtes lues dans access.log, au fil de l'eau. Le flux les
  // pousse chaque seconde ; rien n'est rechargé.
  let { hits, connected, onselect }: { hits: Hit[]; connected: boolean; onselect: (ip: string) => void } = $props()

  const ROWS = 10
  const KEY = 'td-feed-unknown'

  function remembered(): boolean {
    try {
      return localStorage.getItem(KEY) === '1'
    } catch {
      return false
    }
  }

  let unknownOnly = $state(remembered())
  // En pause, la liste est figée sur ce qu'elle montrait ; le flux continue derrière.
  let frozen = $state.raw<Hit[] | null>(null)

  const source = $derived(frozen ?? hits)
  const rows = $derived(visibleHits(source, unknownOnly, ROWS))
  const waiting = $derived.by(() => {
    if (!frozen) return 0
    const top = frozen.length > 0 ? frozen[0].seq : 0
    return hits.reduce((n, h) => n + (h.seq > top && !(unknownOnly && h.name) ? 1 : 0), 0)
  })

  function setFilter(v: boolean) {
    unknownOnly = v
    try {
      localStorage.setItem(KEY, v ? '1' : '0')
    } catch {
      // stockage bloqué : sans importance
    }
  }
  function togglePause() {
    frozen = frozen ? null : hits
  }
  function shortIp(ip: string): string {
    return ip.length > 24 ? ip.slice(0, 12) + '…' + ip.slice(-8) : ip
  }
  function describe(h: Hit): string {
    const who = h.name || `IP inconnue ${h.ip}`
    const status = h.socket ? 'WebSocket accepté' : h.status > 0 ? `statut ${h.status}` : 'abandon'
    return `${who}, ${h.method} ${h.host}${h.path}, ${status}${h.red ? ', succès d’une IP inconnue' : ''}. Ouvrir le détail de l’IP.`
  }
</script>

<section class="card" aria-labelledby="feed-title">
  <header class="card-head">
    <div>
      <h2 class="card-title" id="feed-title">
        <span class="dot" class:on={connected && !frozen} aria-hidden="true"></span>Dernières requêtes
      </h2>
      <p class="card-sub">
        {#if frozen}
          En pause{#if waiting > 0}&nbsp;· {waiting}{waiting >= 200 ? ' ou plus' : ''} en attente{/if}
        {:else if !connected}
          Flux interrompu, reconnexion en cours
        {:else}
          En direct · cliquer une ligne pour le détail de l'IP
        {/if}
      </p>
    </div>
    <div class="tools">
      <div class="segmented" role="group" aria-label="Filtrer les requêtes">
        <button type="button" aria-pressed={!unknownOnly} onclick={() => setFilter(false)}>Toutes</button>
        <button type="button" aria-pressed={unknownOnly} onclick={() => setFilter(true)}>Inconnues</button>
      </div>
      <button type="button" class="pause" class:active={frozen !== null} onclick={togglePause} aria-pressed={frozen !== null}
        title={frozen ? 'Reprendre le défilement' : 'Mettre en pause'} aria-label={frozen ? 'Reprendre le défilement' : 'Mettre en pause'}>
        <Icon name={frozen ? 'play' : 'pause'} size={15} />
      </button>
    </div>
  </header>

  {#if rows.length === 0}
    <p class="empty">
      {#if source.length === 0}
        {connected ? 'Aucune requête lue pour le moment.' : 'En attente du flux en direct…'}
      {:else}
        Aucune IP inconnue parmi les {source.length} dernières requêtes.
      {/if}
    </p>
  {:else}
    <ol class="feed" style:--rows={ROWS}>
      {#each rows as h (h.seq)}
        <li>
          <button type="button" class="row" class:red={h.red} onclick={() => onselect(h.ip)} aria-label={describe(h)}>
            <span class="time mono">{formatClock(new Date(h.time))}</span>
            <span class="status" title={h.socket ? 'WebSocket accepté (journalisé sans statut)' : h.status === 0 ? 'Le client est parti avant la réponse' : undefined}>
              <i class="swatch" style:background="var(--c-{hitClass(h) === '1xx' ? 'abandon' : hitClass(h)})"></i>{hitStatus(h)}
            </span>
            <span class="who">
              {#if h.name}
                <span class="name">{h.name}</span>
              {:else}
                <span class="ip mono">{shortIp(h.ip)}</span>
                {#if h.country}<span class="cc" title={countryName(h.country)}>{h.country}</span>{/if}
              {/if}
              {#if h.red}<span class="flag">succès</span>{/if}
            </span>
            <span class="req">
              <span class="method">{h.method}</span>
              <span class="host">{h.host || '(sans hôte)'}</span><span class="path mono">{h.path}</span>
            </span>
            <span class="dur">{formatDuration(h.ms)}</span>
          </button>
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .card-title {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-3);
  }
  .dot.on {
    background: var(--good-solid);
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--good-solid) 55%, transparent);
    animation: beat 2s ease-out infinite;
  }
  @keyframes beat {
    to {
      box-shadow: 0 0 0 7px transparent;
    }
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .pause {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border-radius: 9px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-2);
    cursor: pointer;
  }
  .pause:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
  .pause.active {
    background: var(--accent-bg);
    border-color: var(--accent);
    color: var(--accent);
  }

  .feed {
    list-style: none;
    margin: 0;
    padding: 0;
    border-top: 1px solid var(--border);
    /* Hauteur réservée : la page ne saute pas pendant que la liste se remplit. */
    min-height: calc(var(--rows) * 35px);
    overflow: hidden;
    border-radius: 0 0 var(--radius) var(--radius);
  }
  li + li {
    border-top: 1px solid var(--border);
  }
  li {
    animation: arrive 0.45s ease-out;
  }
  @keyframes arrive {
    from {
      opacity: 0;
      background: var(--accent-bg);
      transform: translateY(-6px);
    }
  }
  .row {
    display: grid;
    grid-template-columns: 66px 74px minmax(120px, 210px) minmax(0, 1fr) 62px;
    align-items: center;
    gap: 12px;
    width: 100%;
    height: 34px;
    padding: 0 16px;
    border: 0;
    border-left: 3px solid transparent;
    background: none;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--surface-2);
  }
  .row:focus-visible {
    outline-offset: -2px;
  }
  .row.red {
    border-left-color: var(--bad-solid);
    background: var(--bad-bg);
  }
  .row > span {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .time,
  .dur {
    color: var(--text-3);
  }
  .dur {
    text-align: right;
  }
  .status {
    font-weight: 600;
  }
  .who {
    display: flex;
    align-items: baseline;
    gap: 6px;
  }
  .name {
    font-weight: 550;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ip {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cc {
    flex: none;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-3);
  }
  .flag {
    flex: none;
    padding: 0 6px;
    border-radius: 999px;
    border: 1px solid var(--bad-border);
    color: var(--bad);
    font-size: 11px;
    font-weight: 600;
    line-height: 17px;
  }
  .method {
    display: inline-block;
    min-width: 34px;
    margin-right: 6px;
    font-size: 11px;
    font-weight: 650;
    letter-spacing: 0.03em;
    color: var(--text-3);
  }
  .host {
    color: var(--text-3);
  }
  .path {
    font-weight: 500;
  }

  @media (max-width: 720px) {
    .feed {
      min-height: calc(var(--rows) * 55px);
    }
    .row {
      grid-template-columns: auto auto minmax(0, 1fr) auto;
      grid-template-areas: 'time status who dur' 'req req req req';
      gap: 2px 10px;
      height: 54px;
      padding: 0 12px 0 9px;
      align-content: center;
    }
    .time {
      grid-area: time;
    }
    .status {
      grid-area: status;
    }
    .who {
      grid-area: who;
    }
    .dur {
      grid-area: dur;
    }
    .req {
      grid-area: req;
      font-size: 12.5px;
    }
    .method {
      min-width: 0;
    }
  }
</style>
