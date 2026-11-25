<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import Icon from './Icon.svelte'

  let { items }: { items: string[] } = $props()
</script>

{#if items.length > 0}
  <ul class="warnings" aria-label={t('warnings.aria')}>
    {#each items as w (w)}
      <li><Icon name="alert" size={15} /><span>{w}</span></li>
    {/each}
  </ul>
{/if}

<style>
  .warnings {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  li {
    display: flex;
    gap: 9px;
    align-items: flex-start;
    padding: 9px 12px;
    border-radius: var(--radius-sm);
    background: var(--warn-bg);
    border: 1px solid var(--warn-border);
    color: var(--warn);
    font-size: 13px;
  }
  li :global(svg) {
    margin-top: 2px;
  }
  span {
    color: var(--text);
  }
</style>
