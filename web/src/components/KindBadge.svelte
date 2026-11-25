<script lang="ts">
  import type { AlertKind } from '../lib/api'
  import { ALERT_KIND } from '../lib/model'

  let { kind, manual = false }: { kind: AlertKind; manual?: boolean } = $props()

  const m = $derived(ALERT_KIND[kind] ?? ALERT_KIND.alerte)
</script>

<span class="kind {m.tone}" title={manual ? 'Ban posé à la main (cscli ou cette page)' : m.title}>{manual ? 'manuel' : m.label}</span>

<style>
  .kind {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 999px;
    font-size: 11.5px;
    font-weight: 620;
    white-space: nowrap;
    border: 1px solid transparent;
  }
  .bad {
    color: var(--bad);
    background: var(--bad-bg);
    border-color: var(--bad-border);
  }
  .warn {
    color: var(--warn);
    background: var(--warn-bg);
    border-color: var(--warn-border);
  }
  .quiet {
    color: var(--text-2);
    background: var(--surface-3);
  }
</style>
