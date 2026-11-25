<script lang="ts">
  import { BAN_DURATIONS, banIp, fetchIp, unbanIp, type BanDuration, type IpDetail, type Period, isRange } from '../lib/api'
  import { ago, countryName, formatBytes, formatCount, formatDayTime } from '../lib/format'
  import { periodPhrase, ruleSnippet, scenarioLabel, total } from '../lib/model'
  import ClassBar from './ClassBar.svelte'
  import Icon from './Icon.svelte'
  import KindBadge from './KindBadge.svelte'
  import VerdictBadge from './VerdictBadge.svelte'

  let { ip, period, now, onclose }: { ip: string; period: Period; now: Date; onclose: () => void } = $props()

  let detail = $state.raw<IpDetail | null>(null)
  let status = $state<'loading' | 'ok' | 'absent' | 'error'>('loading')
  let kind = $state<'ip' | 'asn'>('ip')
  let copied = $state(false)
  let panel = $state<HTMLElement>()
  let reload = () => {}

  // Blocage : choix, puis confirmation explicite avant d'écrire.
  let banDuration = $state<BanDuration>('24h')
  let banReason = $state('')
  let confirming = $state<'ban' | 'unban' | null>(null)
  let busy = $state(false)
  let actionError = $state('')

  // Chargement, puis rafraîchissement toutes les 5 s tant que le panneau est ouvert
  // (sauf sur un intervalle figé : rien n'y change).
  $effect(() => {
    const target = ip
    const p = period
    let alive = true
    detail = null
    status = 'loading'
    async function load() {
      try {
        const d = await fetchIp(target, p)
        if (!alive) return
        if (d) {
          detail = d
          status = 'ok'
        } else {
          status = 'absent'
        }
      } catch {
        if (alive && !detail) status = 'error'
      }
    }
    reload = load
    confirming = null
    actionError = ''
    banReason = ''
    void load()
    const id = isRange(p) ? undefined : setInterval(load, 5000)
    return () => {
      alive = false
      clearInterval(id)
    }
  })

  $effect(() => {
    panel?.focus()
    const onkey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onclose()
    }
    document.addEventListener('keydown', onkey)
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onkey)
      document.body.style.overflow = overflow
    }
  })

  const snippet = $derived(detail ? ruleSnippet(detail, kind) : '')
  const rules = $derived(detail ? Object.entries(detail.rules).sort((a, b) => b[1] - a[1]) : [])
  const routers = $derived(detail ? Object.entries(detail.routers).sort((a, b) => b[1] - a[1]) : [])
  const named = $derived(detail?.verdict === 'connue')
  const cols = ['2xx', '3xx', '4xx', '5xx'] as const

  async function act(kind: 'ban' | 'unban') {
    if (confirming !== kind) {
      confirming = kind
      actionError = ''
      return
    }
    busy = true
    actionError = ''
    try {
      if (kind === 'ban') await banIp(ip, banDuration, banReason)
      else await unbanIp(ip)
      confirming = null
      banReason = ''
      await reload()
    } catch (e) {
      actionError = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  const durationLabel = $derived(BAN_DURATIONS.find((d) => d.value === banDuration)?.label ?? banDuration)

  async function copy() {
    try {
      await navigator.clipboard.writeText(snippet)
      copied = true
      setTimeout(() => (copied = false), 1800)
    } catch {
      // Presse-papiers indisponible (page servie en http) : le bloc reste sélectionnable.
    }
  }
</script>

<div class="backdrop" onclick={onclose} aria-hidden="true"></div>
<div class="panel" role="dialog" aria-modal="true" aria-label="Détail de l'IP {ip}" tabindex="-1" bind:this={panel}>
  <header>
    <div class="title">
      {#if detail?.name}<h2>{detail.name}</h2>{/if}
      <p class="mono ip" class:lead={!detail?.name}>{ip}</p>
    </div>
    {#if detail}<VerdictBadge verdict={detail.verdict} />{/if}
    <button type="button" class="close" onclick={onclose} aria-label="Fermer le détail"><Icon name="close" size={18} /></button>
  </header>

  <div class="body">
    {#if status === 'loading'}
      <div class="sk">{#each Array(7) as _, i (i)}<span class="skeleton"></span>{/each}</div>
    {:else if status === 'absent'}
      <p class="empty">Aucune requête de cette IP {periodPhrase(period)}.<br />Choisir une période plus longue pour la retrouver.</p>
    {:else if status === 'error' || !detail}
      <p class="empty">Détail indisponible : le tableau de bord ne répond pas.</p>
    {:else}
      <dl class="facts">
        <div><dt>Pays</dt><dd>{detail.country ? `${countryName(detail.country)} (${detail.country})` : '—'}</dd></div>
        <div><dt>Opérateur</dt><dd>{detail.org || '—'}{#if detail.asn}{' '}<span class="muted">· AS{detail.asn}</span>{/if}</dd></div>
        <div><dt>Requêtes</dt><dd>{formatCount(total(detail.byClass))} <span class="muted">{periodPhrase(period)}</span></dd></div>
        <div><dt>Volume</dt><dd>{formatBytes(detail.bytesIn)} <span class="muted">entrants</span> · {formatBytes(detail.bytesOut)} <span class="muted">sortants</span></dd></div>
      </dl>

      <div class="statusline">
        <ClassBar counts={detail.byClass} height={8} />
        <ul>
          {#each cols as k (k)}
            <li class:nil={detail.byClass[k] === 0}><i class="swatch" style:background="var(--c-{k})"></i>{k} <b>{formatCount(detail.byClass[k])}</b></li>
          {/each}
          <li class:nil={detail.byClass.abandon === 0}><i class="swatch" style:background="var(--c-abandon)"></i>abandons <b>{formatCount(detail.byClass.abandon)}</b></li>
        </ul>
      </div>

      {#if detail.ban}
        <div class="note ban">
          <Icon name="ban" size={15} />
          <div class="ban-body">
            <span>Ban CrowdSec en cours : <b>{detail.ban.scenario}</b>{#if detail.ban.until && !detail.ban.until.startsWith('0001')}, jusqu'au {formatDayTime(new Date(detail.ban.until))}{/if}.</span>
            {#if detail.block.enabled}
              <div class="actions">
                <button type="button" class="btn" class:confirm={confirming === 'unban'} disabled={busy} onclick={() => act('unban')}>
                  {confirming === 'unban' ? 'Confirmer la levée du ban' : 'Lever le ban'}
                </button>
                {#if confirming === 'unban'}<button type="button" class="link" onclick={() => (confirming = null)}>Annuler</button>{/if}
              </div>
              {#if actionError}<p class="err">{actionError}</p>{/if}
            {/if}
          </div>
        </div>
      {/if}

      {#if named}
        <p class="note"><Icon name="info" size={15} /><span>Nommée par la règle <b>« {detail.name} »</b> de <span class="mono">names.yaml</span> : toutes ses requêtes sont couvertes.</span></p>
      {:else if rules.length > 0}
        <p class="note warn"><Icon name="alert" size={15} /><span>En partie couverte par <b>« {rules[0][0]} »</b> ({formatCount(rules[0][1])} requêtes). Les autres sortent des routers ou des chemins de la règle : elles comptent comme celles d'une inconnue.</span></p>
      {:else if detail.looksLike}
        <p class="note warn"><Icon name="alert" size={15} /><span>Ressemble à <b>« {detail.looksLike} »</b> (même IP ou même opérateur), mais ses requêtes sortent des routers ou des chemins de cette règle.</span></p>
      {:else}
        <p class="note"><Icon name="info" size={15} /><span>Aucune règle de <span class="mono">names.yaml</span> ne correspond à cette IP.</span></p>
      {/if}

      {#if detail.alerts.length > 0}
        <section>
          <h3>Historique CrowdSec (7 jours)</h3>
          <ul class="raw cs">
            {#each detail.alerts as a (a.id)}
              <li>
                <KindBadge kind={a.kind} manual={a.manual} />
                <span class="req">
                  <span title={a.scenario}>{scenarioLabel(a.scenario)}</span>
                  {#if a.detail}<span class="muted"> · {a.detail}</span>{/if}
                  {#if a.host || a.paths?.length}<span class="mono muted target">{a.host ?? ''}{(a.paths ?? []).join(' ')}{#if a.more}{' '}+{a.more}{/if}</span>{/if}
                </span>
                <span class="when" title={formatDayTime(new Date(a.at))}>{ago(a.at, now)}</span>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <section>
        <h3>URL appelées</h3>
        <ul class="urls">
          {#each detail.urls as u (u.host + u.path)}
            <li>
              <span class="u"><span class="host">{u.host}</span><span class="mono">{u.path}</span></span>
              <span class="counts">
                {#each cols as k (k)}
                  {#if u.byClass[k] > 0}<span class="c"><i class="swatch" style:background="var(--c-{k})"></i>{k} <b>{formatCount(u.byClass[k])}</b></span>{/if}
                {/each}
                {#if u.sockets > 0}<span class="c ws"><i class="swatch" style:background="var(--c-2xx)"></i>WebSocket accepté <b>{formatCount(u.sockets)}</b></span>{/if}
                {#if u.byClass.abandon - u.sockets > 0}<span class="c"><i class="swatch" style:background="var(--c-abandon)"></i>abandons <b>{formatCount(u.byClass.abandon - u.sockets)}</b></span>{/if}
              </span>
            </li>
          {/each}
        </ul>
      </section>

      <section>
        <h3>Routers</h3>
        <p class="chips">
          {#each routers as [r, n] (r)}<span class="chip">{r === '(aucun)' ? 'aucun router' : r} <b>{formatCount(n)}</b></span>{/each}
        </p>
      </section>

      <section>
        <h3>User-agents</h3>
        {#if detail.userAgents.length > 0}
          <ul class="plain">{#each detail.userAgents as ua (ua)}<li class="mono">{ua}</li>{/each}</ul>
        {:else}
          <p class="muted small">Aucun user-agent relevé.</p>
        {/if}
      </section>

      {#if detail.rawPaths.length > 0}
        <section>
          <h3>Dernières requêtes non nommées</h3>
          <ul class="raw">
            {#each detail.rawPaths as h (h.time + h.path + h.status)}
              <li>
                <span class="st s{Math.floor(h.status / 100)}" title={h.status === 0 ? 'Statut 0 : client parti avant la réponse, ou WebSocket (journalisé à sa fermeture)' : undefined}>{h.status === 0 ? 'sans statut' : h.status}</span>
                <span class="mono req">{h.method} {h.host}{h.path}</span>
                <span class="when" title={formatDayTime(new Date(h.time))}>{ago(h.time, now)}</span>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if !detail.ban && !named}
        <section>
          <h3>Bloquer cette IP</h3>
          {#if !detail.block.enabled}
            <p class="small muted">
              Blocage désactivé : le service n'a pas de compte machine CrowdSec (<span class="mono">CROWDSEC_MACHINE_PASSWORD</span>).
            </p>
          {:else if detail.block.refused}
            <p class="small muted">{detail.block.refused}</p>
          {:else}
            <p class="small muted">
              Ban CrowdSec : Traefik refuse alors toutes ses requêtes (403) sur les services publics. Il expire seul et peut être levé
              ici.
            </p>
            <div class="ban-form">
              <div class="segmented" role="group" aria-label="Durée du ban">
                {#each BAN_DURATIONS as d (d.value)}
                  <button type="button" aria-pressed={banDuration === d.value} onclick={() => { banDuration = d.value; confirming = null }}>{d.label}</button>
                {/each}
              </div>
              <input type="text" maxlength="120" placeholder="Motif (facultatif)" aria-label="Motif du ban" bind:value={banReason} />
              <div class="actions">
                <button type="button" class="btn danger" class:confirm={confirming === 'ban'} disabled={busy} onclick={() => act('ban')}>
                  <Icon name="ban" size={14} />{confirming === 'ban' ? `Confirmer le ban de ${durationLabel}` : `Bannir ${durationLabel}`}
                </button>
                {#if confirming === 'ban'}<button type="button" class="link" onclick={() => (confirming = null)}>Annuler</button>{/if}
              </div>
              {#if actionError}<p class="err">{actionError}</p>{/if}
            </div>
          {/if}
        </section>
      {/if}

      {#if !named}
        <section>
          <h3>Nommer cette IP</h3>
          <p class="small muted">
            Bloc à coller sous <span class="mono">names:</span> dans
            <span class="mono">names.yaml</span> (relu à chaud dans les 10 s). Le rouge passé de cette
            IP disparaît au rechargement.
          </p>
          <div class="snippet-head">
            <div class="segmented" role="group" aria-label="Type de règle">
              <button type="button" aria-pressed={kind === 'ip'} onclick={() => (kind = 'ip')}>Par IP</button>
              <button type="button" aria-pressed={kind === 'asn'} disabled={!detail.asn} onclick={() => (kind = 'asn')}>Par opérateur</button>
            </div>
            <button type="button" class="copy" onclick={copy}><Icon name={copied ? 'check' : 'copy'} size={14} />{copied ? 'Copié' : 'Copier'}</button>
          </div>
          <pre class="mono">{snippet}</pre>
          {#if kind === 'asn'}
            <p class="small muted">Une règle par opérateur couvre tous ses clients : la laisser restreinte à ces routers.</p>
          {/if}
        </section>
      {/if}
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 20;
    background: rgb(10 10 14 / 0.45);
    animation: fade 0.18s ease;
  }
  .panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    z-index: 21;
    width: min(520px, 100vw);
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border-left: 1px solid var(--border);
    box-shadow: var(--shadow-lg);
    animation: slide 0.22s cubic-bezier(0.2, 0.8, 0.2, 1);
    outline: none;
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  @keyframes slide {
    from {
      transform: translateX(24px);
      opacity: 0;
    }
  }
  header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 12px 14px 20px;
    border-bottom: 1px solid var(--border);
  }
  .title {
    flex: 1;
    min-width: 0;
  }
  h2 {
    font-size: 17px;
    font-weight: 680;
    letter-spacing: -0.015em;
  }
  .ip {
    color: var(--text-2);
    font-size: 13px;
    word-break: break-all;
  }
  .ip.lead {
    color: var(--text);
    font-size: 16px;
    font-weight: 650;
  }
  .close {
    flex: none;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 8px;
    background: none;
    color: var(--text-2);
    cursor: pointer;
  }
  .close:hover {
    background: var(--surface-3);
    color: var(--text);
  }
  .body {
    flex: 1;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 16px 20px calc(28px + env(safe-area-inset-bottom));
    display: grid;
    gap: 16px;
    align-content: start;
  }
  .facts {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
    margin: 0;
  }
  dt {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  dd {
    margin: 2px 0 0;
    font-weight: 550;
  }
  dd .muted {
    font-weight: 400;
  }
  .statusline ul {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .statusline b {
    color: var(--text);
  }
  .nil {
    opacity: 0.45;
  }
  .note {
    display: flex;
    gap: 9px;
    padding: 10px 12px;
    border-radius: var(--radius-sm);
    background: var(--info-bg);
    border: 1px solid var(--info-border);
    color: var(--info);
    font-size: 13px;
  }
  .note span {
    color: var(--text);
  }
  .note :global(svg) {
    margin-top: 2px;
  }
  .note.warn {
    background: var(--warn-bg);
    border-color: var(--warn-border);
    color: var(--warn);
  }
  .note.ban {
    background: var(--surface-2);
    border-color: var(--border-strong);
    color: var(--text-2);
  }
  h3 {
    margin-bottom: 7px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .urls li {
    display: grid;
    gap: 3px;
    padding: 8px 0;
    border-top: 1px solid var(--border);
    font-size: 13px;
  }
  .urls li:first-child {
    border-top: 0;
    padding-top: 0;
  }
  .host {
    display: block;
    font-size: 12px;
    color: var(--text-3);
  }
  .counts {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 12px;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .counts b {
    color: var(--text);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .chip {
    padding: 2px 10px;
    border-radius: 999px;
    background: var(--surface-3);
    font-size: 12.5px;
    color: var(--text-2);
  }
  .chip b {
    color: var(--text);
  }
  .plain li {
    padding: 3px 0;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .small {
    font-size: 12.5px;
  }
  .raw li {
    display: grid;
    grid-template-columns: 74px minmax(0, 1fr) auto;
    gap: 8px;
    align-items: baseline;
    padding: 5px 0;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
  }
  .raw li:first-child {
    border-top: 0;
  }
  .raw.cs li {
    grid-template-columns: 74px minmax(0, 1fr) auto;
  }
  .target {
    display: block;
    font-size: 12px;
    word-break: break-all;
  }
  .st {
    font-weight: 650;
    color: var(--text-2);
  }
  .st.s2 {
    color: var(--good);
  }
  .st.s4 {
    color: var(--warn);
  }
  .st.s5 {
    color: var(--bad);
  }
  .req {
    color: var(--text);
  }
  .when {
    color: var(--text-3);
    white-space: nowrap;
  }
  .snippet-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin: 10px 0 6px;
  }
  .segmented button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .copy {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--surface);
    font-size: 13px;
    font-weight: 550;
    cursor: pointer;
  }
  .copy:hover {
    background: var(--surface-2);
  }
  pre {
    margin: 0 0 6px;
    padding: 12px 14px;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    border: 1px solid var(--border);
    font-size: 12.5px;
    line-height: 1.6;
    white-space: pre-wrap;
    user-select: all;
  }
  .ban-body {
    display: grid;
    gap: 8px;
    min-width: 0;
  }
  .ban-form {
    display: grid;
    gap: 10px;
    margin-top: 10px;
    justify-items: start;
  }
  .ban-form input {
    width: 100%;
    padding: 7px 10px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 13px;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
    font-weight: 550;
    cursor: pointer;
  }
  .btn:hover {
    background: var(--surface-2);
  }
  .btn:disabled {
    opacity: 0.55;
    cursor: progress;
  }
  .btn.danger {
    color: var(--bad);
    border-color: var(--bad-border);
  }
  .btn.danger:hover {
    background: var(--bad-bg);
  }
  .btn.confirm,
  .btn.danger.confirm {
    background: var(--bad-solid);
    border-color: var(--bad-solid);
    color: #fff;
  }
  .link {
    border: 0;
    background: none;
    padding: 0;
    color: var(--text-2);
    font-size: 13px;
    text-decoration: underline;
    cursor: pointer;
  }
  .err {
    color: var(--bad);
    font-size: 12.5px;
  }
  .sk {
    display: grid;
    gap: 12px;
  }
  .sk .skeleton {
    height: 34px;
  }
  @media (max-width: 720px) {
    .panel {
      width: 100vw;
      border-left: 0;
    }
    header {
      padding-left: 16px;
    }
    .body {
      padding-left: 16px;
      padding-right: 16px;
    }
  }
</style>
