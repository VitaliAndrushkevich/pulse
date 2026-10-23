# Notifications

Pulse delivers monitor and heartbeat alerts through Email, Webhook, Slack,
Matrix, and Telegram channels. Create a channel in **Notifications**, test it,
save it, then add a binding on a monitor or heartbeat to choose its triggers.
Creating a channel alone does not subscribe it to alerts.

## Set up a channel

### Slack

1. Create a Slack app in your workspace, enable **Incoming Webhooks**, and add
   a webhook to the destination channel. Your workspace may require approval.
2. Set the app display name to **Pulse** and use a Pulse mark from
   [`frontend/static/brand/`](../frontend/static/brand/) as its icon.
3. Select **Slack** in Pulse and paste the incoming webhook URL. Both
   `hooks.slack.com` and `hooks.slack-gov.com` URLs are supported.
4. Click **Test** and check the destination before saving.

Pulse sends a Block Kit message with a plain-text fallback. Slack controls the
sender name, icon, and destination through the app/webhook configuration; Pulse
cannot override them per message. The complete webhook URL is a secret and is
encrypted at rest. See [Slack's setup guide](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/).

### Telegram

1. Create a bot with **@BotFather** and copy its token. You can set its name and
   picture to Pulse there.
2. Start a conversation with the bot or add it to a group with permission to
   send messages. A bot cannot initiate a private conversation with a user who
   has never started it. Channel destinations require the appropriate posting
   permissions.
3. Select **Telegram** and enter the bot token and chat ID. Preserve the minus
   sign in negative group IDs. Chat IDs are strings in the API; a public
   destination username such as `@operations` can also be used where supported.
4. For a forum topic, optionally enter its numeric message thread ID.
5. Click **Test**, verify receipt, then save.

Pulse sends plain text through `sendMessage` with link previews disabled.
The bot token is encrypted at rest. See the [Telegram Bot API](https://core.telegram.org/bots/api#sendmessage)
for chat identifiers and bot permissions.

### Matrix

1. Create a dedicated bot account on your homeserver and obtain its access token.
2. Join that account to an **unencrypted** room and give it permission to send
   messages. Set its display name/avatar to Pulse in your Matrix client.
3. Select **Matrix** and enter the homeserver base URL, room ID (for example
   `!operations:example.org`), and access token. Use the room ID, not an alias.
4. Click **Test**, verify receipt, then save.

Self-hosted homeservers are supported. Use HTTPS when credentials traverse a
network. Pulse checks the room encryption state before sending an `m.notice`;
encrypted rooms and rooms whose state cannot be verified are rejected. Full
end-to-end encryption is not supported. The access token is encrypted at rest.
See [Matrix sending messages](https://spec.matrix.org/latest/client-server-api/#put_matrixclientv3roomsroomidsendeventtypetxnid).

### Email and generic webhooks

Email requires instance SMTP settings in **Settings → SMTP** plus channel
recipients. Generic webhooks retain their method, custom headers, and Go
`text/template` body for integrations needing a custom payload. Native channels
format messages automatically; no body template is needed.

## Edit secrets safely

Saved secret fields stay blank and show that a value is configured. Leave them
unchanged to retain the stored value, or enter a replacement. Secret values are
never returned by the channel API.

Webhook header rows follow the same rule. Remove a row to delete the header;
renaming a saved header requires entering a new value. Header names are matched
case-insensitively and duplicate names are rejected. API clients can explicitly
send an empty string to replace a webhook header with an empty value.

Older releases could overwrite header secrets with `[REDACTED]` after editing.
The original value cannot be recovered from that row: re-enter the affected
header once after upgrading. The fix preserves future edits; it cannot restore
an already-lost token.

## Test before saving

The create/edit form's **Test** button sends a real sample notification using
the current unsaved values, including saved secrets left unchanged. It does not
save settings or navigate away. Editing fields clears stale test feedback.
The list's Test action continues to test the saved channel configuration.

Tests have a 10-second deadline, do not create delivery logs, and are never
retried. At most four tests can run concurrently per server; retry manually if
the server reports that all test slots are busy. A successful test verifies that
configuration at that moment, not future credentials or destination permissions.

## Delivery and troubleshooting

Native messages include the resource name, event, state, time, and available
operational details. Monitor and heartbeat links use `PULSE_BASE_URL`; set it to
your public Pulse URL to include links.

Production deliveries run in a bounded worker pool. Retryable failures get up to
four attempts with 30/60/120-second backoff; provider cooldowns can extend the
delay without occupying a worker. Authentication/configuration/permission
failures are permanent for native providers. Matrix reuses its transaction ID
on retries. Slack and Telegram may show duplicates after an ambiguous network
failure. Delivery attempts appear in the channel's delivery log; errors omit
credential-bearing request URLs and remote response bodies.

Pending deliveries and retries are in memory and are lost on restart. The job
buffer is 256 and the retry backlog is capped at 1024; overload can drop work.
Shutdown cancels pending retries and drains queued work with the existing worker
pool up to the configured deadline. Reminders are still not wired end to end.

For a failure, check destination membership, permissions, room encryption, token
validity, and outbound connectivity. Tokens entered in native channels must be
raw tokens; Pulse adds the provider's authentication scheme. A generic webhook
Authorization header must include the complete value, such as `Bearer TOKEN`.

## Upgrade and key rotation

Migration 021 expands the existing channel-type constraint automatically at
startup. Its downgrade refuses to proceed while native channels exist; remove
or convert those channels deliberately before downgrading.

The key-rotation command now covers legacy secrets, monitor credentials, SMTP
passwords, webhook header values, and all three native channel secrets within
one database transaction. Back up the database and protect the existing key.
Stop every Pulse writer before running `make rotate-key` with the old
`PULSE_SECRET_KEY` and new `PULSE_SECRET_KEY_NEW`. After success, configure the
new key, remove `PULSE_SECRET_KEY_NEW`, and restart. Do not change the active key
before rotation commits. If rotation fails, keep the old key; the transaction
does not commit partial results. Keep the database backup and its matching key
together for rollback. Never run rotation alongside a running Pulse instance.
