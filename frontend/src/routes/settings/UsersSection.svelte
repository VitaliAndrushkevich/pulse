<script lang="ts">
  import SelectField from '../../components/SelectField.svelte';
  import { onMount } from 'svelte';
  import { listUsers, createUser, updateUser, resetUserPassword } from '$lib/api';
  import { getUser } from '$lib/stores/auth.svelte';
  import type { User, UserRole } from '$lib/types';
  import { t } from '$lib/i18n';
  import Pagination from '../../components/Pagination.svelte';

  let users = $state.raw<User[]>([]);
  let page = $state(1);
  let totalPages = $state(1);
  let loading = $state(true);
  let busy = $state(false);
  let error = $state('');
  let success = $state('');
  let email = $state('');
  let password = $state('');
  let role = $state<UserRole>('read_only');
  let resetting = $state<string | null>(null);
  let resetPassword = $state('');
  const inputClass = 'mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary';
  const buttonClass = 'rounded-md border border-[var(--color-border)] px-3 py-2 text-sm text-primary disabled:opacity-50';
  function validPassword(value: string) {
    const bytes = new TextEncoder().encode(value).length;
    return bytes >= 8 && bytes <= 72;
  }

  async function load(newPage = page) {
    loading = true;
    error = '';
    try {
      const result = await listUsers(newPage);
      users = result.data;
      page = result.page;
      totalPages = result.total_pages;
    } catch (err) {
      error = err instanceof Error ? err.message : t('users.failed');
    } finally { loading = false; }
  }
  onMount(() => { void load(); });

  async function run(action: () => Promise<unknown>, message: string) {
    if (busy) return;
    busy = true;
    error = '';
    success = '';
    try {
      await action();
      success = message;
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : t('users.failed');
    } finally { busy = false; }
  }

  async function add(event: SubmitEvent) {
    event.preventDefault();
    if (!validPassword(password)) return;
    await run(async () => {
      await createUser({ email: email.trim(), password, role });
      email = '';
      password = '';
      role = 'read_only';
    }, t('users.created'));
  }
  async function reset(event: SubmitEvent, user: User) {
    event.preventDefault();
    if (!validPassword(resetPassword)) return;
    await run(async () => {
      await resetUserPassword(user.id, resetPassword);
      resetPassword = '';
      resetting = null;
    }, t('users.passwordReset'));
  }
</script>

<div class="space-y-6">
  <p class="text-sm text-secondary">{t('users.passwordHint')}</p>
  {#if error}<p role="alert" class="text-sm text-[var(--color-error)]">{error}</p>{/if}
  {#if success}<p role="status" class="text-sm text-[var(--color-success)]">{success}</p>{/if}
  <form onsubmit={add} class="grid gap-4 sm:grid-cols-2">
    <label class="text-sm text-primary" for="user-email">{t('users.email')}
      <input class={inputClass} id="user-email" type="email" autocomplete="off" required bind:value={email} disabled={busy} />
    </label>
    <label class="text-sm text-primary" for="user-password">{t('users.temporaryPassword')}
      <input class={inputClass} id="user-password" type="password" autocomplete="new-password" required minlength={8} maxlength={72} bind:value={password} disabled={busy} />
    </label>
    <label class="text-sm text-primary" for="user-role">{t('users.role')}
      <SelectField class="mt-1 w-full" id="user-role" bind:value={role} disabled={busy}>
        <option value="read_only">{t('users.readOnly')}</option>
        <option value="admin">{t('users.admin')}</option>
      </SelectField>
    </label>
    <div class="flex items-end"><button type="submit" class="rounded-md bg-brand-600 px-4 py-2 text-sm text-white disabled:opacity-50" disabled={busy || !validPassword(password) || !email.trim()}>{t('users.create')}</button></div>
  </form>

  {#if loading}<p class="text-sm text-secondary">{t('users.loading')}</p>
  {:else}
    <ul class="divide-y divide-[var(--color-border)]">
      {#each users as user (user.id)}
        {@const self = user.id === getUser()?.id}
        <li class="space-y-3 py-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="min-w-0">
              <p class="break-all text-sm font-medium text-primary">{user.email}{self ? ` (${t('users.you')})` : ''}</p>
              <p class="text-xs text-secondary">{t(user.role === 'admin' ? 'users.admin' : 'users.readOnly')} · {t(user.enabled ? 'users.enabled' : 'users.disabled')}{user.must_change_password ? ` · ${t('users.passwordPending')}` : ''}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="button" class={buttonClass} disabled={busy || self} onclick={() => run(() => updateUser(user.id, { role: user.role === 'admin' ? 'read_only' : 'admin' }), t('users.updated'))}>{t(user.role === 'admin' ? 'users.makeReadOnly' : 'users.makeAdmin')}</button>
              <button type="button" class={buttonClass} disabled={busy || self} onclick={() => run(() => updateUser(user.id, { enabled: !user.enabled }), t('users.updated'))}>{t(user.enabled ? 'users.disable' : 'users.enable')}</button>
              {#if !self}<button type="button" class={buttonClass} disabled={busy} onclick={() => { resetting = user.id; resetPassword = ''; }}>{t('users.resetPassword')}</button>{/if}
            </div>
          </div>
          {#if resetting === user.id}
            <form class="flex flex-wrap items-end gap-3" onsubmit={(event) => reset(event, user)}>
              <label for="reset-user-password" class="text-sm text-primary">{t('users.temporaryPassword')}
                <input id="reset-user-password" class={inputClass} type="password" autocomplete="new-password" required minlength={8} maxlength={72} bind:value={resetPassword} disabled={busy} />
              </label>
              <button class={buttonClass} type="submit" disabled={busy || !validPassword(resetPassword)}>{t('users.resetPassword')}</button>
              <button class={buttonClass} type="button" disabled={busy} onclick={() => { resetting = null; resetPassword = ''; }}>{t('common.cancel')}</button>
            </form>
          {/if}
        </li>
      {/each}
    </ul>
    {#if totalPages > 1}<Pagination {page} {totalPages} onPageChange={load} />{/if}
  {/if}
</div>
