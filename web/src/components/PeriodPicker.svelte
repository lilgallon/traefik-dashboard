<script lang="ts">
  import { WINDOWS, type Window } from '../lib/api'
  import { WINDOW_LABEL } from '../lib/model'

  let { value, onchange }: { value: Window | null; onchange: (w: Window) => void } = $props()
</script>

<div class="segmented" role="group" aria-label="Période affichée">
  {#each WINDOWS as w (w)}
    <button type="button" aria-pressed={value === w} onclick={() => onchange(w)}>{WINDOW_LABEL[w]}</button>
  {/each}
</div>
