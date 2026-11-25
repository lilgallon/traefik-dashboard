<script lang="ts">
  import { WINDOWS, type Window } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { windowLabel } from '../lib/model'

  let { value, onchange }: { value: Window | null; onchange: (w: Window) => void } = $props()
</script>

<div class="segmented" role="group" aria-label={t('period.aria')}>
  {#each WINDOWS as w (w)}
    <button type="button" aria-pressed={value === w} onclick={() => onchange(w)}>{windowLabel(w)}</button>
  {/each}
</div>
