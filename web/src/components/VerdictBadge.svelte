<script lang="ts">
  import type { VerdictKind } from '../lib/api'

  let { verdict }: { verdict: VerdictKind } = $props()

  // La couleur n'est jamais seule : le libellé est toujours là.
  const meta: Record<VerdictKind, { tone: string; title: string }> = {
    connue: { tone: 'neutral', title: 'Toutes ses requêtes sont couvertes par une règle de nom' },
    succès: { tone: 'bad', title: 'A obtenu un succès hors des URL anonymes' },
    public: { tone: 'quiet', title: 'Succès uniquement sur des URL anonymes par conception' },
    bloquée: { tone: 'good', title: 'Seulement des refus (403) : allowlist, CrowdSec ou AppSec' },
    tâtonne: { tone: 'warn', title: 'Des 404 ou autres 4xx, aucun succès' },
    autre: { tone: 'quiet', title: 'Ni succès ni refus : redirections, abandons ou erreurs serveur' },
  }
  const m = $derived(meta[verdict] ?? meta.autre)
</script>

<span class="badge {m.tone}" title={m.title}><i></i>{verdict}</span>

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
