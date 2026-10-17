<script lang="ts">
  import { t } from '$lib/i18n';

  interface Props {
    pingUrl: string;
    pingFailUrl: string;
  }

  let { pingUrl, pingFailUrl }: Props = $props();

  // Snippets from design §11, populated with this heartbeat's ping URLs.
  const shellSnippet = $derived(
    `# At the end of a successful job:\n` +
      `curl -fsS -m 10 --retry 3 ${pingUrl}\n` +
      `# Report failure when the preceding command fails:\n` +
      `run_backup || curl -fsS -m 10 ${pingFailUrl}`
  );

  const cronSnippet = $derived(
    `# Pinging BEFORE the work defeats the purpose — ping only on success.\n` +
      `0 2 * * * /usr/local/bin/backup.sh && curl -fsS -m 10 ${pingUrl}`
  );

  const ciSnippet = $derived(
    `# GitHub Actions, scheduled workflow\n` +
      `- name: Notify Pulse on success\n` +
      `  if: success()\n` +
      `  run: curl -fsS -m 10 ${pingUrl}`
  );

  const examples = $derived([
    { id: 'shell', label: t('heartbeats.examples.shell'), code: shellSnippet },
    { id: 'cron', label: t('heartbeats.examples.cron'), code: cronSnippet },
    { id: 'ci', label: t('heartbeats.examples.ci'), code: ciSnippet },
  ]);

  // Track which snippet was last copied for transient feedback.
  let copiedId = $state<string | null>(null);
  let copyTimeout: ReturnType<typeof setTimeout> | null = null;

  async function copy(id: string, code: string) {
    try {
      await navigator.clipboard.writeText(code);
      copiedId = id;
      if (copyTimeout) clearTimeout(copyTimeout);
      copyTimeout = setTimeout(() => {
        copiedId = null;
      }, 2000);
    } catch {
      // Clipboard unavailable — the snippet remains selectable in the UI.
    }
  }
</script>

<div class="rounded-xl border border-[var(--color-border)] bg-surface" data-testid="ping-examples">
  <div class="border-b border-[var(--color-border)] px-5 py-3">
    <h2 class="text-sm font-semibold text-primary">{t('heartbeats.examples.title')}</h2>
  </div>

  <div class="p-5 space-y-4">
    {#each examples as example (example.id)}
      <div data-testid="ping-example-{example.id}">
        <div class="flex items-center justify-between mb-1.5">
          <span class="text-xs font-medium text-secondary">{example.label}</span>
          <button
            type="button"
            onclick={() => copy(example.id, example.code)}
            class="rounded-md border border-[var(--color-border)] bg-surface px-2 py-1 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
            data-testid="copy-ping-example-{example.id}"
          >
            {copiedId === example.id ? t('common.copied') : t('common.copy')}
          </button>
        </div>
        <pre class="overflow-x-auto rounded-md border border-[var(--color-border)] bg-page px-3 py-2 text-xs font-mono text-primary"><code>{example.code}</code></pre>
      </div>
    {/each}

    <!-- Independence / validity note (design §11, R11.5) -->
    <p class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800" data-testid="ping-independence-note">
      {t('heartbeats.examples.note')}
    </p>
  </div>
</div>
