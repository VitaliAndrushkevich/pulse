import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import UsersSection from '../UsersSection.svelte';
import { createUser, listUsers, resetUserPassword, updateUser } from '$lib/api';
const admin = { id: 'admin', email: 'admin@example.com', role: 'admin', enabled: true, must_change_password: false };
const reader = { id: 'reader', email: 'reader@example.com', role: 'read_only', enabled: true, must_change_password: true };
vi.mock('$lib/i18n', () => ({ t: (key: string) => key }));
vi.mock('$lib/stores/auth.svelte', () => ({ getUser: () => admin }));
vi.mock('$lib/api', () => ({ listUsers: vi.fn(), createUser: vi.fn(), updateUser: vi.fn(), resetUserPassword: vi.fn() }));
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listUsers).mockResolvedValue({ data: [admin, reader], page: 1, total_pages: 1, limit: 20, total: 2 } as never);
});
async function readerRow() {
  const email = await screen.findByText(reader.email);
  return within(email.closest('li')!);
}
describe('administrator user management', () => {
  it('creates read-only users by default and clears the temporary password', async () => {
    render(UsersSection);
    await screen.findByText(reader.email);
    expect(screen.getByRole('combobox').textContent).toContain('users.readOnly');
    await fireEvent.input(screen.getByLabelText('users.email'), { target: { value: 'new@example.com' } });
    await fireEvent.input(screen.getByLabelText('users.temporaryPassword'), { target: { value: 'temporary123' } });
    await fireEvent.click(screen.getByRole('button', { name: 'users.create' }));
    await waitFor(() => expect(createUser).toHaveBeenCalledWith({ email: 'new@example.com', password: 'temporary123', role: 'read_only' }));
    expect(screen.getByLabelText('users.temporaryPassword')).toHaveProperty('value', '');
  });
  it('disables a user and changes their role', async () => {
    render(UsersSection);
    const row = await readerRow();
    await fireEvent.click(row.getByRole('button', { name: 'users.disable' }));
    await waitFor(() => expect(updateUser).toHaveBeenCalledWith('reader', { enabled: false }));
    await waitFor(async () => expect((await readerRow()).getByRole('button', { name: 'users.makeAdmin' })).toHaveProperty('disabled', false));
    await fireEvent.click((await readerRow()).getByRole('button', { name: 'users.makeAdmin' }));
    await waitFor(() => expect(updateUser).toHaveBeenCalledWith('reader', { role: 'admin' }));
  });
  it('resets a temporary password and clears its form', async () => {
    render(UsersSection);
    const row = await readerRow();
    await fireEvent.click(row.getByRole('button', { name: 'users.resetPassword' }));
    await fireEvent.input(row.getByLabelText('users.temporaryPassword'), { target: { value: 'replacement123' } });
    await fireEvent.submit(row.getByLabelText('users.temporaryPassword').closest('form')!);
    await waitFor(() => expect(resetUserPassword).toHaveBeenCalledWith('reader', 'replacement123'));
    await waitFor(() => expect(row.queryByLabelText('users.temporaryPassword')).toBeNull());
  });
  it('protects the signed-in administrator from changing their own role or disabling themselves', async () => {
    render(UsersSection);
    const label = await screen.findByText(/admin@example.com/);
    const row = within(label.closest('li')!);
    expect(row.getByRole('button', { name: 'users.makeReadOnly' })).toHaveProperty('disabled', true);
    expect(row.getByRole('button', { name: 'users.disable' })).toHaveProperty('disabled', true);
    expect(row.queryByRole('button', { name: 'users.resetPassword' })).toBeNull();
  });
});
