<script lang="ts">
  import type { Period, Summary } from '../lib/api'
  import { ago, countryName, formatClock, formatCount } from '../lib/format'
  import { t, tn } from '../lib/i18n.svelte'
  import { capitalize, periodPhrase, type DataState } from '../lib/model'
  import Icon from './Icon.svelte'

  let {
    mode,
    summary,
    period,
    now,
    onselect,
  }: { mode: DataState; summary: Summary | null; period: Period; now: Date; onselect: (ip: string) => void } = $props()

  const offenders = $derived(summary?.verdict.offenders ?? [])
  const red = $derived(mode !== 'loading' && mode !== 'backfill' && (summary?.verdict.red ?? false))
  const tone = $derived(
    red ? 'bad' : mode === 'live' ? 'good' : mode === 'stale' || mode === 'offline' ? 'warn' : 'idle',
  )
  const successes = $derived(offenders.reduce((n, o) => n + o.count, 0))
</script>

<section class="banner {tone}" aria-live="polite" aria-label={t('banner.aria')}>
  <div class="head">
    <div class="icon" aria-hidden="true">
      {#if red}
        <Icon name="alert" size={24} />
      {:else if mode === 'live'}
        <Icon name="check" size={24} />
      {:else if mode === 'offline'}
        <Icon name="offline" size={24} />
      {:else if mode === 'stale'}
        <Icon name="clock" size={24} />
      {:else}
        <span class="spinner"></span>
      {/if}
    </div>
    <div class="text">
      {#if red}
        <h2>{tn('banner.red.title', offenders.length)}</h2>
        <p>
          {tn('banner.red.text', successes, { period: periodPhrase(period) })}
          {#if mode === 'stale'}{t('banner.red.stale')}{/if}
          {#if mode === 'offline'}{t('banner.red.offline', { time: formatClock(new Date(summary?.generatedAt ?? 0)) })}{/if}
        </p>
      {:else if mode === 'live'}
        <h2>{t('banner.ok.title')}</h2>
        <p>
          {#if (summary?.counts.unknownIps ?? 0) === 0}
            {t('banner.ok.none', { period: capitalize(periodPhrase(period)) })}
          {:else if summary?.counts.unknownIps === 1}
            {t('banner.ok.one', { period: capitalize(periodPhrase(period)) })}
          {:else}
            {t('banner.ok.many', { period: capitalize(periodPhrase(period)), n: formatCount(summary?.counts.unknownIps ?? 0) })}
          {/if}
        </p>
      {:else if mode === 'stale'}
        <h2>{t('banner.stale.title')}</h2>
        <p>
          {t('banner.stale.text1')} <span class="mono">access.log</span>
          {t('banner.stale.text2', { ago: ago(summary?.lastLine ?? '', now) })}
        </p>
      {:else if mode === 'offline'}
        <h2>{t('banner.offline.title')}</h2>
        <p>
          {#if summary}{t('banner.offline.since', { time: formatClock(new Date(summary.generatedAt)) })}{/if}
          {t('banner.offline.retry')}
        </p>
      {:else if mode === 'backfill'}
        <h2>{t('banner.backfill.title')}</h2>
        <p>{t('banner.backfill.text')}</p>
      {:else}
        <h2>{t('banner.loading.title')}</h2>
        <p>{t('banner.loading.text')}</p>
      {/if}
    </div>
  </div>

  {#if red}
    <ul class="offenders">
      {#each offenders as o (o.ip)}
        <li>
          <button type="button" onclick={() => onselect(o.ip)} aria-label={t('common.ipDetail', { ip: o.ip })}>
            <span class="who">
              <span class="ip mono">{o.ip}</span>
              <span class="meta">
                {#if o.country}<span title={countryName(o.country)}>{o.country}</span>{:else}<span>{t('common.unknownCountry')}</span>{/if}
                <span aria-hidden="true">·</span>
                <span>{o.org || t('common.unknownOrg')}</span>
                {#if o.looksLike}<span class="looks">{t('common.looksLike', { name: o.looksLike })}</span>{/if}
              </span>
            </span>
            <span class="urls">
              {#each o.urls.slice(0, 3) as u (u)}<span class="mono url">{u}</span>{/each}
              {#if o.urls.length > 3}<span class="extra">{t('banner.moreUrls', { n: o.urls.length - 3 })}</span>{/if}
            </span>
            <span class="count"><strong>{formatCount(o.count)}</strong> {tn('banner.successes', o.count)}</span>
            <span class="go" aria-hidden="true"><Icon name="chevron" size={16} /></span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .banner {
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--surface);
    box-shadow: var(--shadow);
    overflow: hidden;
    transition: background-color 0.3s, border-color 0.3s;
  }
  .head {
    display: flex;
    gap: 14px;
    align-items: center;
    padding: 16px 18px;
  }
  .icon {
    flex: none;
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    color: #fff;
    background: var(--text-3);
    transition: background-color 0.3s;
  }
  h2 {
    font-size: 18px;
    font-weight: 680;
    letter-spacing: -0.015em;
    line-height: 1.25;
  }
  p {
    margin-top: 3px;
    color: var(--text-2);
    font-size: 13.5px;
  }

  .good {
    background: var(--good-bg);
    border-color: var(--good-border);
  }
  .good .icon {
    background: var(--good-solid);
  }
  .good h2 {
    color: var(--good);
  }
  .bad {
    background: var(--bad-bg);
    border-color: var(--bad-solid);
    box-shadow: 0 0 0 1px var(--bad-solid), var(--shadow);
  }
  .bad .icon {
    background: var(--bad-solid);
  }
  .bad h2 {
    color: var(--bad);
  }
  .warn {
    background: var(--warn-bg);
    border-color: var(--warn-border);
  }
  .warn .icon {
    background: var(--warn-solid);
  }
  .warn h2 {
    color: var(--warn);
  }
  .idle .icon {
    background: var(--surface-3);
    color: var(--text-2);
  }

  .spinner {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    border: 2.5px solid var(--border-strong);
    border-top-color: var(--text-2);
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .offenders {
    list-style: none;
    margin: 0;
    padding: 0;
    border-top: 1px solid var(--bad-border);
    background: var(--surface);
  }
  .offenders li + li {
    border-top: 1px solid var(--border);
  }
  .offenders button {
    display: grid;
    grid-template-columns: minmax(180px, 1.1fr) minmax(0, 2fr) auto 16px;
    gap: 6px 16px;
    align-items: center;
    width: 100%;
    padding: 11px 18px;
    border: 0;
    background: none;
    text-align: left;
    cursor: pointer;
    border-radius: 0;
  }
  .offenders button:hover {
    background: var(--surface-2);
  }
  .who {
    display: grid;
    gap: 1px;
    min-width: 0;
  }
  .ip {
    font-weight: 650;
    font-size: 14px;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 6px;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .looks {
    color: var(--warn);
    font-weight: 550;
  }
  .urls {
    display: grid;
    gap: 2px;
    min-width: 0;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .url {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .extra {
    color: var(--text-3);
  }
  .count {
    white-space: nowrap;
    color: var(--bad);
    font-size: 13px;
  }
  .count strong {
    font-size: 16px;
  }
  .go {
    color: var(--text-3);
  }

  @media (max-width: 720px) {
    .head {
      align-items: flex-start;
      padding: 14px;
    }
    .icon {
      width: 36px;
      height: 36px;
    }
    h2 {
      font-size: 16px;
    }
    .offenders button {
      grid-template-columns: minmax(0, 1fr) auto 16px;
      padding: 11px 14px;
    }
    .urls {
      grid-column: 1 / -1;
      grid-row: 2;
    }
  }
</style>
