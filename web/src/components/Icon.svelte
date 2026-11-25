<script lang="ts">
  // Petits pictogrammes au trait (24 × 24), dessinés ici : aucune dépendance.
  type Name =
    | 'check' | 'alert' | 'clock' | 'offline' | 'chevron' | 'close' | 'copy' | 'sun' | 'moon' | 'auto'
    | 'shield' | 'ban' | 'arrow-down' | 'arrow-up' | 'activity' | 'info' | 'pulse' | 'pause' | 'play'

  let { name, size = 16 }: { name: Name; size?: number } = $props()

  const paths: Record<Name, string> = {
    check: 'M20 6 9 17l-5-5',
    alert: 'M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z',
    clock: 'M12 7v5l3 2m6-2a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
    offline: 'M2 2l20 20M8.5 16.4a5 5 0 0 1 7 0M5 12.9a10 10 0 0 1 5.2-2.7m4.4.3A10 10 0 0 1 19 12.9M1.4 9a16 16 0 0 1 4.3-2.8m4.1-1.3A16 16 0 0 1 22.6 9M12 20h.01',
    chevron: 'm9 18 6-6-6-6',
    close: 'M18 6 6 18M6 6l12 12',
    copy: 'M9 9h10a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2Zm-4 6H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1',
    sun: 'M12 3v2m0 14v2M5.6 5.6 7 7m10 10 1.4 1.4M3 12h2m14 0h2M5.6 18.4 7 17M17 7l1.4-1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z',
    moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z',
    auto: 'M12 3a9 9 0 1 0 0 18Zm0 0a9 9 0 0 1 0 18',
    shield: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z',
    ban: 'M4.9 4.9l14.2 14.2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
    'arrow-down': 'M12 5v14m7-7-7 7-7-7',
    'arrow-up': 'M12 19V5m-7 7 7-7 7 7',
    activity: 'M22 12h-4l-3 9L9 3l-3 9H2',
    info: 'M12 16v-4m0-4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
    pulse: 'M3 12h4l2-6 4 12 2-6h6',
    pause: 'M8 5v14M16 5v14',
    play: 'M7 4.5v15l12-7.5Z',
  }
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
  stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
  <path d={paths[name]} />
</svg>

<style>
  svg {
    flex: none;
    display: block;
  }
</style>
