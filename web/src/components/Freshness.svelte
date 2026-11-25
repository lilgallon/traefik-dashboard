<script lang="ts">
  import { ago } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import type { DataState } from '../lib/model'

  let { mode, connected, lastLine, now }: { mode: DataState; connected: boolean; lastLine: string; now: Date } = $props()

  const view = $derived.by(() => {
    switch (mode) {
      case 'loading':
        return { tone: 'idle', label: t('fresh.loading'), detail: '' }
      case 'offline':
        return { tone: 'bad', label: t('fresh.offline'), detail: t('fresh.offlineDetail') }
      case 'backfill':
        return { tone: 'idle', label: t('fresh.backfill'), detail: '' }
      case 'stale':
        return { tone: 'bad', label: t('fresh.stale'), detail: t('fresh.lastLine', { ago: ago(lastLine, now) }) }
      default:
        return connected
          ? { tone: 'live', label: t('fresh.live'), detail: t('fresh.lastLine', { ago: ago(lastLine, now) }) }
          : { tone: 'warn', label: t('fresh.interrupted'), detail: t('fresh.reconnect', { ago: ago(lastLine, now) }) }
    }
  })
</script>

<div class="fresh {view.tone}" role="status">
  <span class="dot" aria-hidden="true"></span>
  <strong>{view.label}</strong>
  {#if view.detail}<span class="detail">{view.detail}</span>{/if}
</div>

<style>
  .fresh {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 13px;
    color: var(--text-2);
    min-width: 0;
  }
  strong {
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
  }
  .detail {
    color: var(--text-3);
  }
  .dot {
    position: relative;
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-3);
  }
  .live .dot {
    background: var(--good-solid);
  }
  .live .dot::after {
    content: '';
    position: absolute;
    inset: -4px;
    border-radius: 50%;
    border: 2px solid var(--good-solid);
    opacity: 0;
    animation: ping 2s ease-out infinite;
  }
  .warn .dot {
    background: var(--warn-solid);
  }
  .bad .dot {
    background: var(--bad-solid);
  }
  .bad strong {
    color: var(--bad);
  }
  @keyframes ping {
    0% {
      transform: scale(0.5);
      opacity: 0.7;
    }
    100% {
      transform: scale(1.3);
      opacity: 0;
    }
  }
</style>
