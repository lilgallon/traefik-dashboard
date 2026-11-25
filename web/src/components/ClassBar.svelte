<script lang="ts">
  import type { ClassCounts } from '../lib/api'
  import { formatCount, percent } from '../lib/format'
  import { STACK, total } from '../lib/model'

  // Barre segmentée fine : la répartition par famille de statut. Toujours
  // accompagnée de ses chiffres (colonnes du tableau ou légende).
  let { counts, height = 6 }: { counts: ClassCounts; height?: number } = $props()

  const sum = $derived(total(counts))
  const parts = $derived(
    STACK.map((s) => ({ ...s, n: s.key === 'abandon' ? counts.abandon + counts['1xx'] : counts[s.key] })).filter((p) => p.n > 0),
  )
  const label = $derived(parts.map((p) => `${p.label} : ${formatCount(p.n)} (${percent(p.n, sum)})`).join(', '))
</script>

<span class="bar" style:height="{height}px" role="img" aria-label={label || 'aucune requête'} title={label}>
  {#each parts as p (p.key)}
    <span style:flex-grow={p.n} style:background="var({p.cssVar})"></span>
  {/each}
</span>

<style>
  .bar {
    display: flex;
    gap: 2px;
    width: 100%;
    min-width: 40px;
    border-radius: 999px;
    overflow: hidden;
    background: var(--surface-3);
  }
  .bar span {
    flex-basis: 0;
    min-width: 2px;
    transition: flex-grow 0.4s ease;
  }
</style>
