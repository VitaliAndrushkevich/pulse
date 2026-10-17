import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import PasswordChangeSection from '../PasswordChangeSection.svelte';
import { ApiRequestError, changePassword } from '$lib/api';
import { goto } from '$app/navigation';
vi.mock('$lib/i18n/locale.svelte', () => ({ t: (key: string) => key }));
vi.mock('$lib/api', async (original) => ({ ...await original<typeof import('$lib/api')>(), changePassword: vi.fn() }));
beforeEach(() => { vi.clearAllMocks(); });
async function fill(current: string, next: string, confirm = next) {
  await fireEvent.input(screen.getByLabelText('settings.password.currentPassword'), { target: { value: current } });
  await fireEvent.input(screen.getByLabelText('settings.password.newPassword'), { target: { value: next } });
  await fireEvent.input(screen.getByLabelText('settings.password.confirmPassword'), { target: { value: confirm } });
}
describe('required password change', () => {
  it('prevents mismatched or unchanged passwords from completing first login', async () => {
    render(PasswordChangeSection, { forced: true });
    await fill('temporary123', 'different123', 'wrong123');
    expect(screen.getByRole('button')).toHaveProperty('disabled', true);
    await fill('temporary123', 'temporary123');
    expect(screen.getByRole('button')).toHaveProperty('disabled', true);
    expect(changePassword).not.toHaveBeenCalled();
    expect(goto).not.toHaveBeenCalled();
  });
  it('keeps the user in the password form on an incorrect temporary password', async () => {
    vi.mocked(changePassword).mockRejectedValue(new ApiRequestError(401, { code: 'UNAUTHORIZED', message: 'Incorrect password' }, null));
    render(PasswordChangeSection, { forced: true });
    await fill('wrong-password', 'different123');
    await fireEvent.submit(screen.getByRole('button').closest('form')!);
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('settings.password.errors.incorrectCurrent'));
    expect(goto).not.toHaveBeenCalled();
  });
  it('continues to the dashboard only after the password has changed', async () => {
    vi.mocked(changePassword).mockResolvedValue({ token: 'fresh-session' } as never);
    render(PasswordChangeSection, { forced: true });
    await fill('temporary123', 'different123');
    await fireEvent.submit(screen.getByRole('button').closest('form')!);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/'));
    expect(changePassword).toHaveBeenCalledWith({ current_password: 'temporary123', new_password: 'different123' });
  });
});
