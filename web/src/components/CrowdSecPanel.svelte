<script lang="ts">
  import { unbanIp, type Period, type Summary } from '../lib/api'
  import { ago, countryName, formatCount, formatDayTime, remaining } from '../lib/format'
  import { t, tn } from '../lib/i18n.svelte'
  import { periodPhrase, scenarioLabel } from '../lib/model'
  import Icon from './Icon.svelte'
  import KindBadge from './KindBadge.svelte'

  let {
    summary,
    period,
    now,
    busy,
    onselect,
    onchange,
  }: {
    summary: Summary | null
    period: Period
    now: Date
    busy: boolean
    onselect: (ip: string) => void
    onchange: () => void
  } = $props()

  const cs = $derived(summary?.crowdsec ?? null)
  const BANS_SHOWN = 8
  let allBans = $state(false)
  const bans = $derived(cs ? (allBans ? cs.bans : cs.bans.slice(0, BANS_SHOWN)) : [])

  // Levée d'un ban : deux clics sur la même ligne.
  let confirmIp = $state<string | null>(null)
  let pending = $state<string | null>(null)
  let unbanError = $state('')
  async function unban(ip: string) {
    if (confirmIp !== ip) {
      confirmIp = ip
      unbanError = ''
      return
    }
    pending = ip
    try {
      await unbanIp(ip)
      confirmIp = null
      onchange()
    } catch (e) {
      unbanError = `${ip} : ${e instanceof Error ? e.message : String(e)}`
    } finally {
      pending = null
    }
  }

  const isIp = (s: string) => !s.includes('/')
</script>

<section class="card" class:busy aria-labelledby="cs-title">
  <header class="card-head">
    <div>
      <h2 class="card-title" id="cs-title">CrowdSec</h2>
      <p class="card-sub">{t('cs.sub', { period: periodPhrase(period) })}</p>
    </div>
  </header>

  {#if !summary}
    <div class="sk-rows">{#each Array(4) as _, i (i)}<span class="skeleton"></span>{/each}</div>
  {:else if !cs}
    <p class="empty">{t('cs.unavailable')}</p>
  {:else}
    {#if !cs.enabled}
      <p class="note"><Icon name="info" size={15} /><span>{t('cs.noMachine', { env: 'CROWDSEC_MACHINE_PASSWORD' })}</span></p>
    {:else if cs.error}
      <p class="note warn"><Icon name="alert" size={15} /><span>{t('cs.alertsError', { error: cs.error })}</span></p>
    {/if}

    <dl class="stats">
      <div>
        <dt>{t('cs.currentBans')}</dt>
        <dd>{formatCount(cs.bans.length)}</dd>
        <p>{tn('cs.manualCount', cs.bans.filter((b) => b.manual).length)}</p>
      </div>
      <div>
        <dt>{t('cs.bansSet')}</dt>
        <dd>{cs.enabled ? formatCount(cs.counts.bans) : '—'}</dd>
        <p>{cs.enabled ? t('cs.ofWhich', { x: tn('cs.manualCount', cs.counts.manual) }) : periodPhrase(period)}</p>
      </div>
      <div>
        <dt>{t('cs.waf')}</dt>
        <dd>{cs.enabled ? formatCount(cs.counts.waf) : '—'}</dd>
        <p>{t('cs.wafSub')}</p>
      </div>
      <div>
        <dt>{t('cs.reported')}</dt>
        <dd>{cs.enabled ? formatCount(cs.counts.ips) : '—'}</dd>
        <p>{cs.enabled && cs.counts.other > 0 ? t('cs.noDecision', { x: tn('common.alerts', cs.counts.other) }) : t('cs.distinct')}</p>
      </div>
      <div title={cs.blocklist ? t('cs.blocklistTitle', { community: formatCount(cs.blocklist.community), lists: formatCount(cs.blocklist.lists), ago: ago(cs.blocklist.at, now) }) : undefined}>
        <dt>{t('cs.blocklist')}</dt>
        <dd>{cs.blocklist ? formatCount(cs.blocklist.community + cs.blocklist.lists) : '—'}</dd>
        <p>{cs.blocklist ? t('cs.blocklistSub') : t('cs.notRead')}</p>
      </div>
    </dl>

    <div class="cols">
      <div class="col">
        <h3>{t('cs.currentBans')}</h3>
        {#if cs.bans.length === 0}
          <p class="none">{t('cs.noBans')}</p>
        {:else}
          <ul class="bans">
            {#each bans as b (b.ip)}
              <li>
                <div class="who">
                  <button type="button" class="ip mono" onclick={() => onselect(b.ip)}>{b.ip}</button>
                  <span class="where">
                    {#if b.country}<span title={countryName(b.country)}>{b.country}</span>{/if}{#if b.country && b.org}{' · '}{/if}{b.org}
                  </span>
                </div>
                <div class="why">
                  <KindBadge kind="ban" manual={b.manual} />
                  <span class="sc" title={b.scenario}>{scenarioLabel(b.scenario)}</span>
                </div>
                <div class="end">
                  <span class="until" title={b.until ? formatDayTime(new Date(b.until)) : undefined}>{remaining(b.until, now)}</span>
                  {#if cs.enabled}
                    <button type="button" class="lift" class:confirm={confirmIp === b.ip} disabled={pending === b.ip} onclick={() => unban(b.ip)}>
                      {confirmIp === b.ip ? t('cs.confirm') : t('cs.lift')}
                    </button>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
          {#if unbanError}<p class="err">{unbanError}</p>{/if}
          {#if cs.bans.length > BANS_SHOWN}
            <button type="button" class="more" onclick={() => (allBans = !allBans)}>
              {allBans ? t('common.collapse') : t('cs.seeAll', { n: cs.bans.length })}
            </button>
          {/if}
        {/if}
      </div>

      <div class="col">
        <h3>{t('cs.scenarios')}</h3>
        {#if !cs.enabled}
          <p class="none">—</p>
        {:else if cs.scenarios.length === 0}
          <p class="none">{t('cs.noAlerts', { period: periodPhrase(period) })}</p>
        {:else}
          <ul class="scen">
            {#each cs.scenarios as s (s.scenario + s.kind)}
              <li>
                <div class="sname">
                  <span class="sc" title={s.scenario}>{scenarioLabel(s.scenario)}</span>
                  <KindBadge kind={s.kind} />
                </div>
                {#if s.detail}<p class="detail">{s.detail}</p>{/if}
                <p class="facts">
                  <b>{tn(s.kind === 'waf' ? 'common.requests' : 'common.alerts', s.alerts)}</b>
                  · {tn('common.ips', s.ips)}
                  {#if s.hosts.length > 0}· {s.hosts.join(', ')}{/if}
                  · <span title={formatDayTime(new Date(s.last))}>{ago(s.last, now)}</span>
                </p>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </div>

    {#if cs.enabled && cs.recent.length > 0}
      <div class="recent">
        <h3>{t('cs.recent')}</h3>
        <ul>
          {#each cs.recent as a (a.id)}
            <li>
              <span class="when" title={formatDayTime(new Date(a.at))}>{ago(a.at, now)}</span>
              <KindBadge kind={a.kind} manual={a.manual} />
              <span class="src">
                {#if isIp(a.ip)}<button type="button" class="ip mono" onclick={() => onselect(a.ip)}>{a.ip}</button>{:else}<span class="mono">{a.ip}</span>{/if}
                {#if a.country}<span class="cc" title={countryName(a.country)}>{a.country}</span>{/if}
              </span>
              <span class="what">
                <span class="sc" title={a.scenario}>{scenarioLabel(a.scenario)}</span>
                {#if a.host || a.paths?.length}
                  <span class="mono target">{a.host ?? ''}{a.paths?.[0] ?? ''}{#if (a.paths?.length ?? 0) > 1 || a.more}{' '}<span class="plus">+{(a.paths?.length ?? 1) - 1 + (a.more ?? 0)}</span>{/if}</span>
                {/if}
              </span>
            </li>
          {/each}
        </ul>
      </div>
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
  .note {
    display: flex;
    gap: 9px;
    margin: 0 16px 10px;
    padding: 9px 12px;
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
    flex: none;
    margin-top: 2px;
  }
  .note.warn {
    background: var(--warn-bg);
    border-color: var(--warn-border);
    color: var(--warn);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    margin: 0;
    border-top: 1px solid var(--border);
  }
  .stats > div {
    padding: 12px 16px;
    border-left: 1px solid var(--border);
    min-width: 0;
  }
  .stats > div:first-child {
    border-left: 0;
  }
  dt {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  dd {
    margin: 3px 0 0;
    font-size: 22px;
    font-weight: 680;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }
  .stats p {
    font-size: 12px;
    color: var(--text-3);
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    border-top: 1px solid var(--border);
  }
  .col {
    padding: 12px 16px 14px;
    min-width: 0;
  }
  .col + .col {
    border-left: 1px solid var(--border);
  }
  h3 {
    margin-bottom: 8px;
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
  .none {
    font-size: 13px;
    color: var(--text-3);
  }
  .ip {
    padding: 0;
    border: 0;
    background: none;
    color: var(--text);
    font-size: 12.5px;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
    word-break: break-all;
  }
  .ip:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .sc {
    font-weight: 550;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .bans li {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr) auto;
    gap: 4px 12px;
    align-items: center;
    padding: 7px 0;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
  }
  .bans li:first-child {
    border-top: 0;
    padding-top: 0;
  }
  .who {
    display: grid;
    min-width: 0;
  }
  .where {
    color: var(--text-3);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .why {
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
  }
  .end {
    display: flex;
    align-items: center;
    gap: 8px;
    justify-self: end;
  }
  .until {
    color: var(--text-2);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  .lift {
    padding: 3px 9px;
    border: 1px solid var(--border-strong);
    border-radius: 7px;
    background: var(--surface);
    color: var(--text-2);
    font-size: 12px;
    font-weight: 550;
    cursor: pointer;
  }
  .lift:hover {
    color: var(--text);
    background: var(--surface-2);
  }
  .lift.confirm {
    background: var(--bad-solid);
    border-color: var(--bad-solid);
    color: #fff;
  }
  .lift:disabled {
    opacity: 0.55;
    cursor: progress;
  }
  .err {
    margin-top: 6px;
    color: var(--bad);
    font-size: 12.5px;
  }
  .more {
    margin-top: 6px;
    padding: 4px 0;
    border: 0;
    background: none;
    color: var(--accent);
    font-size: 12.5px;
    font-weight: 550;
    cursor: pointer;
  }
  .scen li {
    padding: 7px 0;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
  }
  .scen li:first-child {
    border-top: 0;
    padding-top: 0;
  }
  .sname {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .detail {
    color: var(--text-2);
  }
  .facts {
    color: var(--text-3);
  }
  .facts b {
    color: var(--text);
    font-weight: 600;
  }
  .recent {
    padding: 12px 16px 14px;
    border-top: 1px solid var(--border);
  }
  .recent li {
    display: grid;
    grid-template-columns: 96px 74px minmax(0, 170px) minmax(0, 1fr);
    gap: 10px;
    align-items: center;
    padding: 5px 0;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
  }
  .recent li:first-child {
    border-top: 0;
  }
  .when {
    color: var(--text-3);
    white-space: nowrap;
  }
  .src {
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
  }
  .cc {
    color: var(--text-3);
    font-size: 12px;
  }
  .what {
    display: flex;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
  }
  .target {
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .plus {
    color: var(--text-3);
  }
  .sk-rows {
    display: grid;
    gap: 10px;
    padding: 6px 16px 16px;
  }
  .sk-rows .skeleton {
    height: 34px;
  }
  @media (max-width: 1000px) {
    .stats {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
    .stats > div:nth-child(4) {
      border-left: 0;
    }
    .stats > div:nth-child(n + 4) {
      border-top: 1px solid var(--border);
    }
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
    .col + .col {
      border-left: 0;
      border-top: 1px solid var(--border);
    }
  }
  @media (max-width: 720px) {
    .stats {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .stats > div:nth-child(4) {
      border-left: 1px solid var(--border);
    }
    .stats > div:nth-child(odd) {
      border-left: 0;
    }
    .stats > div:nth-child(n + 3) {
      border-top: 1px solid var(--border);
    }
    .bans li {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .why {
      grid-column: 1 / -1;
      grid-row: 2;
    }
    .recent li {
      grid-template-columns: auto auto minmax(0, 1fr);
    }
    .what {
      grid-column: 1 / -1;
    }
  }
</style>
