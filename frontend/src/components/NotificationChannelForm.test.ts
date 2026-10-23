import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import type { NotificationChannel } from '$lib/types';

vi.mock('$lib/api', () => ({
  createNotificationChannel: vi.fn(),
  updateNotificationChannel: vi.fn(),
  testNotificationChannelDraft: vi.fn(),
  getTemplateVariables: vi.fn().mockResolvedValue([]),
}));
vi.mock('$lib/i18n', () => ({ t: (key: string, params?: Record<string, string>) => params?.error ? `${key}: ${params.error}` : key }));
import { createNotificationChannel, updateNotificationChannel, testNotificationChannelDraft } from '$lib/api';
import NotificationChannelForm from './NotificationChannelForm.svelte';

const webhook: NotificationChannel = {
  id: 'channel-1', name: 'Ops', type: 'webhook',
  config: { url: 'https://example.com/hook', method: 'POST', body_template: '{"text":"test"}', headers: [{ name: 'Authorization', value: '[REDACTED]' }, { name: 'X-Key', value_configured: true }] },
  created_at: '', updated_at: '',
};
function edit(channel = webhook) {
  const onSubmit = vi.fn();
  render(NotificationChannelForm, { mode: 'edit', initialData: channel, onSubmit, onCancel: vi.fn() });
  return onSubmit;
}
async function submit() { await fireEvent.submit(screen.getByTestId('notification-channel-form')); }

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(updateNotificationChannel).mockResolvedValue(webhook);
  vi.mocked(createNotificationChannel).mockResolvedValue(webhook);
  vi.mocked(testNotificationChannelDraft).mockResolvedValue({ success: true, channel_type: 'webhook' });
});
afterEach(cleanup);

describe('saved webhook secrets', () => {
  it('keeps saved values out of inputs and omits unchanged values after an unrelated edit', async () => {
    edit();
    expect((screen.getByTestId('input-webhook-header-value-0') as HTMLInputElement).value).toBe('');
    await fireEvent.input(screen.getByTestId('input-channel-name'), { target: { value: 'Renamed' } });
    await submit();
    expect(updateNotificationChannel).toHaveBeenCalledWith('channel-1', expect.objectContaining({ name: 'Renamed', config: expect.objectContaining({ headers: [{ name: 'Authorization' }, { name: 'X-Key' }] }) }));
  });
  it('retains a case-only rename and supports explicitly clearing a value', async () => {
    edit();
    await fireEvent.input(screen.getByTestId('input-webhook-header-name-0'), { target: { value: 'authorization' } });
    await fireEvent.input(screen.getByTestId('input-webhook-header-value-0'), { target: { value: 'temporary' } });
    await fireEvent.input(screen.getByTestId('input-webhook-header-value-0'), { target: { value: '' } });
    await submit();
    expect(updateNotificationChannel).toHaveBeenCalledWith('channel-1', expect.objectContaining({ config: expect.objectContaining({ headers: [{ name: 'authorization', value: '' }, { name: 'X-Key' }] }) }));
  });
  it('sends a replacement only for the edited header', async () => {
    edit();
    await fireEvent.input(screen.getByTestId('input-webhook-header-value-0'), { target: { value: 'Bearer replacement' } });
    await submit();
    expect(updateNotificationChannel).toHaveBeenCalledWith('channel-1', expect.objectContaining({ config: expect.objectContaining({ headers: [{ name: 'Authorization', value: 'Bearer replacement' }, { name: 'X-Key' }] }) }));
  });
  it('removes a header without moving another saved secret into its row', async () => {
    edit();
    await fireEvent.click(screen.getByTestId('btn-remove-webhook-header-0'));
    await submit();
    expect(updateNotificationChannel).toHaveBeenCalledWith('channel-1', expect.objectContaining({ config: expect.objectContaining({ headers: [{ name: 'X-Key' }] }) }));
  });
  it('requires a replacement when renaming a saved header', async () => {
    edit();
    await fireEvent.input(screen.getByTestId('input-webhook-header-name-0'), { target: { value: 'X-Renamed' } });
    await submit();
    expect(updateNotificationChannel).not.toHaveBeenCalled();
    await fireEvent.input(screen.getByTestId('input-webhook-header-value-0'), { target: { value: 'new secret' } });
    await submit();
    expect(updateNotificationChannel).toHaveBeenCalled();
  });
});

describe('draft tests', () => {
  it('tests current unsaved values with saved secret references without saving or navigating', async () => {
    const onSubmit = edit();
    await fireEvent.input(screen.getByTestId('input-webhook-url'), { target: { value: 'https://example.com/changed' } });
    await fireEvent.click(screen.getByTestId('btn-test-channel'));
    await waitFor(() => expect(testNotificationChannelDraft).toHaveBeenCalledWith(expect.objectContaining({ existing_channel_id: 'channel-1', config: expect.objectContaining({ url: 'https://example.com/changed', headers: [{ name: 'Authorization' }, { name: 'X-Key' }] }) })));
    expect(updateNotificationChannel).not.toHaveBeenCalled();
    expect(createNotificationChannel).not.toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByRole('status').textContent).toContain('notifications.test.success');
    await fireEvent.input(screen.getByTestId('input-channel-name'), { target: { value: 'Changed again' } });
    expect(screen.queryByRole('status')).toBeNull();
  });
  it('disables duplicate sends and ignores results for an edited draft', async () => {
    let finish!: (value: { success: boolean; channel_type: 'webhook' }) => void;
    vi.mocked(testNotificationChannelDraft).mockReturnValue(new Promise(resolve => { finish = resolve; }));
    edit();
    await fireEvent.click(screen.getByTestId('btn-test-channel'));
    expect((screen.getByTestId('btn-test-channel') as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.input(screen.getByTestId('input-channel-name'), { target: { value: 'Changed while sending' } });
    finish({ success: true, channel_type: 'webhook' });
    await waitFor(() => expect((screen.getByTestId('btn-test-channel') as HTMLButtonElement).disabled).toBe(false));
    expect(screen.queryByRole('status')).toBeNull();
  });
  it('shows an accessible failure without discarding the draft', async () => {
    vi.mocked(testNotificationChannelDraft).mockResolvedValue({ success: false, channel_type: 'webhook', error: 'Delivery rejected' });
    edit();
    await fireEvent.click(screen.getByTestId('btn-test-channel'));
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('Delivery rejected'));
    expect((screen.getByTestId('input-channel-name') as HTMLInputElement).value).toBe('Ops');
  });
});

describe('native platform payloads', () => {
  it('tests an unsaved channel without creating it', async () => {
    render(NotificationChannelForm, { mode: 'create', onSubmit: vi.fn(), onCancel: vi.fn() });
    await fireEvent.input(screen.getByTestId('input-channel-name'), { target: { value: 'New chat' } });
    const nativeSelect = screen.getByTestId('input-channel-type');
    await fireEvent.change(nativeSelect, { target: { value: 'telegram' } });
    await fireEvent.input(screen.getByTestId('input-telegram-bot-token'), { target: { value: '123456:bot-secret' } });
    await fireEvent.input(screen.getByTestId('input-telegram-chat-id'), { target: { value: '-100123' } });
    await fireEvent.input(screen.getByTestId('input-telegram-thread-id'), { target: { value: '42' } });
    await fireEvent.click(screen.getByTestId('btn-test-channel'));
    await waitFor(() => expect(testNotificationChannelDraft).toHaveBeenCalledWith({ name: 'New chat', type: 'telegram', config: { bot_token: '123456:bot-secret', chat_id: '-100123', message_thread_id: 42 } }));
    expect(createNotificationChannel).not.toHaveBeenCalled();
  });

  it.each([
    { type: 'slack' as const, config: { webhook_url_configured: true }, input: 'input-slack-webhook-url', replacement: 'https://hooks.slack.com/services/new', key: 'webhook_url' },
    { type: 'telegram' as const, config: { bot_token_configured: true, chat_id: '-100123', message_thread_id: 42 }, input: 'input-telegram-bot-token', replacement: '123456:replacement', key: 'bot_token' },
    { type: 'matrix' as const, config: { access_token_configured: true, homeserver_url: 'https://matrix.example.com', room_id: '!ops:example.com' }, input: 'input-matrix-access-token', replacement: 'new matrix token', key: 'access_token' },
  ])('preserves and replaces $type secrets', async ({ type, config, input, replacement, key }) => {
    edit({ ...webhook, type, config });
    expect((screen.getByTestId(input) as HTMLInputElement).value).toBe('');
    await submit();
    const preserved = vi.mocked(updateNotificationChannel).mock.calls[0][1].config;
    expect(preserved).not.toHaveProperty(key);
    expect(Object.keys(preserved).some(key => key.endsWith('_configured'))).toBe(false);
    await fireEvent.input(screen.getByTestId(input), { target: { value: replacement } });
    await submit();
    expect(vi.mocked(updateNotificationChannel).mock.calls[1][1].config).toHaveProperty(key, replacement);
  });
});
