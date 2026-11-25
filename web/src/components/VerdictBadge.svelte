<script lang="ts">
  import type { VerdictKind } from '../lib/api'
  import { verdictMeta } from '../lib/model'

  let { verdict }: { verdict: VerdictKind } = $props()

  // La couleur n'est jamais seule : le libellé est toujours là.
  const m = $derived(verdictMeta(verdict))
</script>

<span class="badge {m.tone}" title={m.title}><i></i>{m.label}</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 9px 2px 8px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
    border: 1px solid transparent;
  }
  i {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }
  .neutral {
    color: var(--text-2);
    border-color: var(--border-strong);
  }
  .neutral i {
    display: none;
  }
  .neutral {
    padding-left: 9px;
  }
  .quiet {
    color: var(--text-2);
    background: var(--surface-3);
  }
  .good {
    color: var(--good);
    background: var(--good-bg);
    border-color: var(--good-border);
  }
  .warn {
    color: var(--warn);
    background: var(--warn-bg);
    border-color: var(--warn-border);
  }
  .bad {
    color: var(--bad);
    background: var(--bad-bg);
    border-color: var(--bad-border);
  }
</style>
